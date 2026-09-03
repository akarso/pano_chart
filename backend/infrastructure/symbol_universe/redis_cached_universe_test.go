package symbol_universe

import (
	"context"
	"encoding/json"
	"errors"
	"pano_chart/backend/domain"
	"testing"
	"time"
)

type fakeRedis struct {
	store map[string]string
	fail  bool
}

func (f *fakeRedis) Get(ctx context.Context, key string) (string, error) {
	if f.fail {
		return "", errors.New("redis fail")
	}
	return f.store[key], nil
}
func (f *fakeRedis) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if f.fail {
		return errors.New("redis fail")
	}
	f.store[key] = value
	return nil
}

type fakeProvider struct {
	syms   []domain.Symbol
	err    error
	called int
}

func (f *fakeProvider) Symbols(ctx context.Context, exchangeInfoURL, tickerURL string) ([]domain.Symbol, error) {
	f.called++
	return f.syms, f.err
}

func TestRedisCachedSymbolUniverse_ReturnsCachedValueWhenPresent(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	syms := []string{"BTCUSDT", "ETHUSDT"}
	b, _ := json.Marshal(syms)
	fr.store["key"] = string(b)
	prov := &fakeProvider{syms: nil}
	cache := NewRedisCachedSymbolUniverse(prov, fr, time.Minute, "key")
	out, err := cache.Symbols(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0].String() != "BTCUSDT" || out[1].String() != "ETHUSDT" {
		t.Errorf("unexpected symbols: %+v", out)
	}
	if prov.called != 0 {
		t.Errorf("provider should not be called on cache hit")
	}
}

func TestRedisCachedSymbolUniverse_CallsNextOnCacheMiss(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	prov := &fakeProvider{syms: []domain.Symbol{domain.NewSymbolUnsafe("BTCUSDT")}}
	cache := NewRedisCachedSymbolUniverse(prov, fr, time.Minute, "key")
	out, err := cache.Symbols(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].String() != "BTCUSDT" {
		t.Errorf("unexpected symbols: %+v", out)
	}
	if prov.called != 1 {
		t.Errorf("provider should be called on cache miss")
	}
}

func TestRedisCachedSymbolUniverse_StoresValueWithTTL(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	prov := &fakeProvider{syms: []domain.Symbol{domain.NewSymbolUnsafe("BTCUSDT")}}
	cache := NewRedisCachedSymbolUniverse(prov, fr, 42*time.Second, "key")
	_, _ = cache.Symbols(context.Background(), "", "")
	if _, ok := fr.store["key"]; !ok {
		t.Errorf("should store value in redis")
	}
}

func TestRedisCachedSymbolUniverse_FallsBackWhenRedisFails(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}, fail: true}
	prov := &fakeProvider{syms: []domain.Symbol{domain.NewSymbolUnsafe("BTCUSDT")}}
	cache := NewRedisCachedSymbolUniverse(prov, fr, time.Minute, "key")
	out, err := cache.Symbols(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].String() != "BTCUSDT" {
		t.Errorf("unexpected symbols: %+v", out)
	}
}

func TestRedisCachedSymbolUniverse_DoesNotCacheOnNextError(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	prov := &fakeProvider{err: errors.New("fail")}
	cache := NewRedisCachedSymbolUniverse(prov, fr, time.Minute, "key")
	_, err := cache.Symbols(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error from provider")
	}
	if _, ok := fr.store["key"]; ok {
		t.Errorf("should not cache on provider error")
	}
}
