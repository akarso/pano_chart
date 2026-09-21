package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// putEvalScript builds the symbol hash on a temp key, then swaps live keys.
// Live hash is never DELed before the replacement is ready (RENAME).
// Mid-script process abort can still leave array/at vs hash briefly skewed;
// the next successful Put heals. Concurrent callers see atomic script commits.
//
// KEYS[1]=array KEYS[2]=at KEYS[3]=sym KEYS[4]=symTmp
// ARGV[1]=arrayJSON ARGV[2]=atUnix ARGV[3]=ttlSec
// ARGV[4..]= field, value pairs for the hash (may be empty).
const putEvalScript = `
local ttl = tonumber(ARGV[3])
redis.call('DEL', KEYS[4])
local i = 4
while i <= #ARGV do
  redis.call('HSET', KEYS[4], ARGV[i], ARGV[i+1])
  i = i + 2
end
if #ARGV >= 4 then
  redis.call('EXPIRE', KEYS[4], ttl)
  redis.call('RENAME', KEYS[4], KEYS[3])
else
  redis.call('DEL', KEYS[3])
  redis.call('DEL', KEYS[4])
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ttl)
redis.call('SET', KEYS[2], ARGV[2], 'EX', ttl)
return 'OK'
`

// releaseLockScript deletes key only if value == holder.
const releaseLockScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// RedisClient is the subset of Redis operations the evaluation store needs.
// Get/HGet should surface redis.Nil as an error (shared GoRedisClient contract);
// the store maps Nil → miss locally.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	HGet(ctx context.Context, key, field string) (string, error)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
}

// RedisEvaluationStore persists EvaluationSnapshots in Redis.
//
// Keys (tf must be a canonical timeframe — validated on every call):
//   - eval:{tf}       → JSON array of snapshots
//   - eval:{tf}:at    → unix seconds of Put
//   - eval:{tf}:sym   → Redis hash field=symbol → JSON snapshot
//
// Empty Put writes "[]" with a fresh at (valid empty snapshot, not a miss).
type RedisEvaluationStore struct {
	redis RedisClient
}

// NewRedisEvaluationStore constructs the store.
func NewRedisEvaluationStore(redis RedisClient) *RedisEvaluationStore {
	return &RedisEvaluationStore{redis: redis}
}

func arrayKey(tf string) string     { return fmt.Sprintf("eval:%s", tf) }
func atKey(tf string) string        { return fmt.Sprintf("eval:%s:at", tf) }
func symbolKey(tf string) string    { return fmt.Sprintf("eval:%s:sym", tf) }
func symbolTmpKey(tf string) string { return fmt.Sprintf("eval:%s:sym:tmp", tf) }

func parseTF(tf string) (domain.Timeframe, error) {
	parsed, err := domain.NewTimeframe(tf)
	if err != nil {
		return "", fmt.Errorf("evaluation store: %w", err)
	}
	return parsed, nil
}

func (s *RedisEvaluationStore) get(ctx context.Context, key string) (string, error) {
	raw, err := s.redis.Get(ctx, key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return raw, nil
}

func (s *RedisEvaluationStore) hget(ctx context.Context, key, field string) (string, error) {
	raw, err := s.redis.HGet(ctx, key, field)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return raw, nil
}

// Put implements ports.EvaluationStore.
func (s *RedisEvaluationStore) Put(ctx context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error {
	parsed, err := parseTF(tf)
	if err != nil {
		return err
	}
	ttl := domain.EvaluationStoreTTL(parsed)
	atUnix := computedAt.Unix()

	bySym := make(map[string]domain.EvaluationSnapshot, len(evals))
	order := make([]string, 0, len(evals))
	for _, e := range evals {
		e.ComputedAt = atUnix
		if e.Timestamp.IsZero() {
			e.Timestamp = computedAt
		}
		e.Timeframe = parsed.String()
		if _, seen := bySym[e.Symbol]; !seen {
			order = append(order, e.Symbol)
		}
		bySym[e.Symbol] = e
	}
	stamped := make([]domain.EvaluationSnapshot, 0, len(order))
	for _, sym := range order {
		stamped = append(stamped, bySym[sym])
	}

	arrayJSON, err := json.Marshal(stamped)
	if err != nil {
		return fmt.Errorf("marshal evals: %w", err)
	}

	args := make([]interface{}, 0, 3+len(stamped)*2)
	args = append(args, string(arrayJSON), strconv.FormatInt(atUnix, 10), int(ttl.Seconds()))
	for _, e := range stamped {
		b, mErr := json.Marshal(e)
		if mErr != nil {
			return fmt.Errorf("marshal symbol %s: %w", e.Symbol, mErr)
		}
		args = append(args, e.Symbol, string(b))
	}

	tfStr := parsed.String()
	_, err = s.redis.Eval(ctx, putEvalScript, []string{
		arrayKey(tfStr), atKey(tfStr), symbolKey(tfStr), symbolTmpKey(tfStr),
	}, args...)
	if err != nil {
		return fmt.Errorf("atomic put: %w", err)
	}
	return nil
}

// Get implements ports.EvaluationStore.
func (s *RedisEvaluationStore) Get(ctx context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error) {
	parsed, err := parseTF(tf)
	if err != nil {
		return nil, time.Time{}, err
	}
	at, err := s.readAt(ctx, parsed.String())
	if err != nil {
		return nil, time.Time{}, err
	}
	raw, err := s.get(ctx, arrayKey(parsed.String()))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("get array: %w", err)
	}
	if raw == "" {
		return nil, time.Time{}, ports.ErrEvaluationNotFound
	}
	var evals []domain.EvaluationSnapshot
	if err := json.Unmarshal([]byte(raw), &evals); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal evals: %w", err)
	}
	return evals, at, nil
}

// GetSymbol implements ports.EvaluationStore.
func (s *RedisEvaluationStore) GetSymbol(ctx context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error) {
	parsed, err := parseTF(tf)
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, err
	}
	at, err := s.readAt(ctx, parsed.String())
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, err
	}
	raw, err := s.hget(ctx, symbolKey(parsed.String()), symbol)
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, fmt.Errorf("hget symbol: %w", err)
	}
	if raw == "" {
		return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
	}
	var snap domain.EvaluationSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, fmt.Errorf("unmarshal symbol: %w", err)
	}
	return snap, at, nil
}

func (s *RedisEvaluationStore) readAt(ctx context.Context, tf string) (time.Time, error) {
	raw, err := s.get(ctx, atKey(tf))
	if err != nil {
		return time.Time{}, fmt.Errorf("get at: %w", err)
	}
	if raw == "" {
		return time.Time{}, ports.ErrEvaluationNotFound
	}
	unix, scanErr := strconv.ParseInt(raw, 10, 64)
	if scanErr != nil {
		return time.Time{}, fmt.Errorf("parse at: %w", scanErr)
	}
	return time.Unix(unix, 0).UTC(), nil
}

// RedisRefreshLock implements RefreshLock via SET NX EX + compare-and-del Release.
type RedisRefreshLock struct {
	redis RedisClient
}

// NewRedisRefreshLock wraps a Redis client as a refresh lease.
func NewRedisRefreshLock(redis RedisClient) *RedisRefreshLock {
	return &RedisRefreshLock{redis: redis}
}

// TryAcquire implements application/evaluation.RefreshLock.
func (l *RedisRefreshLock) TryAcquire(ctx context.Context, key string, ttl time.Duration, holder string) (bool, error) {
	return l.redis.SetNX(ctx, key, holder, ttl)
}

// Release implements application/evaluation.RefreshLock.
func (l *RedisRefreshLock) Release(ctx context.Context, key, holder string) error {
	_, err := l.redis.Eval(ctx, releaseLockScript, []string{key}, holder)
	return err
}

// Compile-time check.
var _ ports.EvaluationStore = (*RedisEvaluationStore)(nil)
