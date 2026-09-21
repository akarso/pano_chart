package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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

// getSymbolScript atomically reads hash field + at timestamp.
// KEYS[1]=sym hash KEYS[2]=at  ARGV[1]=symbol
// Returns {snap|false, at|false}.
const getSymbolScript = `
local snap = redis.call('HGET', KEYS[1], ARGV[1])
local at = redis.call('GET', KEYS[2])
return {snap, at}
`

// RedisClient is the subset of Redis operations the evaluation store needs.
// Get/HGet/MGet should surface redis.Nil as an error (shared GoRedisClient
// contract); the store maps Nil → miss locally.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	HGet(ctx context.Context, key, field string) (string, error)
	MGet(ctx context.Context, keys ...string) ([]string, error)
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

func (s *RedisEvaluationStore) mget(ctx context.Context, keys ...string) ([]string, error) {
	vals, err := s.redis.MGet(ctx, keys...)
	if err != nil {
		return nil, err
	}
	return vals, nil
}

func parseAtUnix(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, ports.ErrEvaluationNotFound
	}
	unix, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse at: %w", err)
	}
	return time.Unix(unix, 0).UTC(), nil
}

func luaString(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case []byte:
		return string(t), true
	default:
		// Redis Lua false for missing key/field.
		return "", false
	}
}

// Put implements ports.EvaluationStore.
func (s *RedisEvaluationStore) Put(ctx context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error {
	parsed, err := parseTF(tf)
	if err != nil {
		return err
	}
	stamped := stampAndDedupe(evals, parsed, computedAt)
	args, err := marshalPutArgs(stamped, computedAt.Unix(), domain.EvaluationStoreTTL(parsed))
	if err != nil {
		return err
	}
	return s.evalPut(ctx, parsed.String(), args)
}

// stampAndDedupe copies snapshots, stamps ComputedAt/Timestamp/Timeframe, and
// keeps the last entry per symbol.
func stampAndDedupe(evals []domain.EvaluationSnapshot, tf domain.Timeframe, computedAt time.Time) []domain.EvaluationSnapshot {
	atUnix := computedAt.Unix()
	bySym := make(map[string]domain.EvaluationSnapshot, len(evals))
	order := make([]string, 0, len(evals))
	for _, e := range evals {
		e.ComputedAt = atUnix
		if e.Timestamp.IsZero() {
			e.Timestamp = computedAt
		}
		e.Timeframe = tf.String()
		if _, seen := bySym[e.Symbol]; !seen {
			order = append(order, e.Symbol)
		}
		bySym[e.Symbol] = e
	}
	out := make([]domain.EvaluationSnapshot, 0, len(order))
	for _, sym := range order {
		out = append(out, bySym[sym])
	}
	return out
}

// marshalPutArgs builds Lua ARGV: arrayJSON, atUnix, ttlSec, then field/value pairs.
func marshalPutArgs(stamped []domain.EvaluationSnapshot, atUnix int64, ttl time.Duration) ([]interface{}, error) {
	arrayJSON, err := json.Marshal(stamped)
	if err != nil {
		return nil, fmt.Errorf("marshal evals: %w", err)
	}
	args := make([]interface{}, 0, 3+len(stamped)*2)
	args = append(args, string(arrayJSON), strconv.FormatInt(atUnix, 10), int(ttl.Seconds()))
	for _, e := range stamped {
		b, mErr := json.Marshal(e)
		if mErr != nil {
			return nil, fmt.Errorf("marshal symbol %s: %w", e.Symbol, mErr)
		}
		args = append(args, e.Symbol, string(b))
	}
	return args, nil
}

func (s *RedisEvaluationStore) evalPut(ctx context.Context, tf string, args []interface{}) error {
	_, err := s.redis.Eval(ctx, putEvalScript, []string{
		arrayKey(tf), atKey(tf), symbolKey(tf), symbolTmpKey(tf),
	}, args...)
	if err != nil {
		return fmt.Errorf("atomic put: %w", err)
	}
	return nil
}

// Get implements ports.EvaluationStore. Array and at are read via MGET so
// the returned timestamp matches the returned snapshot generation.
func (s *RedisEvaluationStore) Get(ctx context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error) {
	parsed, err := parseTF(tf)
	if err != nil {
		return nil, time.Time{}, err
	}
	tfStr := parsed.String()
	vals, err := s.mget(ctx, arrayKey(tfStr), atKey(tfStr))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("mget array+at: %w", err)
	}
	if len(vals) != 2 || vals[0] == "" || vals[1] == "" {
		return nil, time.Time{}, ports.ErrEvaluationNotFound
	}
	at, err := parseAtUnix(vals[1])
	if err != nil {
		return nil, time.Time{}, err
	}
	var evals []domain.EvaluationSnapshot
	if err := json.Unmarshal([]byte(vals[0]), &evals); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal evals: %w", err)
	}
	return evals, at, nil
}

// GetSymbol implements ports.EvaluationStore. Symbol hash field and at are
// read atomically via Lua so the returned time matches the snapshot.
func (s *RedisEvaluationStore) GetSymbol(ctx context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error) {
	parsed, err := parseTF(tf)
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, err
	}
	tfStr := parsed.String()
	raw, err := s.redis.Eval(ctx, getSymbolScript, []string{symbolKey(tfStr), atKey(tfStr)}, symbol)
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, fmt.Errorf("get symbol: %w", err)
	}
	pair, ok := raw.([]interface{})
	if !ok || len(pair) != 2 {
		return domain.EvaluationSnapshot{}, time.Time{}, fmt.Errorf("get symbol: unexpected reply %T", raw)
	}
	snapRaw, snapOK := luaString(pair[0])
	atRaw, atOK := luaString(pair[1])
	if !snapOK || snapRaw == "" || !atOK || atRaw == "" {
		return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
	}
	at, err := parseAtUnix(atRaw)
	if err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, err
	}
	var snap domain.EvaluationSnapshot
	if err := json.Unmarshal([]byte(snapRaw), &snap); err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, fmt.Errorf("unmarshal symbol: %w", err)
	}
	return snap, at, nil
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
