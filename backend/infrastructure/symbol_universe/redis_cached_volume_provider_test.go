package symbol_universe

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeRedisVolume struct {
	store map[string]string
	fail  bool
}

func (f *fakeRedisVolume) Get(ctx context.Context, key string) (string, error) {
	if f.fail {
		return "", errors.New("redis fail")
	}
	return f.store[key], nil
}
func (f *fakeRedisVolume) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if f.fail {
		return errors.New("redis fail")
	}
	f.store[key] = value
	return nil
}

type fakeVolumeProvider struct {
	m     map[string]float64
	err   error
	called int
}

func (f *fakeVolumeProvider) Volumes(ctx context.Context) (map[string]float64, error) {
	f.called++
	return f.m, f.err
}

func TestRedisCachedVolumeProvider_ReturnsCachedVolumesWhenPresent(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}}
	vols := map[string]float64{"BTCUSDT": 123.4, "ETHUSDT": 567.8}
	b, _ := json.Marshal(vols)
	fr.store["key"] = string(b)
	prov := &fakeVolumeProvider{m: nil}
	cache := NewRedisCachedVolumeProvider(prov, fr, time.Minute, "key")
	out, err := cache.Volumes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out["BTCUSDT"] != 123.4 || out["ETHUSDT"] != 567.8 {
		t.Errorf("unexpected volumes: %+v", out)
	}
	if prov.called != 0 {
		t.Errorf("provider should not be called on cache hit")
	}
}

func TestRedisCachedVolumeProvider_CallsNextOnCacheMiss(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}}
	prov := &fakeVolumeProvider{m: map[string]float64{"BTCUSDT": 42.0}}
	cache := NewRedisCachedVolumeProvider(prov, fr, time.Minute, "key")
	out, err := cache.Volumes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out["BTCUSDT"] != 42.0 {
		t.Errorf("unexpected volumes: %+v", out)
	}
	if prov.called != 1 {
		t.Errorf("provider should be called on cache miss")
	}
}

func TestRedisCachedVolumeProvider_StoresValueWithTTL(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}}
	prov := &fakeVolumeProvider{m: map[string]float64{"BTCUSDT": 1.0}}
	cache := NewRedisCachedVolumeProvider(prov, fr, 42*time.Second, "key")
	_, _ = cache.Volumes(context.Background())
	if _, ok := fr.store["key"]; !ok {
		t.Errorf("should store value in redis")
	}
}

func TestRedisCachedVolumeProvider_FallsBackOnRedisFailure(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}, fail: true}
	prov := &fakeVolumeProvider{m: map[string]float64{"BTCUSDT": 2.0}}
	cache := NewRedisCachedVolumeProvider(prov, fr, time.Minute, "key")
	out, err := cache.Volumes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out["BTCUSDT"] != 2.0 {
		t.Errorf("unexpected volumes: %+v", out)
	}
}

func TestRedisCachedVolumeProvider_DoesNotCacheWhenNextFails(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}}
	prov := &fakeVolumeProvider{err: errors.New("fail")}
	cache := NewRedisCachedVolumeProvider(prov, fr, time.Minute, "key")
	_, err := cache.Volumes(context.Background())
	if err == nil {
		t.Fatal("expected error from provider")
	}
	if _, ok := fr.store["key"]; ok {
		t.Errorf("should not cache on provider error")
	}
}

func TestRedisCachedVolumeProvider_DeterministicDeserialization(t *testing.T) {
	fr := &fakeRedisVolume{store: map[string]string{}}
	vols := map[string]float64{"BTCUSDT": 1.1, "ETHUSDT": 2.2}
	b, _ := json.Marshal(vols)
	fr.store["key"] = string(b)
	prov := &fakeVolumeProvider{m: nil}
	cache := NewRedisCachedVolumeProvider(prov, fr, time.Minute, "key")
	out1, err1 := cache.Volumes(context.Background())
	out2, err2 := cache.Volumes(context.Background())
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v %v", err1, err2)
	}
	if len(out1) != len(out2) || out1["BTCUSDT"] != out2["BTCUSDT"] || out1["ETHUSDT"] != out2["ETHUSDT"] {
		t.Errorf("deserialization not deterministic: %v %v", out1, out2)
	}
}
