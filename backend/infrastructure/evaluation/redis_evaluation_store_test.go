package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// fakeRedis implements RedisClient. Get/HGet return redis.Nil on miss (shared
// client contract). Eval applies Put via temp-hash-then-rename semantics.
type fakeRedis struct {
	strings map[string]string
	hashes  map[string]map[string]string
	ttls    map[string]time.Duration
	nx      map[string]string

	failGet  bool
	failEval bool
	failNX   bool

	// abortAfterTemp simulates mid-script abort after temp hash is built but
	// before live keys swap — live state must remain unchanged.
	abortAfterTemp bool
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		strings: make(map[string]string),
		hashes:  make(map[string]map[string]string),
		ttls:    make(map[string]time.Duration),
		nx:      make(map[string]string),
	}
}

func (f *fakeRedis) Get(_ context.Context, key string) (string, error) {
	if f.failGet {
		return "", errors.New("redis transport error")
	}
	v, ok := f.strings[key]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (f *fakeRedis) HGet(_ context.Context, key, field string) (string, error) {
	if f.failGet {
		return "", errors.New("redis transport error")
	}
	h := f.hashes[key]
	if h == nil {
		return "", redis.Nil
	}
	v, ok := h[field]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (f *fakeRedis) MGet(_ context.Context, keys ...string) ([]string, error) {
	if f.failGet {
		return nil, errors.New("redis transport error")
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		if v, ok := f.strings[key]; ok {
			out[i] = v
		}
	}
	return out, nil
}

func (f *fakeRedis) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	if f.failNX {
		return false, errors.New("redis nx fail")
	}
	if _, ok := f.nx[key]; ok {
		return false, nil
	}
	f.nx[key] = value
	f.ttls[key] = ttl
	return true, nil
}

func (f *fakeRedis) Eval(_ context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	if f.failEval {
		return nil, errors.New("redis eval fail")
	}
	switch {
	case isReleaseLockScript(script, keys):
		return f.evalReleaseLock(keys[0], args)
	case isGetSymbolScript(script, keys):
		return f.evalGetSymbol(keys, args)
	case isPutEvalScript(keys, args):
		return f.evalPut(keys, args)
	default:
		return nil, errors.New("bad eval args")
	}
}

func isReleaseLockScript(script string, keys []string) bool {
	return len(keys) == 1 && strings.Contains(script, "GET") && strings.Contains(script, "DEL")
}

func isGetSymbolScript(script string, keys []string) bool {
	return len(keys) == 2 && strings.Contains(script, "HGET")
}

func isPutEvalScript(keys []string, args []interface{}) bool {
	return len(keys) == 4 && len(args) >= 3
}

func (f *fakeRedis) evalReleaseLock(key string, args []interface{}) (interface{}, error) {
	holder, _ := args[0].(string)
	if f.nx[key] == holder {
		delete(f.nx, key)
		delete(f.ttls, key)
		return int64(1), nil
	}
	return int64(0), nil
}

func (f *fakeRedis) evalGetSymbol(keys []string, args []interface{}) (interface{}, error) {
	if f.failGet {
		return nil, errors.New("redis transport error")
	}
	sym := ""
	if len(args) > 0 {
		sym, _ = args[0].(string)
	}
	var snap interface{}
	if h := f.hashes[keys[0]]; h != nil {
		if v, ok := h[sym]; ok {
			snap = v
		}
	}
	var at interface{}
	if v, ok := f.strings[keys[1]]; ok {
		at = v
	}
	return []interface{}{snap, at}, nil
}

func (f *fakeRedis) evalPut(keys []string, args []interface{}) (interface{}, error) {
	arrayKey, atKey, symKey, tmpKey := keys[0], keys[1], keys[2], keys[3]
	arrayJSON, _ := args[0].(string)
	atUnix, _ := args[1].(string)
	ttlSec, _ := args[2].(int)
	ttl := time.Duration(ttlSec) * time.Second

	newHash := make(map[string]string)
	for i := 3; i+1 < len(args); i += 2 {
		field, _ := args[i].(string)
		val, _ := args[i+1].(string)
		newHash[field] = val
	}

	// Build temp first (live untouched).
	delete(f.hashes, tmpKey)
	if len(newHash) > 0 {
		f.hashes[tmpKey] = newHash
		f.ttls[tmpKey] = ttl
	}
	if f.abortAfterTemp {
		return nil, errors.New("simulated mid-script abort after temp")
	}

	if len(newHash) > 0 {
		f.hashes[symKey] = newHash
		f.ttls[symKey] = ttl
		delete(f.hashes, tmpKey)
		delete(f.ttls, tmpKey)
	} else {
		delete(f.hashes, symKey)
		delete(f.ttls, symKey)
		delete(f.hashes, tmpKey)
		delete(f.ttls, tmpKey)
	}
	f.strings[arrayKey] = arrayJSON
	f.ttls[arrayKey] = ttl
	f.strings[atKey] = atUnix
	f.ttls[atKey] = ttl
	return "OK", nil
}

func sampleEvals() []domain.EvaluationSnapshot {
	return []domain.EvaluationSnapshot{
		{
			Symbol:           "BTCUSDT",
			Timeframe:        "1h",
			TrendScore:       0.8,
			SidewaysScore:    0.2,
			CompressionScore: 0.1,
			Bias:             "up",
			Price:            50000,
			ATR:              100,
			Sparkline:        []float64{100, 101, 102},
			AlgoVersion:      domain.AlgoVersion,
		},
		{
			Symbol:           "ETHUSDT",
			Timeframe:        "1h",
			TrendScore:       0.4,
			SidewaysScore:    0.6,
			CompressionScore: 0.3,
			Bias:             "neutral",
			Price:            3000,
			ATR:              20,
			Sparkline:        []float64{50, 50, 51},
			AlgoVersion:      domain.AlgoVersion,
		},
	}
}

func TestRedisEvaluationStore_RoundTrip(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()

	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, gotAt, err := store.Get(context.Background(), "1h")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !gotAt.Equal(at) {
		t.Errorf("computedAt: want %v, got %v", at, gotAt)
	}
	if len(got) != 2 {
		t.Fatalf("len: want 2, got %d", len(got))
	}
	if got[0].ComputedAt != at.Unix() {
		t.Errorf("ComputedAt: want %d, got %d", at.Unix(), got[0].ComputedAt)
	}

	raw := fr.strings[store.arrayKey("1h")]
	var probe []map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := probe[0]["trendScore"]; !ok {
		t.Errorf("expected camelCase trendScore in JSON, got %v", probe[0])
	}

	sym, _, err := store.GetSymbol(context.Background(), "1h", "ETHUSDT")
	if err != nil || sym.SidewaysScore != 0.6 {
		t.Errorf("symbol: %+v err=%v", sym, err)
	}
}

func TestRedisEvaluationStore_MissViaNil(t *testing.T) {
	store := NewRedisEvaluationStore(newFakeRedis())
	_, _, err := store.Get(context.Background(), "1h")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Errorf("Get miss: want ErrEvaluationNotFound, got %v", err)
	}
	_, _, err = store.GetSymbol(context.Background(), "1h", "BTCUSDT")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Errorf("GetSymbol miss: want ErrEvaluationNotFound, got %v", err)
	}
}

func TestRedisEvaluationStore_TransportErrorNotMiss(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	fr.failGet = true
	_, _, err := store.Get(context.Background(), "1h")
	if errors.Is(err, ports.ErrEvaluationNotFound) || err == nil {
		t.Errorf("transport error must not look like miss, got %v", err)
	}
}

func TestRedisEvaluationStore_FailedEvalLeavesPreviousIntact(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	fr.failEval = true
	err := store.Put(context.Background(), "1h", []domain.EvaluationSnapshot{sampleEvals()[0]}, at.Add(time.Minute))
	if err == nil {
		t.Fatal("expected Put failure")
	}
	got, gotAt, err := store.Get(context.Background(), "1h")
	if err != nil || !gotAt.Equal(at) || len(got) != 2 {
		t.Errorf("previous state lost: at=%v n=%d err=%v", gotAt, len(got), err)
	}
}

func TestRedisEvaluationStore_AbortAfterTempLeavesLiveIntact(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	fr.abortAfterTemp = true
	err := store.Put(context.Background(), "1h", []domain.EvaluationSnapshot{sampleEvals()[0]}, at.Add(time.Minute))
	if err == nil {
		t.Fatal("expected abort")
	}
	got, gotAt, err := store.Get(context.Background(), "1h")
	if err != nil || !gotAt.Equal(at) || len(got) != 2 {
		t.Errorf("live state must be intact after temp-only abort: at=%v n=%d err=%v", gotAt, len(got), err)
	}
	_, _, err = store.GetSymbol(context.Background(), "1h", "ETHUSDT")
	if err != nil {
		t.Errorf("ETH should remain: %v", err)
	}
}

func TestRedisEvaluationStore_EmptyPut(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Put(context.Background(), "1h", nil, at.Add(time.Minute)); err != nil {
		t.Fatalf("empty Put: %v", err)
	}
	got, gotAt, err := store.Get(context.Background(), "1h")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty array (not miss), got %d", len(got))
	}
	if gotAt.Equal(at) {
		t.Error("empty Put should refresh at")
	}
	_, _, err = store.GetSymbol(context.Background(), "1h", "BTCUSDT")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Errorf("symbols cleared: %v", err)
	}
}

func TestRedisEvaluationStore_PutReplacesSymbolHash(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.Put(context.Background(), "1h", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Put(context.Background(), "1h", []domain.EvaluationSnapshot{sampleEvals()[0]}, at.Add(time.Minute)); err != nil {
		t.Fatalf("Put2: %v", err)
	}
	_, _, err := store.GetSymbol(context.Background(), "1h", "ETHUSDT")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Errorf("ETH should be gone, got err=%v", err)
	}
}

func TestRedisEvaluationStore_DuplicateSymbolsLastWins(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Unix(1_700_000_000, 0).UTC()
	dups := []domain.EvaluationSnapshot{
		{Symbol: "BTCUSDT", TrendScore: 0.1},
		{Symbol: "BTCUSDT", TrendScore: 0.9},
	}
	if err := store.Put(context.Background(), "1h", dups, at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, _, err := store.Get(context.Background(), "1h")
	if err != nil || len(got) != 1 || got[0].TrendScore != 0.9 {
		t.Errorf("dedupe: %+v err=%v", got, err)
	}
}

func TestRedisEvaluationStore_RejectsInvalidTF(t *testing.T) {
	store := NewRedisEvaluationStore(newFakeRedis())
	if err := store.Put(context.Background(), "1h:at", sampleEvals(), time.Now()); err == nil {
		t.Fatal("expected invalid tf error")
	}
}

func TestRedisEvaluationStore_TTLUsesSharedHelper(t *testing.T) {
	fr := newFakeRedis()
	store := NewRedisEvaluationStore(fr)
	at := time.Now().UTC()
	if err := store.Put(context.Background(), "15m", sampleEvals(), at); err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := domain.EvaluationStoreTTL(domain.Timeframe15m)
	if fr.ttls[store.arrayKey("15m")] != want {
		t.Errorf("15m TTL: want %v, got %v", want, fr.ttls[store.arrayKey("15m")])
	}
	if _, err := strconv.ParseInt(fr.strings[store.atKey("15m")], 10, 64); err != nil {
		t.Errorf("at key: %v", err)
	}
}

func TestRedisEvaluationStore_TrendAlgoNamespacesKeys(t *testing.T) {
	fr := newFakeRedis()
	pred := NewRedisEvaluationStore(fr)
	pred.SetTrendAlgo("predictability")
	strength := NewRedisEvaluationStore(fr)
	strength.SetTrendAlgo("strength")
	at := time.Now().UTC()

	predEvals := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", TrendScore: 0.1, AlgoVersion: domain.AlgoVersion, TrendAlgo: "predictability",
	}}
	strengthEvals := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: domain.AlgoVersion, TrendAlgo: "strength",
	}}
	if err := pred.Put(context.Background(), "1h", predEvals, at); err != nil {
		t.Fatal(err)
	}
	if err := strength.Put(context.Background(), "1h", strengthEvals, at); err != nil {
		t.Fatal(err)
	}
	if pred.arrayKey("1h") == strength.arrayKey("1h") {
		t.Fatal("trend algos must use distinct Redis keys")
	}
	gotPred, _, err := pred.Get(context.Background(), "1h")
	if err != nil || len(gotPred) != 1 || gotPred[0].TrendScore != 0.1 {
		t.Fatalf("predictability store polluted: %+v err=%v", gotPred, err)
	}
	gotStrength, _, err := strength.Get(context.Background(), "1h")
	if err != nil || len(gotStrength) != 1 || gotStrength[0].TrendScore != 0.9 {
		t.Fatalf("strength store polluted: %+v err=%v", gotStrength, err)
	}
}

func TestRedisEvaluationStore_CompressionAlgoNamespacesKeys(t *testing.T) {
	fr := newFakeRedis()
	abs := NewRedisEvaluationStore(fr)
	abs.SetCompressionAlgo("absolute")
	pct := NewRedisEvaluationStore(fr)
	pct.SetCompressionAlgo("percentile")
	at := time.Now().UTC()

	absEvals := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", CompressionScore: 0.1, AlgoVersion: domain.AlgoVersion, CompressionAlgo: "absolute",
	}}
	pctEvals := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", CompressionScore: 0.9, AlgoVersion: domain.AlgoVersion, CompressionAlgo: "percentile",
	}}
	if err := abs.Put(context.Background(), "1h", absEvals, at); err != nil {
		t.Fatal(err)
	}
	if err := pct.Put(context.Background(), "1h", pctEvals, at); err != nil {
		t.Fatal(err)
	}
	if abs.arrayKey("1h") == pct.arrayKey("1h") {
		t.Fatal("compression algos must use distinct Redis keys")
	}
	gotAbs, _, err := abs.Get(context.Background(), "1h")
	if err != nil || len(gotAbs) != 1 || gotAbs[0].CompressionScore != 0.1 {
		t.Fatalf("absolute store polluted: %+v err=%v", gotAbs, err)
	}
	gotPct, _, err := pct.Get(context.Background(), "1h")
	if err != nil || len(gotPct) != 1 || gotPct[0].CompressionScore != 0.9 {
		t.Fatalf("percentile store polluted: %+v err=%v", gotPct, err)
	}
}

func TestRedisEvaluationStore_PredictabilityReadsLegacyKeys(t *testing.T) {
	fr := newFakeRedis()
	at := time.Unix(1_700_000_000, 0).UTC()
	legacy := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", TrendScore: 0.42, SidewaysScore: 0.1,
		AlgoVersion: domain.AlgoVersion,
	}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	fr.strings[legacyArrayKey("1h")] = string(raw)
	fr.strings[legacyAtKey("1h")] = strconv.FormatInt(at.Unix(), 10)
	symJSON, err := json.Marshal(legacy[0])
	if err != nil {
		t.Fatal(err)
	}
	fr.hashes[legacySymbolKey("1h")] = map[string]string{"BTCUSDT": string(symJSON)}

	store := NewRedisEvaluationStore(fr) // default predictability
	got, gotAt, err := store.Get(context.Background(), "1h")
	if err != nil {
		t.Fatalf("Get legacy: %v", err)
	}
	if !gotAt.Equal(at) || len(got) != 1 || got[0].TrendScore != 0.42 {
		t.Fatalf("Get legacy: at=%v got=%+v", gotAt, got)
	}
	sym, symAt, err := store.GetSymbol(context.Background(), "1h", "BTCUSDT")
	if err != nil || !symAt.Equal(at) || sym.TrendScore != 0.42 {
		t.Fatalf("GetSymbol legacy: %+v at=%v err=%v", sym, symAt, err)
	}

	// Namespaced Put must win over legacy on subsequent reads.
	migrated := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", TrendScore: 0.77, AlgoVersion: domain.AlgoVersion, TrendAlgo: "predictability",
	}}
	if err := store.Put(context.Background(), "1h", migrated, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _, err = store.Get(context.Background(), "1h")
	if err != nil || len(got) != 1 || got[0].TrendScore != 0.77 {
		t.Fatalf("namespaced must prefer over legacy: %+v err=%v", got, err)
	}

	// Strength must not read predictability legacy keys.
	strength := NewRedisEvaluationStore(fr)
	strength.SetTrendAlgo("strength")
	_, _, err = strength.Get(context.Background(), "1h")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Fatalf("strength must not fall back to legacy, got %v", err)
	}
}

func TestRedisEvaluationStore_RemovedSymbolDoesNotResurrectLegacy(t *testing.T) {
	fr := newFakeRedis()
	at := time.Unix(1_700_000_000, 0).UTC()
	legacySnap := domain.EvaluationSnapshot{
		Symbol: "OLDUSDT", TrendScore: 0.9, AlgoVersion: domain.AlgoVersion,
	}
	legacyJSON, err := json.Marshal(legacySnap)
	if err != nil {
		t.Fatal(err)
	}
	fr.strings[legacyAtKey("1h")] = strconv.FormatInt(at.Unix(), 10)
	fr.hashes[legacySymbolKey("1h")] = map[string]string{"OLDUSDT": string(legacyJSON)}

	store := NewRedisEvaluationStore(fr)
	// Current predictability batch exists but does not include OLDUSDT.
	current := []domain.EvaluationSnapshot{{
		Symbol: "BTCUSDT", TrendScore: 0.5, AlgoVersion: domain.AlgoVersion, TrendAlgo: "predictability",
	}}
	if err := store.Put(context.Background(), "1h", current, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	_, _, err = store.GetSymbol(context.Background(), "1h", "OLDUSDT")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Fatalf("removed symbol must stay a miss, got %v", err)
	}
	// Empty current batch: still a generation — must not resurrect legacy.
	if err := store.Put(context.Background(), "1h", nil, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.GetSymbol(context.Background(), "1h", "OLDUSDT")
	if !errors.Is(err, ports.ErrEvaluationNotFound) {
		t.Fatalf("empty batch must not resurrect legacy, got %v", err)
	}
}

func TestRedisRefreshLock_AcquireRelease(t *testing.T) {
	fr := newFakeRedis()
	lock := NewRedisRefreshLock(fr)
	ok, err := lock.TryAcquire(context.Background(), "eval:refresh:1h", time.Minute, "a")
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	ok, err = lock.TryAcquire(context.Background(), "eval:refresh:1h", time.Minute, "b")
	if err != nil || ok {
		t.Fatalf("second acquire should fail: ok=%v err=%v", ok, err)
	}
	if err := lock.Release(context.Background(), "eval:refresh:1h", "b"); err != nil {
		t.Fatalf("wrong holder release: %v", err)
	}
	// Wrong holder must not clear.
	if _, held := fr.nx["eval:refresh:1h"]; !held {
		t.Fatal("wrong holder must not release")
	}
	if err := lock.Release(context.Background(), "eval:refresh:1h", "a"); err != nil {
		t.Fatalf("release: %v", err)
	}
	ok, err = lock.TryAcquire(context.Background(), "eval:refresh:1h", time.Minute, "b")
	if err != nil || !ok {
		t.Fatalf("re-acquire after release: ok=%v err=%v", ok, err)
	}
}
