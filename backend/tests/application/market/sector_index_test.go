package market_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	infmarket "pano_chart/backend/infrastructure/market"
)

func writeTempSectorsYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sectors.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newSectorSvc(
	t *testing.T,
	provider metrics.CandleProvider,
	yamlBody string,
) *metrics.SectorIndexService {
	t.Helper()
	path := writeTempSectorsYAML(t, yamlBody)
	cat, err := metrics.LoadSectorCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	comp := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	return metrics.NewSectorIndexService(comp, cat)
}

func TestSectorCatalog_RejectsDuplicateSymbol(t *testing.T) {
	path := writeTempSectorsYAML(t, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT]
  - id: defi
    name: DeFi
    symbols: [BTCUSDT]
`)
	if _, err := metrics.LoadSectorCatalog(path); err == nil {
		t.Fatal("expected duplicate symbol error")
	}
}

func TestSectorCatalog_RejectsDuplicateID(t *testing.T) {
	path := writeTempSectorsYAML(t, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT]
  - id: l1
    name: Again
    symbols: [ETHUSDT]
`)
	if _, err := metrics.LoadSectorCatalog(path); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestSectorCatalog_RejectsReservedOther(t *testing.T) {
	path := writeTempSectorsYAML(t, `
sectors:
  - id: other
    name: Other
    symbols: [BTCUSDT]
`)
	if _, err := metrics.LoadSectorCatalog(path); err == nil {
		t.Fatal("expected reserved id error")
	}
}

func TestSectorCatalog_NormalizesIDAndRejectsPathChars(t *testing.T) {
	path := writeTempSectorsYAML(t, `
sectors:
  - id: L1
    name: Layer 1
    symbols: [BTCUSDT]
`)
	cat, err := metrics.LoadSectorCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if cat.ForSymbol("BTCUSDT") != "l1" {
		t.Fatalf("id=%q want l1", cat.ForSymbol("BTCUSDT"))
	}

	bad := writeTempSectorsYAML(t, `
sectors:
  - id: ../evil
    name: Evil
    symbols: [ETHUSDT]
`)
	if _, err := metrics.LoadSectorCatalog(bad); err == nil {
		t.Fatal("expected invalid sector id error")
	}
}

func TestSectorCatalog_RejectsEmpty(t *testing.T) {
	path := writeTempSectorsYAML(t, `sectors: []`)
	if _, err := metrics.LoadSectorCatalog(path); err == nil {
		t.Fatal("expected empty catalog error")
	}
}

func TestLoadSectorCatalog_MissingFileIsNotExist(t *testing.T) {
	_, err := metrics.LoadSectorCatalog(filepath.Join(t.TempDir(), "missing-sectors.yaml"))
	if err == nil {
		t.Fatal("expected missing-file error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist wrap, got %v", err)
	}
}

func TestSectorCatalog_ForSymbolOther(t *testing.T) {
	path := writeTempSectorsYAML(t, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT, ETHUSDT]
`)
	cat, err := metrics.LoadSectorCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if cat.ForSymbol("BTCUSDT") != "l1" || cat.ForSymbol("DOGEUSDT") != "other" {
		t.Fatal(cat.ForSymbol("BTCUSDT"), cat.ForSymbol("DOGEUSDT"))
	}
}

func TestSectorIndex_TwoSymbolsSymbolCount(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
				{symbol: btc, timeframe: tf, ts: ts4h(2), close: 121},
			},
			"ETHUSDT:4h": {
				{symbol: eth, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: eth, timeframe: tf, ts: ts4h(1), close: 105},
				{symbol: eth, timeframe: tf, ts: ts4h(2), close: 110},
			},
		},
	}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT, ETHUSDT]
`)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sectors) != 1 || result.Sectors[0].SymbolCount != 2 {
		t.Fatalf("%+v", result.Sectors)
	}
	if !result.Sectors[0].RSAvailable {
		t.Fatal("expected rsAvailable")
	}
	// Sector == whole market (2/2 symbols) → RS vs itself must be ≈0.
	if math.Abs(result.Sectors[0].RS) > 1e-9 {
		t.Fatalf("sector spanning entire market should have RS≈0, got %g", result.Sectors[0].RS)
	}
}

func TestSectorIndex_AlignedWindowYoungSectorRSNearZero(t *testing.T) {
	old := makeSymbol2("OLDUSDT")
	young := makeSymbol2("YOUNGUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{old, young},
		candles: map[string][]fakeCandle{
			"OLDUSDT:4h": {
				{symbol: old, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: old, timeframe: tf, ts: ts4h(1), close: 110},
				{symbol: old, timeframe: tf, ts: ts4h(2), close: 110},
				{symbol: old, timeframe: tf, ts: ts4h(3), close: 110},
				{symbol: old, timeframe: tf, ts: ts4h(4), close: 110},
			},
			"YOUNGUSDT:4h": {
				{symbol: young, timeframe: tf, ts: ts4h(2), close: 50},
				{symbol: young, timeframe: tf, ts: ts4h(3), close: 50},
				{symbol: young, timeframe: tf, ts: ts4h(4), close: 50},
			},
		},
	}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: old
    name: Old
    symbols: [OLDUSDT]
  - id: young
    name: Young
    symbols: [YOUNGUSDT]
`).WithMinSymbols(1)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	var youngSec *mkt.SectorIndex
	for i := range result.Sectors {
		if result.Sectors[i].ID == "young" {
			youngSec = &result.Sectors[i]
		}
	}
	if youngSec == nil {
		t.Fatalf("missing young: %+v", result.Sectors)
		return
	}
	if !youngSec.RSAvailable {
		t.Fatal("young RS should be available on shared bars")
	}
	if math.Abs(youngSec.RS) > 1e-9 {
		t.Fatalf("young RS=%g want ≈0", youngSec.RS)
	}
}

func TestSectorIndex_HandComputedRS(t *testing.T) {
	// Single-symbol sectors vs volume-weighted market.
	// fakeCandleProvider uses volume=1000 on every candle (NewCandleUnsafe in
	// composite_index_test.go); quote-vol weight = Σ volume×close over the window.
	strong := makeSymbol2("STRONGUSDT")
	weak := makeSymbol2("WEAKUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{strong, weak},
		candles: map[string][]fakeCandle{
			"STRONGUSDT:4h": {
				{symbol: strong, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: strong, timeframe: tf, ts: ts4h(1), close: 150},
			},
			"WEAKUSDT:4h": {
				{symbol: weak, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: weak, timeframe: tf, ts: ts4h(1), close: 90},
			},
		},
	}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: strong
    name: Strong
    symbols: [STRONGUSDT]
  - id: weak
    name: Weak
    symbols: [WEAKUSDT]
`).WithMinSymbols(1)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]mkt.SectorIndex{}
	for _, s := range result.Sectors {
		byID[s.ID] = s
	}
	rStrong := math.Log(1.5)
	rWeak := math.Log(0.9)
	wStrong := 1000.0*100 + 1000.0*150
	wWeak := 1000.0*100 + 1000.0*90
	marketRet := (rStrong*wStrong + rWeak*wWeak) / (wStrong + wWeak)
	wantStrong := rStrong - marketRet
	wantWeak := rWeak - marketRet
	if math.Abs(byID["strong"].RS-wantStrong) > 1e-6 {
		t.Fatalf("strong RS=%g want %g", byID["strong"].RS, wantStrong)
	}
	if math.Abs(byID["weak"].RS-wantWeak) > 1e-6 {
		t.Fatalf("weak RS=%g want %g", byID["weak"].RS, wantWeak)
	}
	if result.MarketSymbolCount != 2 {
		t.Fatalf("marketSymbolCount=%d", result.MarketSymbolCount)
	}
}

func TestSectorIndex_CandleSkewClampsAndCaches(t *testing.T) {
	// 2 fresh symbols have one extra bar that rallies hard; 4 stale ones do not.
	// Market ref ends before the new bar (≥50% coverage). points/return/rs must
	// share the clamped overlap — not the trailing fresh bar.
	tf := makeTimeframe2("4h")
	freshA := makeSymbol2("FASTAUSDT")
	freshB := makeSymbol2("FASTBUSDT")
	candles := map[string][]fakeCandle{}
	syms := []domain.Symbol{freshA, freshB}
	for _, s := range []domain.Symbol{freshA, freshB} {
		candles[s.String()+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(0), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(1), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(2), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(3), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(4), close: 150}, // +50% on unmatched bar
		}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "STALEUSDT"
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[s.String()+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(0), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(1), close: 110},
			{symbol: s, timeframe: tf, ts: ts4h(2), close: 110},
			{symbol: s, timeframe: tf, ts: ts4h(3), close: 110},
		}
	}
	provider := &fakeCandleProvider{symbols: syms, candles: candles}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: fast
    name: Fast
    symbols: [FASTAUSDT, FASTBUSDT]
  - id: slow
    name: Slow
    symbols: [ASTALEUSDT, BSTALEUSDT, CSTALEUSDT, DSTALEUSDT]
`)
	redis := &memRedis{}
	cached := infmarket.NewRedisCachedSectors(svc, redis, time.Minute, "skew_sectors")

	r1, err := cached.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	var fast *mkt.SectorIndex
	for i := range r1.Sectors {
		if r1.Sectors[i].ID == "fast" {
			fast = &r1.Sectors[i]
		}
	}
	if fast == nil || !fast.RSAvailable {
		t.Fatalf("fast should be available via clamp: %+v", r1.Sectors)
	}
	if len(fast.Points) == 0 {
		t.Fatal("expected clamped points")
	}
	lastTS := fast.Points[len(fast.Points)-1].Timestamp
	if lastTS != ts4h(3).Unix() {
		t.Fatalf("points must end on market overlap (t3), got ts=%d want %d (len=%d)",
			lastTS, ts4h(3).Unix(), len(fast.Points))
	}
	if math.Abs(fast.Points[0].Value-100) > 1e-9 {
		t.Fatalf("clamped points must rebase to 100, got %g", fast.Points[0].Value)
	}
	fromPts := math.Log(fast.Points[len(fast.Points)-1].Value / fast.Points[0].Value)
	if math.Abs(fromPts-fast.Return) > 1e-12 {
		t.Fatalf("points imply return %g but Return=%g", fromPts, fast.Return)
	}
	// Overlap is flat for fast members; trailing +50% bar must not enter Return.
	if math.Abs(fast.Return) > 1e-9 {
		t.Fatalf("clamped return should be ~0, got %g (rs=%g)", fast.Return, fast.RS)
	}
	// Unclamped series would read ln(150/100) ≈ 0.405 — prove we did not publish that.
	if math.Abs(fast.Return-math.Log(1.5)) < 0.01 {
		t.Fatal("return looks like the unmatched trailing bar was included")
	}
	if len(redis.store) == 0 {
		t.Fatal("expected cache write after successful calculate")
	}
	r2, err := cached.Calculate(context.Background(), "4h", 200)
	if err != nil || len(r2.Sectors) == 0 {
		t.Fatalf("cache hit failed: %v %+v", err, r2)
	}
}

func TestSectorIndex_SkipsBelowMinSymbols(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 101},
			},
		},
	}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT]
`)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sectors) != 0 {
		t.Fatalf("expected empty, got %+v", result.Sectors)
	}
}

func TestSectorIndex_SortedByRSDescending(t *testing.T) {
	strong := makeSymbol2("STRONGUSDT")
	weak := makeSymbol2("WEAKUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{strong, weak},
		candles: map[string][]fakeCandle{
			"STRONGUSDT:4h": {
				{symbol: strong, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: strong, timeframe: tf, ts: ts4h(1), close: 150},
			},
			"WEAKUSDT:4h": {
				{symbol: weak, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: weak, timeframe: tf, ts: ts4h(1), close: 90},
			},
		},
	}
	svc := newSectorSvc(t, provider, `
sectors:
  - id: weak
    name: Weak
    symbols: [WEAKUSDT]
  - id: strong
    name: Strong
    symbols: [STRONGUSDT]
`).WithMinSymbols(1)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sectors[0].ID != "strong" {
		t.Fatalf("first=%s", result.Sectors[0].ID)
	}
}

func TestSectorIndex_ExcludesStablesFromOther(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	usdc := makeSymbol2("USDCUSDT")
	orphan := makeSymbol2("ORPHANUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, usdc, orphan},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"USDCUSDT:4h": {
				{symbol: usdc, timeframe: tf, ts: ts4h(0), close: 1},
				{symbol: usdc, timeframe: tf, ts: ts4h(1), close: 1},
			},
			"ORPHANUSDT:4h": {
				{symbol: orphan, timeframe: tf, ts: ts4h(0), close: 10},
				{symbol: orphan, timeframe: tf, ts: ts4h(1), close: 11},
			},
		},
	}
	path := writeTempSectorsYAML(t, `
sectors:
  - id: l1
    name: Layer 1
    symbols: [BTCUSDT]
`)
	cat, err := metrics.LoadSectorCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	filter := metrics.DefaultSymbolFilter()
	comp := metrics.NewCompositeIndexServiceFiltered(provider, 4, filter)
	svc := metrics.NewSectorIndexService(comp, cat).WithMinSymbols(1).WithIncludeOther(true)
	result, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range result.Sectors {
		if sec.ID == "other" && sec.SymbolCount != 1 {
			t.Fatalf("other SymbolCount=%d", sec.SymbolCount)
		}
	}
}

type fakeSectorsCalc struct {
	result mkt.SectorIndexResult
	err    error
	calls  int32
}

func (f *fakeSectorsCalc) Calculate(_ context.Context, timeframe string, limit int) (mkt.SectorIndexResult, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.err != nil {
		return mkt.SectorIndexResult{}, f.err
	}
	out := f.result
	if out.Timeframe == "" {
		out.Timeframe = timeframe
	}
	_ = limit
	return out, nil
}

type memRedis struct {
	mu    sync.Mutex
	store map[string]string
}

func (m *memRedis) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return "", nil
	}
	return m.store[key], nil
}

func (m *memRedis) Set(_ context.Context, key string, value string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		m.store = map[string]string{}
	}
	m.store[key] = value
	return nil
}

func TestRedisCachedSectors_DoesNotCacheEmpty(t *testing.T) {
	redis := &memRedis{}
	next := &fakeSectorsCalc{result: mkt.SectorIndexResult{Timeframe: "4h"}}
	cached := infmarket.NewRedisCachedSectors(next, redis, time.Minute, "test_sectors")
	_, _ = cached.Calculate(context.Background(), "4h", 200)
	if len(redis.store) != 0 {
		t.Fatalf("empty must not cache: %v", redis.store)
	}
}

func TestRedisCachedSectors_CachesUnavailable(t *testing.T) {
	redis := &memRedis{}
	next := &fakeSectorsCalc{
		result: mkt.SectorIndexResult{
			Timeframe: "4h",
			Sectors: []mkt.SectorIndex{
				{ID: "l1", Name: "L1", SymbolCount: 2, RSAvailable: false},
			},
		},
	}
	cached := infmarket.NewRedisCachedSectors(next, redis, time.Minute, "test_sectors")
	_, _ = cached.Calculate(context.Background(), "4h", 200)
	if len(redis.store) == 0 {
		t.Fatal("rs-unavailable should still cache")
	}
	_, _ = cached.Calculate(context.Background(), "4h", 200)
	if atomic.LoadInt32(&next.calls) != 1 {
		t.Fatalf("expected cache hit, calls=%d", next.calls)
	}
}

func TestRedisCachedSectors_HitMiss(t *testing.T) {
	redis := &memRedis{}
	next := &fakeSectorsCalc{
		result: mkt.SectorIndexResult{
			Timeframe:         "4h",
			MarketSymbolCount: 10,
			Sectors: []mkt.SectorIndex{
				{ID: "l1", Name: "Layer 1", SymbolCount: 2, Return: 0.01, RS: 0.02, RSAvailable: true},
			},
		},
	}
	cached := infmarket.NewRedisCachedSectors(next, redis, time.Minute, "test_sectors")
	r1, err := cached.Calculate(context.Background(), "4h", 200)
	if err != nil || atomic.LoadInt32(&next.calls) != 1 || !r1.Sectors[0].RSAvailable {
		t.Fatalf("miss: calls=%d err=%v %+v", next.calls, err, r1)
	}
	if r1.MarketSymbolCount != 10 {
		t.Fatalf("marketSymbolCount=%d", r1.MarketSymbolCount)
	}
	_, err = cached.Calculate(context.Background(), "4h", 200)
	if err != nil || atomic.LoadInt32(&next.calls) != 1 {
		t.Fatalf("hit: calls=%d", next.calls)
	}
}

func TestRedisCachedSectors_TTLForTimeframe(t *testing.T) {
	cached := infmarket.NewRedisCachedSectors(&fakeSectorsCalc{}, &memRedis{}, 3*time.Minute, "t")
	if got := cached.TTLForTimeframe("1m"); got != 30*time.Second {
		t.Fatalf("1m ttl=%v", got)
	}
	if got := cached.TTLForTimeframe("4h"); got != 3*time.Minute {
		t.Fatalf("4h ttl=%v", got)
	}
}

func TestSectorsHandler_DefaultParams(t *testing.T) {
	calc := &fakeSectorsCalc{
		result: mkt.SectorIndexResult{
			Timeframe:         "4h",
			MarketSymbolCount: 42,
			Sectors: []mkt.SectorIndex{
				{
					ID: "l1", Name: "Layer 1", SymbolCount: 2,
					Points: []mkt.IndexPoint{
						{Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), Value: 100.123},
						{Timestamp: time.Date(2025, 1, 1, 4, 0, 0, 0, time.UTC).Unix(), Value: 101.456},
					},
					Return: 0.01234567, RS: 0.00123456, RSAvailable: true,
				},
			},
		},
	}
	h := adhttp.NewMarketSectorsHandler(calc)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["timeframe"] != "4h" {
		t.Fatalf("timeframe=%v", body["timeframe"])
	}
	if body["marketSymbolCount"].(float64) != 42 {
		t.Fatalf("marketSymbolCount=%v", body["marketSymbolCount"])
	}
	sec := body["sectors"].([]interface{})[0].(map[string]interface{})
	if sec["id"] != "l1" || sec["symbolCount"].(float64) != 2 || sec["rsAvailable"] != true {
		t.Fatalf("sec=%v", sec)
	}
	v0 := sec["points"].([]interface{})[0].(map[string]interface{})["v"].(float64)
	if v0 != 100.123 {
		t.Fatalf("rounded v=%v", v0)
	}
}

func TestSectorsHandler_InvalidTimeframe(t *testing.T) {
	h := adhttp.NewMarketSectorsHandler(&fakeSectorsCalc{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors?timeframe=xyz", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["error"] != "invalid timeframe" {
		t.Fatalf("%v", body)
	}
}

type capturingSectorsCalc struct {
	tf    string
	limit int
}

func (c *capturingSectorsCalc) Calculate(_ context.Context, timeframe string, limit int) (mkt.SectorIndexResult, error) {
	c.tf = timeframe
	c.limit = limit
	return mkt.SectorIndexResult{
		Timeframe: timeframe,
		Sectors:   []mkt.SectorIndex{{ID: "l1", Name: "L1", SymbolCount: 2, RSAvailable: true}},
	}, nil
}

func TestSectorsHandler_NormalizesTimeframeCacheKey(t *testing.T) {
	cap := &capturingSectorsCalc{}
	h := adhttp.NewMarketSectorsHandler(cap)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors?timeframe=4H", nil))
	if rr.Code != 200 || cap.tf != "4h" {
		t.Fatalf("status=%d tf=%q", rr.Code, cap.tf)
	}
}

func TestSectorsHandler_LimitClamping(t *testing.T) {
	cap := &capturingSectorsCalc{}
	h := adhttp.NewMarketSectorsHandler(cap)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors?limit=501", nil))
	if rr.Code != 200 || cap.limit != 500 {
		t.Fatalf("clamp status=%d limit=%d", rr.Code, cap.limit)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors?limit=0", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("limit=0 → %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors?limit=abc", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("limit=abc → %d", rr.Code)
	}
}

func TestSectorsHandler_Error(t *testing.T) {
	h := adhttp.NewMarketSectorsHandler(&fakeSectorsCalc{err: context.DeadlineExceeded})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/market/sectors", nil))
	if rr.Code != 500 {
		t.Fatalf("status %d", rr.Code)
	}
}
