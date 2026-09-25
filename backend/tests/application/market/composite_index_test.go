package market_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake CandleProvider ---

type fakeCandle struct {
	symbol    domain.Symbol
	timeframe domain.Timeframe
	ts        time.Time
	close     float64
}

type fakeCandleProvider struct {
	symbols []domain.Symbol
	candles map[string][]fakeCandle
	err     error
}

func (f *fakeCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}

func (f *fakeCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	fcs, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("no data for %s", key)
	}
	candles := make([]domain.Candle, 0, len(fcs))
	for _, fc := range fcs {
		c := domain.NewCandleUnsafe(fc.symbol, fc.timeframe, fc.ts, fc.close, fc.close, fc.close, fc.close, 1000)
		candles = append(candles, c)
	}
	if n < len(candles) {
		candles = candles[len(candles)-n:]
	}
	return domain.NewCandleSeries(sym, tf, candles)
}

func makeSymbol2(s string) domain.Symbol {
	sym, _ := domain.NewSymbol(s)
	return sym
}

func makeTimeframe2(s string) domain.Timeframe {
	tf, _ := domain.NewTimeframe(s)
	return tf
}

func ts4h(idx int) time.Time {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(idx) * 4 * time.Hour)
}

// --- Composite Index Service Tests ---

func TestCompositeIndex_SingleSymbol(t *testing.T) {
	sym := makeSymbol2("BTCUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: sym, timeframe: tf, ts: ts4h(0), close: 50000},
				{symbol: sym, timeframe: tf, ts: ts4h(1), close: 51000},
				{symbol: sym, timeframe: tf, ts: ts4h(2), close: 49000},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Errorf("expected 1 symbol, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(idx.Points))
	}
	if idx.Points[0].Value != 100 {
		t.Errorf("expected first value 100, got %.2f", idx.Points[0].Value)
	}
	if math.Abs(idx.Points[1].Value-102) > 0.01 {
		t.Errorf("expected ~102, got %.2f", idx.Points[1].Value)
	}
	if math.Abs(idx.Points[2].Value-98) > 0.01 {
		t.Errorf("expected ~98, got %.2f", idx.Points[2].Value)
	}
}

func TestCompositeIndex_MedianOfThree(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	sol := makeSymbol2("SOLUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth, sol},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"ETHUSDT:4h": {
				{symbol: eth, timeframe: tf, ts: ts4h(0), close: 200},
				{symbol: eth, timeframe: tf, ts: ts4h(1), close: 200},
			},
			"SOLUSDT:4h": {
				{symbol: sol, timeframe: tf, ts: ts4h(0), close: 50},
				{symbol: sol, timeframe: tf, ts: ts4h(1), close: 52.5},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 3 {
		t.Errorf("expected 3 symbols, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(idx.Points))
	}
	if idx.Points[0].Value != 100 {
		t.Errorf("expected first value 100, got %.2f", idx.Points[0].Value)
	}
	// Median of [100, 105, 110] = 105
	if math.Abs(idx.Points[1].Value-105) > 0.01 {
		t.Errorf("expected median ~105, got %.2f", idx.Points[1].Value)
	}
}

func TestCompositeIndex_EmptyUniverse(t *testing.T) {
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{},
		candles: map[string][]fakeCandle{},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 0 {
		t.Errorf("expected 0, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 0 {
		t.Errorf("expected 0 points, got %d", len(idx.Points))
	}
}

func TestCompositeIndex_SkipsFailedSymbols(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 105},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Errorf("expected 1 contributing symbol, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 2 {
		t.Errorf("expected 2 points, got %d", len(idx.Points))
	}
}

func TestCompositeIndex_InvalidTimeframe(t *testing.T) {
	provider := &fakeCandleProvider{symbols: []domain.Symbol{}}
	svc := metrics.NewCompositeIndexService(provider, 4)
	_, err := svc.Calculate(context.Background(), "invalid", 200)
	if err == nil {
		t.Error("expected error for invalid timeframe")
	}
}

func TestCompositeIndex_DefaultLimit(t *testing.T) {
	provider := &fakeCandleProvider{symbols: []domain.Symbol{}}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "1h", 0)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", idx.Timeframe)
	}
}

func TestCompositeIndex_MedianEvenCount(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"ETHUSDT:4h": {
				{symbol: eth, timeframe: tf, ts: ts4h(0), close: 200},
				{symbol: eth, timeframe: tf, ts: ts4h(1), close: 200},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	// PR-095: median of log-returns → 100 * sqrt(1.1) ≈ 104.88 (not arithmetic 105).
	want := 100 * math.Sqrt(1.1)
	if math.Abs(idx.Points[1].Value-want) > 0.01 {
		t.Errorf("expected median ~%.2f, got %.2f", want, idx.Points[1].Value)
	}
}

func TestCompositeIndex_TimestampAlignmentMissingBar(t *testing.T) {
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{a, b},
		candles: map[string][]fakeCandle{
			"AAAUSDT:4h": {
				{symbol: a, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: a, timeframe: tf, ts: ts4h(1), close: 110},
				{symbol: a, timeframe: tf, ts: ts4h(2), close: 120},
			},
			"BBBUSDT:4h": {
				{symbol: b, timeframe: tf, ts: ts4h(0), close: 100},
				// missing ts4h(1)
				{symbol: b, timeframe: tf, ts: ts4h(2), close: 100},
			},
		},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Points) != 3 {
		t.Fatalf("expected 3 aligned bars (including sparse middle), got %d", len(idx.Points))
	}
	if idx.Points[1].Timestamp != ts4h(1).Unix() {
		t.Errorf("middle bar timestamp = %d, want %d", idx.Points[1].Timestamp, ts4h(1).Unix())
	}
	// Middle bar: only AAA (prev=100→110).
	if math.Abs(idx.Points[1].Value-110) > 0.01 {
		t.Errorf("middle value = %.2f, want 110 (only AAA)", idx.Points[1].Value)
	}
	// t2: AAA ln(120/110) and BBB ln(100/100)=0 via last print before t2.
	wantT2 := 110 * math.Sqrt(120.0/110.0)
	if math.Abs(idx.Points[2].Value-wantT2) > 0.01 {
		t.Errorf("t2 = %.4f, want %.4f (BBB flat return included)", idx.Points[2].Value, wantT2)
	}
}

func TestCompositeIndex_LogReturnRoundTrip(t *testing.T) {
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{a, b},
		candles: map[string][]fakeCandle{
			"AAAUSDT:4h": {
				{symbol: a, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: a, timeframe: tf, ts: ts4h(1), close: 150},
				{symbol: a, timeframe: tf, ts: ts4h(2), close: 100},
			},
			"BBBUSDT:4h": {
				{symbol: b, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: b, timeframe: tf, ts: ts4h(1), close: 100},
				{symbol: b, timeframe: tf, ts: ts4h(2), close: 100},
			},
		},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Points) != 3 {
		t.Fatalf("got %d points", len(idx.Points))
	}
	// Log median at t1: median(ln(1.5), 0) = ln(sqrt(1.5)) — not arithmetic 125.
	wantT1 := 100 * math.Sqrt(1.5)
	if math.Abs(idx.Points[1].Value-wantT1) > 0.01 {
		t.Errorf("t1 = %.4f, want %.4f (log vs arithmetic)", idx.Points[1].Value, wantT1)
	}
	if math.Abs(idx.Points[2].Value-100) > 0.01 {
		t.Errorf("log composite should end at 100±0.01, got %.4f", idx.Points[2].Value)
	}
}

func TestCompositeIndex_CoverageThresholdThreeSymbols(t *testing.T) {
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	c := makeSymbol2("CCCUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{a, b, c},
		candles: map[string][]fakeCandle{
			"AAAUSDT:4h": {
				{symbol: a, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: a, timeframe: tf, ts: ts4h(1), close: 110}, // alone at t1
				{symbol: a, timeframe: tf, ts: ts4h(2), close: 120},
			},
			"BBBUSDT:4h": {
				{symbol: b, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: b, timeframe: tf, ts: ts4h(2), close: 100},
			},
			"CCCUSDT:4h": {
				{symbol: c, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: c, timeframe: tf, ts: ts4h(2), close: 100},
			},
		},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(1).Unix() {
			t.Fatal("t1 present on only 1/3 symbols must not enter the timeline")
		}
	}
	if len(idx.Points) != 2 {
		t.Fatalf("want t0+t2 only, got %d points", len(idx.Points))
	}
	if idx.Points[0].Value != 100 {
		t.Fatalf("t0 = %.2f want 100", idx.Points[0].Value)
	}
	if idx.Points[1].Timestamp != ts4h(2).Unix() {
		t.Fatalf("second point ts=%d want t2", idx.Points[1].Timestamp)
	}
	// Median of AAA ln(120/110), BBB 0, CCC 0 → 0 return → stays 100.
	if math.Abs(idx.Points[1].Value-100) > 0.01 {
		t.Errorf("t2 = %.4f want 100 (median of one move + two flats)", idx.Points[1].Value)
	}
}

func TestCompositeIndex_StaggeredNoFakeFlatBar(t *testing.T) {
	// A: t0,t1 ; B: t2,t3. Live clock prefers B's later cluster.
	// t2 is B's anchor (100); t3 compounds +10%. A's older grid is absent.
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{a, b},
		candles: map[string][]fakeCandle{
			"AAAUSDT:4h": {
				{symbol: a, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: a, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"BBBUSDT:4h": {
				{symbol: b, timeframe: tf, ts: ts4h(2), close: 200},
				{symbol: b, timeframe: tf, ts: ts4h(3), close: 220},
			},
		},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Fatalf("SymbolCount=%d want 1 (live cluster B)", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(0).Unix() || p.Timestamp == ts4h(1).Unix() {
			t.Fatalf("older cluster timestamp %d must be absent", p.Timestamp)
		}
	}
	if len(idx.Points) != 2 {
		t.Fatalf("want t2,t3 (len=2), got %d: %v", len(idx.Points), idx.Points)
	}
	if math.Abs(idx.Points[0].Value-100) > 0.01 {
		t.Errorf("t2 anchor = %.2f want 100", idx.Points[0].Value)
	}
	if math.Abs(idx.Points[1].Value-110) > 0.01 {
		t.Errorf("t3 = %.4f want 110", idx.Points[1].Value)
	}
}

func TestCompositeIndex_ZeroCloseSkippedStaysFinite(t *testing.T) {
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{a, b},
		candles: map[string][]fakeCandle{
			"AAAUSDT:4h": {
				{symbol: a, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: a, timeframe: tf, ts: ts4h(1), close: 0}, // invalid
				{symbol: a, timeframe: tf, ts: ts4h(2), close: 110},
			},
			"BBBUSDT:4h": {
				{symbol: b, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: b, timeframe: tf, ts: ts4h(1), close: 105},
				{symbol: b, timeframe: tf, ts: ts4h(2), close: 110},
			},
		},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(provider, 4, nil)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range idx.Points {
		if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) || p.Value <= 0 {
			t.Fatalf("non-finite index point %+v", p)
		}
	}
	if len(idx.Points) < 2 {
		t.Fatalf("expected surviving bars, got %d", len(idx.Points))
	}
	// t1: only BBB (AAA zero dropped) → +5%.
	if idx.Points[1].Timestamp != ts4h(1).Unix() {
		t.Fatalf("points[1] ts=%d want t1", idx.Points[1].Timestamp)
	}
	if math.Abs(idx.Points[1].Value-105) > 0.01 {
		t.Errorf("t1 = %.4f want 105", idx.Points[1].Value)
	}
}

func TestCompositeIndex_ExcludedSymbolCountIsContributors(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	usdc := makeSymbol2("USDCUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, usdc},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 101},
			},
			"USDCUSDT:4h": {
				{symbol: usdc, timeframe: tf, ts: ts4h(0), close: 1},
				{symbol: usdc, timeframe: tf, ts: ts4h(1), close: 1},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Fatalf("SymbolCount=%d want 1 (USDC excluded)", idx.SymbolCount)
	}
}

func TestCompositeIndex_MinorityDisjointIgnored(t *testing.T) {
	// 3 aligned recent (resumed last-N) + 2 older disjoint: need from 3.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"AAAUSDT", "BBBUSDT", "CCCUSDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 10; j < 10+100; j++ {
			close := 100.0
			if j == 11 {
				close = 110
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: close})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"OLD1USDT", "OLD2USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(0), close: 50},
			{symbol: s, timeframe: tf, ts: ts4h(1), close: 55},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 3 {
		t.Fatalf("SymbolCount=%d want 3 (active live set)", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(0).Unix() || p.Timestamp == ts4h(1).Unix() {
			t.Fatalf("disjoint old timestamp %d must be absent", p.Timestamp)
		}
	}
}

func TestCompositeIndex_LargerOldClusterDoesNotSteal(t *testing.T) {
	// 4 recent resumed last-N + 5 old-disjoint denser stub.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 20; j < 20+100; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(0), close: 10},
			{symbol: s, timeframe: tf, ts: ts4h(1), close: 11},
			{symbol: s, timeframe: tf, ts: ts4h(2), close: 12},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 recent", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp < ts4h(20).Unix() {
			t.Fatalf("old timestamp %d stole the timeline", p.Timestamp)
		}
	}
}

func TestCompositeIndex_BridgeBarDoesNotPullDeadCluster(t *testing.T) {
	// 4 live resumed last-N + 5 old; R1 also prints on an old shared timestamp.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 20; j < 20+100; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		if i == 0 {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(2), close: 99})
		}
		candles[name+":4h"] = bars
	}
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(0), close: 10},
			{symbol: s, timeframe: tf, ts: ts4h(1), close: 11},
			{symbol: s, timeframe: tf, ts: ts4h(2), close: 12},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live (bridge must not pull olds)", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp < ts4h(20).Unix() {
			t.Fatalf("old/bridged timestamp %d must not appear on the tape", p.Timestamp)
		}
	}
}

func TestCompositeIndex_ReturnPairLimitKeepsOlderBars(t *testing.T) {
	// One live cluster, limit=3 keeps the last 3 return-pair bars.
	tf := makeTimeframe2("4h")
	a := makeSymbol2("AAAUSDT")
	b := makeSymbol2("BBBUSDT")
	var aBars, bBars []fakeCandle
	for i := 0; i < 6; i++ {
		aBars = append(aBars, fakeCandle{symbol: a, timeframe: tf, ts: ts4h(i), close: 100 + float64(i)})
		bBars = append(bBars, fakeCandle{symbol: b, timeframe: tf, ts: ts4h(i), close: 100})
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{
			symbols: []domain.Symbol{a, b},
			candles: map[string][]fakeCandle{"AAAUSDT:4h": aBars, "BBBUSDT:4h": bBars},
		}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Points) != 3 {
		t.Fatalf("want 3 bars under limit, got %d: %v", len(idx.Points), idx.Points)
	}
	if idx.Points[0].Timestamp != ts4h(3).Unix() {
		t.Fatalf("trim should keep last 3 return-pair bars starting t3, got ts=%d", idx.Points[0].Timestamp)
	}
	if idx.Points[2].Timestamp != ts4h(5).Unix() {
		t.Fatalf("last bar ts=%d want t5", idx.Points[2].Timestamp)
	}
}

func TestCompositeIndex_FirstPrintBurstDoesNotEatLimit(t *testing.T) {
	// Full CalculateTape path with ≥2 bars per newcomer (fetch gate).
	// 5 majors on 0..8 (hole at 7); 5 newcomers only at 7,8. Dense tip is t8
	// (all 10), so both sets stay active. t7 is print-covered by newcomers
	// only (no return pairs) — must not take a limit slot.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("M%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j <= 8; j++ {
			if j == 7 {
				continue // hole: newcomers alone at t7
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("N%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(7), close: 50},
			{symbol: s, timeframe: tf, ts: ts4h(8), close: 51},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 4)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 10 {
		t.Fatalf("SymbolCount=%d want 10", idx.SymbolCount)
	}
	want := []int64{ts4h(4).Unix(), ts4h(5).Unix(), ts4h(6).Unix(), ts4h(8).Unix()}
	if len(idx.Points) != len(want) {
		t.Fatalf("want %d bars %v, got %d: %v", len(want), want, len(idx.Points), idx.Points)
	}
	for i, ts := range want {
		if idx.Points[i].Timestamp != ts {
			t.Fatalf("points[%d].Timestamp=%d want %d (full points=%v)", i, idx.Points[i].Timestamp, ts, idx.Points)
		}
	}
}

func TestCompositeIndex_CoordinatedFuturePairDoesNotSteal(t *testing.T) {
	// Two names share a far-ahead pair; four aligned live names must remain.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(10), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(11), close: 101},
			{symbol: s, timeframe: tf, ts: ts4h(12), close: 102},
		}
	}
	for _, name := range []string{"F1USDT", "F2USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(100), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(101), close: 1},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(100).Unix() {
			t.Fatalf("future timestamp %d stole the tape", p.Timestamp)
		}
	}
}

func TestCompositeIndex_CoordinatedFutureTrioDoesNotSteal(t *testing.T) {
	// 4 live + 3 shared future bars (freq==3 in (peak/2, need)); live must remain.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(10), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(11), close: 101},
			{symbol: s, timeframe: tf, ts: ts4h(12), close: 102},
		}
	}
	for _, name := range []string{"F1USDT", "F2USDT", "F3USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(100), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(101), close: 1},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(100).Unix() {
			t.Fatalf("future trio timestamp %d stole the tape", p.Timestamp)
		}
	}
}

func TestCompositeIndex_FutureSingletonTwoBarsDoesNotSteal(t *testing.T) {
	// One name with a 2-bar far-ahead singleton (passes fetch gate); live must remain.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(10), close: 100},
			{symbol: s, timeframe: tf, ts: ts4h(11), close: 101},
			{symbol: s, timeframe: tf, ts: ts4h(12), close: 102},
		}
	}
	fut := makeSymbol2("FUTUREUSDT")
	syms = append(syms, fut)
	candles["FUTUREUSDT:4h"] = []fakeCandle{
		{symbol: fut, timeframe: tf, ts: ts4h(100), close: 1},
		{symbol: fut, timeframe: tf, ts: ts4h(101), close: 1},
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(100).Unix() {
			t.Fatalf("future singleton timestamp %d stole the tape", p.Timestamp)
		}
	}
}

func TestCompositeIndex_LatestTipNotUnionOfEqualDensity(t *testing.T) {
	tf := makeTimeframe2("4h")
	t.Run("withOld", func(t *testing.T) {
		candles := map[string][]fakeCandle{}
		var syms []domain.Symbol
		for _, name := range []string{"E1USDT", "E2USDT", "E3USDT"} {
			s := makeSymbol2(name)
			syms = append(syms, s)
			candles[name+":4h"] = []fakeCandle{
				{symbol: s, timeframe: tf, ts: ts4h(80), close: 100},
				{symbol: s, timeframe: tf, ts: ts4h(81), close: 101},
			}
		}
		for _, name := range []string{"L1USDT", "L2USDT", "L3USDT"} {
			s := makeSymbol2(name)
			syms = append(syms, s)
			candles[name+":4h"] = []fakeCandle{
				{symbol: s, timeframe: tf, ts: ts4h(90), close: 100},
				{symbol: s, timeframe: tf, ts: ts4h(91), close: 101},
			}
		}
		for _, name := range []string{"O1USDT", "O2USDT"} {
			s := makeSymbol2(name)
			syms = append(syms, s)
			candles[name+":4h"] = []fakeCandle{
				{symbol: s, timeframe: tf, ts: ts4h(0), close: 50},
				{symbol: s, timeframe: tf, ts: ts4h(1), close: 51},
			}
		}
		svc := metrics.NewCompositeIndexServiceFiltered(
			&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
		)
		idx, err := svc.Calculate(context.Background(), "4h", 200)
		if err != nil {
			t.Fatal(err)
		}
		if idx.SymbolCount != 3 {
			t.Fatalf("SymbolCount=%d want 3 (latest tip only)", idx.SymbolCount)
		}
		for _, p := range idx.Points {
			if p.Timestamp < ts4h(90).Unix() {
				t.Fatalf("earlier equal-density timestamp %d must not merge into the tip", p.Timestamp)
			}
		}
	})
	t.Run("clean3vs3", func(t *testing.T) {
		candles := map[string][]fakeCandle{}
		var syms []domain.Symbol
		for _, name := range []string{"E1USDT", "E2USDT", "E3USDT"} {
			s := makeSymbol2(name)
			syms = append(syms, s)
			candles[name+":4h"] = []fakeCandle{
				{symbol: s, timeframe: tf, ts: ts4h(80), close: 100},
				{symbol: s, timeframe: tf, ts: ts4h(81), close: 101},
			}
		}
		for _, name := range []string{"L1USDT", "L2USDT", "L3USDT"} {
			s := makeSymbol2(name)
			syms = append(syms, s)
			candles[name+":4h"] = []fakeCandle{
				{symbol: s, timeframe: tf, ts: ts4h(90), close: 100},
				{symbol: s, timeframe: tf, ts: ts4h(91), close: 101},
			}
		}
		svc := metrics.NewCompositeIndexServiceFiltered(
			&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
		)
		idx, err := svc.Calculate(context.Background(), "4h", 200)
		if err != nil {
			t.Fatal(err)
		}
		if idx.SymbolCount != 3 {
			t.Fatalf("SymbolCount=%d want 3 late", idx.SymbolCount)
		}
		for _, p := range idx.Points {
			if p.Timestamp < ts4h(90).Unix() {
				t.Fatalf("early timestamp %d must not own the tape", p.Timestamp)
			}
		}
	})
}

func TestCompositeIndex_TipMissStaysOnPriorDenseRun(t *testing.T) {
	// Shared history so t10 and t11 both sit in the last quarter; A4 misses tip.
	// A3+A4 each +10% at t10; A1+A2 flat. Median is +10% only if A4 stays active
	// (with A4 gone, median of three returns is 0).
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i, name := range []string{"A1USDT", "A2USDT", "A3USDT", "A4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j <= 10; j++ {
			close := 100.0
			if j == 10 && i >= 2 {
				close = 110
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: close})
		}
		if i < 3 {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(11), close: bars[len(bars)-1].close})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 (tip miss must not eject)", idx.SymbolCount)
	}
	var t10val float64
	found := false
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(10).Unix() {
			t10val = p.Value
			found = true
			break
		}
	}
	if !found {
		t.Fatal("t10 bar missing")
	}
	if math.Abs(t10val-100*math.Sqrt(1.1)) > 0.01 {
		t.Fatalf("t10=%.4f want %.4f (A4 must contribute; without A4 median return is 0 → 100)", t10val, 100*math.Sqrt(1.1))
	}
}

func TestCompositeIndex_FiveBarHoleTipMissStays(t *testing.T) {
	// A3+A4 +10% at t18; A4 misses tip t24. Median at t18 is 100√1.1 only if A4 stays.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i, name := range []string{"A1USDT", "A2USDT", "A3USDT", "A4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j <= 18; j++ {
			close := 100.0
			if j == 18 && i >= 2 {
				close = 110
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: close})
		}
		if i < 3 {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(24), close: bars[len(bars)-1].close})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 across 5-bar hole", idx.SymbolCount)
	}
	var t18val float64
	found := false
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(18).Unix() {
			t18val = p.Value
			found = true
			break
		}
	}
	if !found {
		t.Fatal("t18 bar missing")
	}
	want := 100 * math.Sqrt(1.1)
	if math.Abs(t18val-want) > 0.01 {
		t.Fatalf("t18=%.4f want %.4f (A4 must contribute across 5-bar hole)", t18val, want)
	}
}

func TestCompositeIndex_FutureTrioAfterLongLivePrefix(t *testing.T) {
	tf := makeTimeframe2("4h")
	for _, aheadDays := range []int{14, 80} {
		aheadDays := aheadDays
		t.Run(fmt.Sprintf("plus%dd", aheadDays), func(t *testing.T) {
			candles := map[string][]fakeCandle{}
			var syms []domain.Symbol
			for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := 0; j < 200; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				candles[name+":4h"] = bars
			}
			start := 199 + aheadDays*6
			for _, name := range []string{"F1USDT", "F2USDT", "F3USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				candles[name+":4h"] = []fakeCandle{
					{symbol: s, timeframe: tf, ts: ts4h(start), close: 1},
					{symbol: s, timeframe: tf, ts: ts4h(start + 1), close: 1},
				}
			}
			svc := metrics.NewCompositeIndexServiceFiltered(
				&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
			)
			idx, err := svc.Calculate(context.Background(), "4h", 200)
			if err != nil {
				t.Fatal(err)
			}
			if idx.SymbolCount != 4 {
				t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
			}
			for _, p := range idx.Points {
				if p.Timestamp >= ts4h(start).Unix() {
					t.Fatalf("future trio at +%dd stole the tape at %d", aheadDays, p.Timestamp)
				}
			}
		})
	}
}

func TestCompositeIndex_LongStaleLastNKeepsFarLive(t *testing.T) {
	// Fetcher-shaped: 5 stale t0–t199; 4 live overlap last K of prefix + resume after 50-bar halt.
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 50
	resume := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := staleBars - 20; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		bars = append(bars,
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume), close: 100},
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume + 1), close: 101},
		)
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
	}
	if len(idx.Points) == 0 {
		t.Fatal("empty tape")
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(resume).Unix() {
		t.Fatalf("tape tip %d want ≥ resume t%d", last, resume)
	}
}

func TestCompositeIndex_WeekendHaltKeepsContinuingLive(t *testing.T) {
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 12
	resume := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := staleBars - 20; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		bars = append(bars,
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume), close: 100},
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume + 1), close: 101},
		)
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live after weekend halt", idx.SymbolCount)
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(resume).Unix() {
		t.Fatalf("tape tip %d want ≥ resume t%d", last, resume)
	}
}

func TestCompositeIndex_FourBarHaltKeepsContinuingLive(t *testing.T) {
	// ts4h: resume = 199+4 ⇒ 4-step gap (three missing bars) > robustStep.
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 4
	resume := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := staleBars - 20; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		bars = append(bars,
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume), close: 100},
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume + 1), close: 101},
		)
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live after 4-bar halt", idx.SymbolCount)
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(resume).Unix() {
		t.Fatalf("tape tip %d want ≥ resume t%d", last, resume)
	}
}

func TestCompositeIndex_ResumedLastNNoOverlapKeepsLive(t *testing.T) {
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 50
	start := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := start; j < start+200; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 resumed last-N", idx.SymbolCount)
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(start).Unix() {
		t.Fatalf("tape tip %d want ≥ resume start t%d", last, start)
	}
}

func TestCompositeIndex_NoOverlap99BarsStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	const staleBars = 200
	for _, halt := range []int{4, 12, 50} {
		halt := halt
		t.Run(fmt.Sprintf("halt%d", halt), func(t *testing.T) {
			start := staleBars - 1 + halt
			candles := map[string][]fakeCandle{}
			var syms []domain.Symbol
			for i := 1; i <= 5; i++ {
				name := fmt.Sprintf("OLD%dUSDT", i)
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := 0; j < staleBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
				}
				candles[name+":4h"] = bars
			}
			for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := start; j < start+99; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				candles[name+":4h"] = bars
			}
			svc := metrics.NewCompositeIndexServiceFiltered(
				&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
			)
			idx, err := svc.Calculate(context.Background(), "4h", 200)
			if err != nil {
				t.Fatal(err)
			}
			if idx.SymbolCount != 5 {
				t.Fatalf("SymbolCount=%d want 5 stale (99 < resumedLastNBars)", idx.SymbolCount)
			}
			for _, p := range idx.Points {
				if p.Timestamp >= ts4h(start).Unix() {
					t.Fatalf("99-bar island stole the tape at %d (halt %d)", p.Timestamp, halt)
				}
			}
		})
	}
}

func TestCompositeIndex_NoOverlap100BarsKeepsLive(t *testing.T) {
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 50
	start := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"R1USDT", "R2USDT", "R3USDT", "R4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := start; j < start+100; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live at resumedLastNBars", idx.SymbolCount)
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(start).Unix() {
		t.Fatalf("tape tip %d want ≥ resume start t%d", last, start)
	}
}

func TestCompositeIndex_FutureQuartetAfterLongLivePrefix(t *testing.T) {
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < 200; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(400), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(401), close: 1},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 5 {
		t.Fatalf("SymbolCount=%d want 5 live", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(400).Unix() {
			t.Fatalf("future quartet stole the tape at %d", p.Timestamp)
		}
	}
}

func TestCompositeIndex_FutureQuartetAfterStaggeredPrefixStillStrips(t *testing.T) {
	// Five live, rotating miss → prefixMax = 4; 2-bar quartet must strip.
	tf := makeTimeframe2("4h")
	for _, liveBars := range []int{19, 200} {
		liveBars := liveBars
		t.Run(fmt.Sprintf("prefix%d", liveBars), func(t *testing.T) {
			candles := map[string][]fakeCandle{}
			var syms []domain.Symbol
			for i, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := 0; j < liveBars; j++ {
					if j%5 == i {
						continue
					}
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				candles[name+":4h"] = bars
			}
			for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				candles[name+":4h"] = []fakeCandle{
					{symbol: s, timeframe: tf, ts: ts4h(400), close: 1},
					{symbol: s, timeframe: tf, ts: ts4h(401), close: 1},
				}
			}
			svc := metrics.NewCompositeIndexServiceFiltered(
				&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
			)
			idx, err := svc.Calculate(context.Background(), "4h", 200)
			if err != nil {
				t.Fatal(err)
			}
			if idx.SymbolCount != 5 {
				t.Fatalf("SymbolCount=%d want 5 live", idx.SymbolCount)
			}
			for _, p := range idx.Points {
				if p.Timestamp >= ts4h(400).Unix() {
					t.Fatalf("staggered-prefix quartet stole the tape at %d", p.Timestamp)
				}
			}
		})
	}
}

func TestCompositeIndex_FutureQuartet20BarsStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < 200; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 400; j < 420; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 1})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 5 {
		t.Fatalf("SymbolCount=%d want 5 live (20-bar burst is not resumed last-N)", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(400).Unix() {
			t.Fatalf("20-bar future quartet stole the tape at %d", p.Timestamp)
		}
	}
}

func TestCompositeIndex_FutureQuartet20After20PrefixStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	for _, liveBars := range []int{19, 20} {
		liveBars := liveBars
		for _, halt := range []int{4, 12} {
			halt := halt
			t.Run(fmt.Sprintf("prefix%d_halt%d", liveBars, halt), func(t *testing.T) {
				start := liveBars - 1 + halt
				candles := map[string][]fakeCandle{}
				var syms []domain.Symbol
				for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
					s := makeSymbol2(name)
					syms = append(syms, s)
					var bars []fakeCandle
					for j := 0; j < liveBars; j++ {
						bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
					}
					candles[name+":4h"] = bars
				}
				for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT"} {
					s := makeSymbol2(name)
					syms = append(syms, s)
					var bars []fakeCandle
					for j := start; j < start+20; j++ {
						bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 1})
					}
					candles[name+":4h"] = bars
				}
				svc := metrics.NewCompositeIndexServiceFiltered(
					&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
				)
				idx, err := svc.Calculate(context.Background(), "4h", 200)
				if err != nil {
					t.Fatal(err)
				}
				if idx.SymbolCount != 5 {
					t.Fatalf("SymbolCount=%d want 5 live", idx.SymbolCount)
				}
				for _, p := range idx.Points {
					if p.Timestamp >= ts4h(start).Unix() {
						t.Fatalf("future stole the tape at %d", p.Timestamp)
					}
				}
			})
		}
	}
}

func TestCompositeIndex_HalfNewIslandStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	for _, islandBars := range []int{2, 100} {
		islandBars := islandBars
		t.Run(fmt.Sprintf("island%d", islandBars), func(t *testing.T) {
			const staleBars = 200
			const halt = 12
			resume := staleBars - 1 + halt
			candles := map[string][]fakeCandle{}
			var syms []domain.Symbol
			for i := 1; i <= 5; i++ {
				name := fmt.Sprintf("OLD%dUSDT", i)
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := 0; j < staleBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
				}
				candles[name+":4h"] = bars
			}
			for _, name := range []string{"R1USDT", "R2USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := staleBars - 20; j < staleBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				for j := resume; j < resume+islandBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				candles[name+":4h"] = bars
			}
			for _, name := range []string{"F1USDT", "F2USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := resume; j < resume+islandBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 1})
				}
				candles[name+":4h"] = bars
			}
			svc := metrics.NewCompositeIndexServiceFiltered(
				&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
			)
			idx, err := svc.Calculate(context.Background(), "4h", 200)
			if err != nil {
				t.Fatal(err)
			}
			if idx.SymbolCount != 7 {
				t.Fatalf("SymbolCount=%d want 5 stale + 2 tip-continuing (no F)", idx.SymbolCount)
			}
			for _, p := range idx.Points {
				if p.Timestamp >= ts4h(resume).Unix() {
					t.Fatalf("half-new island stole the tape at %d", p.Timestamp)
				}
			}
		})
	}
}

func TestCompositeIndex_EqualDensityFutureFiveStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	for _, liveBars := range []int{7, 200} {
		liveBars := liveBars
		t.Run(fmt.Sprintf("prefix%d", liveBars), func(t *testing.T) {
			candles := map[string][]fakeCandle{}
			var syms []domain.Symbol
			for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				var bars []fakeCandle
				for j := 0; j < liveBars; j++ {
					bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
				}
				candles[name+":4h"] = bars
			}
			for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT", "F5USDT"} {
				s := makeSymbol2(name)
				syms = append(syms, s)
				candles[name+":4h"] = []fakeCandle{
					{symbol: s, timeframe: tf, ts: ts4h(400), close: 1},
					{symbol: s, timeframe: tf, ts: ts4h(401), close: 1},
				}
			}
			svc := metrics.NewCompositeIndexServiceFiltered(
				&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
			)
			idx, err := svc.Calculate(context.Background(), "4h", 200)
			if err != nil {
				t.Fatal(err)
			}
			if idx.SymbolCount != 5 {
				t.Fatalf("SymbolCount=%d want 5 live", idx.SymbolCount)
			}
			for _, p := range idx.Points {
				if p.Timestamp >= ts4h(400).Unix() {
					t.Fatalf("5-vs-5 future stole the tape at %d", p.Timestamp)
				}
			}
		})
	}
}

func TestCompositeIndex_FiveNewAfterFourLiveStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < 200; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT", "F5USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(400), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(401), close: 1},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 live", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(400).Unix() {
			t.Fatalf("5-new after 4-live stole the tape at %d", p.Timestamp)
		}
	}
}

func TestCompositeIndex_FutureQuartetOneOldTickStillStrips(t *testing.T) {
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for _, name := range []string{"L1USDT", "L2USDT", "L3USDT", "L4USDT", "L5USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < 200; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		candles[name+":4h"] = bars
	}
	for _, name := range []string{"F1USDT", "F2USDT", "F3USDT", "F4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		candles[name+":4h"] = []fakeCandle{
			{symbol: s, timeframe: tf, ts: ts4h(100), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(400), close: 1},
			{symbol: s, timeframe: tf, ts: ts4h(401), close: 1},
		}
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 5 {
		t.Fatalf("SymbolCount=%d want 5 live (one old tick is not continuation)", idx.SymbolCount)
	}
	for _, p := range idx.Points {
		if p.Timestamp >= ts4h(400).Unix() {
			t.Fatalf("one-tick quartet stole the tape at %d", p.Timestamp)
		}
	}
}

func TestCompositeIndex_ContinuingLiveAfterLargerStale(t *testing.T) {
	tf := makeTimeframe2("4h")
	const staleBars = 200
	const halt = 50
	resume := staleBars - 1 + halt
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i := 1; i <= 50; i++ {
		name := fmt.Sprintf("OLD%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 10})
		}
		candles[name+":4h"] = bars
	}
	for i := 1; i <= 30; i++ {
		name := fmt.Sprintf("R%dUSDT", i)
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := staleBars - 20; j < staleBars; j++ {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: 100})
		}
		bars = append(bars,
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume), close: 100},
			fakeCandle{symbol: s, timeframe: tf, ts: ts4h(resume + 1), close: 101},
		)
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 30 {
		t.Fatalf("SymbolCount=%d want 30 live", idx.SymbolCount)
	}
	if len(idx.Points) == 0 {
		t.Fatal("empty tape")
	}
	last := idx.Points[len(idx.Points)-1].Timestamp
	if last < ts4h(resume).Unix() {
		t.Fatalf("tape tip %d want ≥ resume t%d", last, resume)
	}
}

func TestCompositeIndex_OffGridSecondDoesNotBreakTipMiss(t *testing.T) {
	// One +1s misaligned print must not shrink robustStep and eject a tip-miss.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i, name := range []string{"A1USDT", "A2USDT", "A3USDT", "A4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j <= 10; j++ {
			close := 100.0
			if j == 10 && i >= 2 {
				close = 110
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: close})
		}
		if i == 0 {
			off := ts4h(5).Add(time.Second)
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: off, close: 100})
		}
		if i < 3 {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(11), close: bars[len(bars)-1].close})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 4 {
		t.Fatalf("SymbolCount=%d want 4 despite 1s noise", idx.SymbolCount)
	}
	var t10val float64
	found := false
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(10).Unix() {
			t10val = p.Value
			found = true
			break
		}
	}
	if !found {
		t.Fatal("t10 bar missing")
	}
	if math.Abs(t10val-100*math.Sqrt(1.1)) > 0.01 {
		t.Fatalf("t10=%.4f want %.4f (A4 must stay despite off-grid print)", t10val, 100*math.Sqrt(1.1))
	}
}

func TestCompositeIndex_SixBarHoleTipMissDrops(t *testing.T) {
	// Product policy: six or more missing bars drop the prior side of the tip run.
	tf := makeTimeframe2("4h")
	candles := map[string][]fakeCandle{}
	var syms []domain.Symbol
	for i, name := range []string{"A1USDT", "A2USDT", "A3USDT", "A4USDT"} {
		s := makeSymbol2(name)
		syms = append(syms, s)
		var bars []fakeCandle
		for j := 0; j <= 25; j++ {
			close := 100.0
			if j == 25 && i >= 2 {
				close = 110
			}
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(j), close: close})
		}
		if i < 3 {
			bars = append(bars, fakeCandle{symbol: s, timeframe: tf, ts: ts4h(32), close: bars[len(bars)-1].close})
		}
		candles[name+":4h"] = bars
	}
	svc := metrics.NewCompositeIndexServiceFiltered(
		&fakeCandleProvider{symbols: syms, candles: candles}, 4, nil,
	)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 3 {
		t.Fatalf("SymbolCount=%d want 3 (A4 dropped across 6+ missing bars)", idx.SymbolCount)
	}
	var t25val float64
	found := false
	for _, p := range idx.Points {
		if p.Timestamp == ts4h(25).Unix() {
			t25val = p.Value
			found = true
			break
		}
	}
	if !found {
		t.Fatal("t25 bar must be present")
	}
	if math.Abs(t25val-100) > 0.01 {
		t.Fatalf("t25=%.4f want 100 (A4 dropped; median of tip-side returns is 0)", t25val)
	}
}

func TestCompositeIndex_ExcludedSymbolNeverFetched(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	usdc := makeSymbol2("USDCUSDT")
	tf := makeTimeframe2("4h")
	provider := &spyCandleProvider{
		fakeCandleProvider: fakeCandleProvider{
			symbols: []domain.Symbol{btc, usdc},
			candles: map[string][]fakeCandle{
				"BTCUSDT:4h": {
					{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
					{symbol: btc, timeframe: tf, ts: ts4h(1), close: 101},
				},
				"USDCUSDT:4h": {
					{symbol: usdc, timeframe: tf, ts: ts4h(0), close: 1},
					{symbol: usdc, timeframe: tf, ts: ts4h(1), close: 1},
				},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	if _, err := svc.Calculate(context.Background(), "4h", 200); err != nil {
		t.Fatal(err)
	}
	fetched := provider.snapshot()
	for _, key := range fetched {
		if key == "USDCUSDT:4h" {
			t.Fatal("excluded USDCUSDT was fetched")
		}
	}
	if len(fetched) != 1 || fetched[0] != "BTCUSDT:4h" {
		t.Fatalf("fetched = %v, want only BTCUSDT:4h", fetched)
	}
}

type spyCandleProvider struct {
	fakeCandleProvider
	mu      sync.Mutex
	fetched []string
}

func (s *spyCandleProvider) GetLastNCandles(ctx context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	s.mu.Lock()
	s.fetched = append(s.fetched, sym.String()+":"+tf.String())
	s.mu.Unlock()
	return s.fakeCandleProvider.GetLastNCandles(ctx, sym, tf, n)
}

func (s *spyCandleProvider) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.fetched))
	copy(out, s.fetched)
	return out
}

// --- HTTP Handler Tests ---

type fakeCompositeCalc struct {
	index mkt.CompositeIndex
	err   error
}

func (f *fakeCompositeCalc) Calculate(_ context.Context, _ string, _ int) (mkt.CompositeIndex, error) {
	if f.err != nil {
		return mkt.CompositeIndex{}, f.err
	}
	return f.index, nil
}

func TestCompositeHandler_DefaultParams(t *testing.T) {
	calc := &fakeCompositeCalc{
		index: mkt.CompositeIndex{
			Timeframe:   "4h",
			SymbolCount: 5,
			Points: []mkt.IndexPoint{
				{Timestamp: 1000, Value: 100.0},
				{Timestamp: 2000, Value: 101.23},
			},
		},
	}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["timeframe"] != "4h" {
		t.Errorf("expected 4h, got %v", resp["timeframe"])
	}
	if int(resp["symbolCount"].(float64)) != 5 {
		t.Errorf("expected 5, got %v", resp["symbolCount"])
	}
	pts := resp["points"].([]interface{})
	if len(pts) != 2 {
		t.Errorf("expected 2 points, got %d", len(pts))
	}
}

func TestCompositeHandler_Error(t *testing.T) {
	calc := &fakeCompositeCalc{err: fmt.Errorf("provider down")}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestCompositeHandler_PointRounding(t *testing.T) {
	calc := &fakeCompositeCalc{
		index: mkt.CompositeIndex{
			Timeframe:   "4h",
			SymbolCount: 1,
			Points: []mkt.IndexPoint{
				{Timestamp: 1000, Value: 100.12345},
			},
		},
	}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var resp struct {
		Points []struct {
			V float64 `json:"v"`
		} `json:"points"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Points[0].V != 100.12 {
		t.Errorf("expected 100.12, got %.5f", resp.Points[0].V)
	}
}
