package usecases

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	domainsignal "pano_chart/backend/domain/signal"
)

// --- Per-component percentile tests ---

func TestGetRankings_ComponentPercentiles(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")
	sol := domain.NewSymbolUnsafe("SOLUSDT")

	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	btcCandles, _ := domain.NewCandleSeries(btc, tf, []domain.Candle{
		mustNewCandleAt(btc, tf, base, 50000),
		mustNewCandleAt(btc, tf, base.Add(time.Hour), 51000),
	})
	ethCandles, _ := domain.NewCandleSeries(eth, tf, []domain.Candle{
		mustNewCandleAt(eth, tf, base, 3000),
		mustNewCandleAt(eth, tf, base.Add(time.Hour), 3100),
	})
	solCandles, _ := domain.NewCandleSeries(sol, tf, []domain.Candle{
		mustNewCandleAt(sol, tf, base, 100),
		mustNewCandleAt(sol, tf, base.Add(time.Hour), 105),
	})

	cr := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles, eth: ethCandles, sol: solCandles,
	}, nil)

	trend := &stubCalculator{name: "Trend Predictability", scores: map[string]float64{
		"BTCUSDT": 0.9, "ETHUSDT": 0.5, "SOLUSDT": 0.1,
	}}
	sideways := &stubCalculator{name: "Sideways Consistency", scores: map[string]float64{
		"BTCUSDT": 0.3, "ETHUSDT": 0.8, "SOLUSDT": 0.6,
	}}
	gain := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{
		"BTCUSDT": 0.5, "ETHUSDT": 0.2, "SOLUSDT": 0.9,
	}}

	weights := []usecases.ScoreWeight{
		{Calculator: trend, Weight: 1.0},
		{Calculator: sideways, Weight: 1.0},
		{Calculator: gain, Weight: 1.0},
	}

	universe := &fakeUniverse{symbols: []domain.Symbol{btc, eth, sol}}
	volumes := &fakeVolumes{vols: map[string]float64{
		"BTCUSDT": 1000, "ETHUSDT": 500, "SOLUSDT": 100,
	}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, cr,
		"http://fake/exchangeInfo", "http://fake/ticker",
		2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Trend scores: BTC=0.9, ETH=0.5, SOL=0.1
	// Sorted desc: BTC(0), ETH(1), SOL(2) → percentiles: 1.0, 0.5, 0.0
	//
	// Sideways scores: ETH=0.8, SOL=0.6, BTC=0.3
	// Sorted desc: ETH(0), SOL(1), BTC(2) → percentiles: 1.0, 0.5, 0.0
	//
	// Gain scores: SOL=0.9, BTC=0.5, ETH=0.2
	// Sorted desc: SOL(0), BTC(1), ETH(2) → percentiles: 1.0, 0.5, 0.0

	expected := map[string]struct {
		trend, sideways, gain float64
	}{
		"BTCUSDT": {trend: 1.0, sideways: 0.0, gain: 0.5},
		"ETHUSDT": {trend: 0.5, sideways: 1.0, gain: 0.0},
		"SOLUSDT": {trend: 0.0, sideways: 0.5, gain: 1.0},
	}

	for _, r := range results {
		exp := expected[r.Symbol.String()]
		if math.Abs(r.TrendPercentile-exp.trend) > 1e-9 {
			t.Errorf("%s: TrendPercentile want %.2f got %.2f",
				r.Symbol.String(), exp.trend, r.TrendPercentile)
		}
		if math.Abs(r.SidewaysPercentile-exp.sideways) > 1e-9 {
			t.Errorf("%s: SidewaysPercentile want %.2f got %.2f",
				r.Symbol.String(), exp.sideways, r.SidewaysPercentile)
		}
		if math.Abs(r.GainPercentile-exp.gain) > 1e-9 {
			t.Errorf("%s: GainPercentile want %.2f got %.2f",
				r.Symbol.String(), exp.gain, r.GainPercentile)
		}
	}
}

// --- Max percentile + dominant component tests ---

func TestGetRankings_MaxPercentileAndDominantComponent(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")
	sol := domain.NewSymbolUnsafe("SOLUSDT")

	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	btcCandles, _ := domain.NewCandleSeries(btc, tf, []domain.Candle{
		mustNewCandleAt(btc, tf, base, 50000),
		mustNewCandleAt(btc, tf, base.Add(time.Hour), 51000),
	})
	ethCandles, _ := domain.NewCandleSeries(eth, tf, []domain.Candle{
		mustNewCandleAt(eth, tf, base, 3000),
		mustNewCandleAt(eth, tf, base.Add(time.Hour), 3100),
	})
	solCandles, _ := domain.NewCandleSeries(sol, tf, []domain.Candle{
		mustNewCandleAt(sol, tf, base, 100),
		mustNewCandleAt(sol, tf, base.Add(time.Hour), 105),
	})

	cr := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles, eth: ethCandles, sol: solCandles,
	}, nil)

	trend := &stubCalculator{name: "Trend Predictability", scores: map[string]float64{
		"BTCUSDT": 0.9, "ETHUSDT": 0.5, "SOLUSDT": 0.1,
	}}
	sideways := &stubCalculator{name: "Sideways Consistency", scores: map[string]float64{
		"BTCUSDT": 0.3, "ETHUSDT": 0.8, "SOLUSDT": 0.6,
	}}
	gain := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{
		"BTCUSDT": 0.5, "ETHUSDT": 0.2, "SOLUSDT": 0.9,
	}}

	weights := []usecases.ScoreWeight{
		{Calculator: trend, Weight: 1.0},
		{Calculator: sideways, Weight: 1.0},
		{Calculator: gain, Weight: 1.0},
	}

	universe := &fakeUniverse{symbols: []domain.Symbol{btc, eth, sol}}
	volumes := &fakeVolumes{vols: map[string]float64{
		"BTCUSDT": 1000, "ETHUSDT": 500, "SOLUSDT": 100,
	}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, cr,
		"http://fake/exchangeInfo", "http://fake/ticker",
		2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results

	// BTC: trend=1.0, sideways=0.0, gain=0.5 -> max=1.0 dominant=trend
	// ETH: trend=0.5, sideways=1.0, gain=0.0 -> max=1.0 dominant=sideways
	// SOL: trend=0.0, sideways=0.5, gain=1.0 -> max=1.0 dominant=gain
	expected := map[string]struct {
		maxP     float64
		dominant string
	}{
		"BTCUSDT": {maxP: 1.0, dominant: "trend"},
		"ETHUSDT": {maxP: 1.0, dominant: "sideways"},
		"SOLUSDT": {maxP: 1.0, dominant: "gain"},
	}

	for _, r := range results {
		exp := expected[r.Symbol.String()]
		if math.Abs(r.MaxPercentile-exp.maxP) > 1e-9 {
			t.Errorf("%s: MaxPercentile want %.2f got %.2f",
				r.Symbol.String(), exp.maxP, r.MaxPercentile)
		}
		if r.DominantComponent != exp.dominant {
			t.Errorf("%s: DominantComponent want %q got %q",
				r.Symbol.String(), exp.dominant, r.DominantComponent)
		}
	}
}

// --- Badge assignment tests ---

func TestGetRankings_BadgeAssignment_TopNOnly(t *testing.T) {
	// 10 symbols → TopN = max(1, ceil(10 * 0.2)) = 2.
	// Design: SYM_J dominates trend, SYM_I dominates sideways.
	// These two should get badges; the other 8 should not.

	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	names := []string{
		"SYMAUSDT", "SYMBUSDT", "SYMCUSDT", "SYMDUSDT", "SYMEUSDT",
		"SYMFUSDT", "SYMGUSDT", "SYMHUSDT", "SYMIUSDT", "SYMJUSDT",
	}
	symbols := make([]domain.Symbol, 10)
	candlesMap := make(map[domain.Symbol]domain.CandleSeries)
	volMap := make(map[string]float64)

	for i, name := range names {
		s := domain.NewSymbolUnsafe(name)
		symbols[i] = s
		candles, _ := domain.NewCandleSeries(s, tf, []domain.Candle{
			mustNewCandleAt(s, tf, base, float64(100+i*10)),
			mustNewCandleAt(s, tf, base.Add(time.Hour), float64(105+i*10)),
		})
		candlesMap[s] = candles
		volMap[name] = float64(2000 - i*100)
	}

	// Trend: only SYMJ has a high score.
	// Sideways: only SYMI has a high score.
	// Gain: moderate for all, no standouts.
	trendScores := map[string]float64{
		"SYMAUSDT": 0.10, "SYMBUSDT": 0.15, "SYMCUSDT": 0.20,
		"SYMDUSDT": 0.25, "SYMEUSDT": 0.30, "SYMFUSDT": 0.35,
		"SYMGUSDT": 0.40, "SYMHUSDT": 0.45, "SYMIUSDT": 0.50,
		"SYMJUSDT": 0.95,
	}
	sidewaysScores := map[string]float64{
		"SYMAUSDT": 0.10, "SYMBUSDT": 0.15, "SYMCUSDT": 0.20,
		"SYMDUSDT": 0.25, "SYMEUSDT": 0.30, "SYMFUSDT": 0.35,
		"SYMGUSDT": 0.40, "SYMHUSDT": 0.45, "SYMIUSDT": 0.95,
		"SYMJUSDT": 0.12,
	}
	gainScores := map[string]float64{
		"SYMAUSDT": 0.42, "SYMBUSDT": 0.44, "SYMCUSDT": 0.46,
		"SYMDUSDT": 0.48, "SYMEUSDT": 0.50, "SYMFUSDT": 0.52,
		"SYMGUSDT": 0.54, "SYMHUSDT": 0.56, "SYMIUSDT": 0.49,
		"SYMJUSDT": 0.97,
	}

	trend := &stubCalculator{name: "Trend Predictability", scores: trendScores}
	sideways := &stubCalculator{name: "Sideways Consistency", scores: sidewaysScores}
	gain := &stubCalculator{name: "Gain/Loss", scores: gainScores}

	weights := []usecases.ScoreWeight{
		{Calculator: trend, Weight: 1.0},
		{Calculator: sideways, Weight: 1.0},
		{Calculator: gain, Weight: 1.0},
	}

	universe := &fakeUniverse{symbols: symbols}
	volumes := &fakeVolumes{vols: volMap}
	ranker := usecases.NewDefaultRankSymbols(weights)

	cr := NewFakeCandleRepository(candlesMap, nil)
	uc := usecases.NewGetRankings(
		universe, ranker, volumes, cr,
		"http://fake/exchangeInfo", "http://fake/ticker",
		2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results

	badged := map[string]string{}
	for _, r := range results {
		if r.BadgeComponent != "" {
			badged[r.Symbol.String()] = r.BadgeComponent
		}
	}

	if len(badged) != 2 {
		t.Fatalf("expected 2 badges, got %d: %v", len(badged), badged)
	}

	// SYMJ should get a trend badge (highest maxPercentile via trend=1.0).
	if b, ok := badged["SYMJUSDT"]; !ok {
		t.Error("SYMJUSDT should have a badge")
	} else if b != "trend" {
		t.Errorf("SYMJUSDT badge want 'trend', got %q", b)
	}

	// SYMI should get a sideways badge (highest maxPercentile via sideways=1.0).
	if b, ok := badged["SYMIUSDT"]; !ok {
		t.Error("SYMIUSDT should have a badge")
	} else if b != "sideways" {
		t.Errorf("SYMIUSDT badge want 'sideways', got %q", b)
	}
}

func TestGetRankings_NoBadgesForSingleSymbol(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	btcCandles, _ := domain.NewCandleSeries(btc, tf, []domain.Candle{
		mustNewCandleAt(btc, tf, base, 50000),
		mustNewCandleAt(btc, tf, base.Add(time.Hour), 51000),
	})

	cr := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
	}, nil)

	trend := &stubCalculator{name: "Trend Predictability", scores: map[string]float64{
		"BTCUSDT": 0.9,
	}}
	sideways := &stubCalculator{name: "Sideways Consistency", scores: map[string]float64{
		"BTCUSDT": 0.3,
	}}
	gain := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{
		"BTCUSDT": 0.5,
	}}

	weights := []usecases.ScoreWeight{
		{Calculator: trend, Weight: 1.0},
		{Calculator: sideways, Weight: 1.0},
		{Calculator: gain, Weight: 1.0},
	}

	universe := &fakeUniverse{symbols: []domain.Symbol{btc}}
	volumes := &fakeVolumes{vols: map[string]float64{"BTCUSDT": 1000}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, cr,
		"http://fake/exchangeInfo", "http://fake/ticker",
		2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results

	// N=1 → no meaningful competition → no badge.
	if results[0].BadgeComponent != "" {
		t.Errorf("single symbol should have no badge, got %q",
			results[0].BadgeComponent)
	}
}

// --- Badge signal writers (PR-090) ---

type capturingBadgeEmitter struct {
	mu   sync.Mutex
	sigs []domainsignal.Signal
}

func (c *capturingBadgeEmitter) Emit(_ context.Context, s domainsignal.Signal) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sigs = append(c.sigs, s)
	return true
}

func TestGetRankings_NilSignalEmitterNoop(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	candles := func(sym domain.Symbol, start float64, rising bool) domain.CandleSeries {
		cs := make([]domain.Candle, 20)
		for i := 0; i < 20; i++ {
			px := start + float64(i)
			if !rising {
				px = start - float64(i)
			}
			cs[i] = mustNewCandleAt(sym, tf, base.Add(time.Duration(i)*time.Hour), px)
		}
		s, _ := domain.NewCandleSeries(sym, tf, cs)
		return s
	}

	cr := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: candles(btc, 50000, true),
		eth: candles(eth, 3000, false),
	}, nil)
	weights := []usecases.ScoreWeight{
		{Calculator: &stubCalculator{name: "Trend Predictability", scores: map[string]float64{"BTCUSDT": 0.9, "ETHUSDT": 0.8}}, Weight: 1},
		{Calculator: &stubCalculator{name: "Sideways Consistency", scores: map[string]float64{"BTCUSDT": 0.1, "ETHUSDT": 0.1}}, Weight: 1},
		{Calculator: &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"BTCUSDT": 0.2, "ETHUSDT": 0.2}}, Weight: 1},
	}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{btc, eth}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"BTCUSDT": 1, "ETHUSDT": 1}},
		cr, "http://fake/ei", "http://fake/t",
		20, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	// nil emitter by default
	if _, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{Timeframe: tf, Sort: usecases.SortByTotal}); err != nil {
		t.Fatal(err)
	}
}

func TestGetRankings_EmitsBadgeLabelsAndRangeContext(t *testing.T) {
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mk := func(sym domain.Symbol, start float64, n int, step float64) domain.CandleSeries {
		cs := make([]domain.Candle, n)
		for i := 0; i < n; i++ {
			px := start + float64(i)*step
			cs[i] = mustNewCandleAt(sym, tf, base.Add(time.Duration(i)*time.Hour), px)
		}
		s, _ := domain.NewCandleSeries(sym, tf, cs)
		return s
	}

	// 15 symbols → topN = ceil(15*0.2) = 3 badges.
	const n = 15
	symbols := make([]domain.Symbol, n)
	candlesMap := map[domain.Symbol]domain.CandleSeries{}
	volMap := map[string]float64{}
	trendScores := map[string]float64{}
	sidewaysScores := map[string]float64{}
	gainScores := map[string]float64{}

	for i := 0; i < n; i++ {
		name := "SYM" + string(rune('A'+i)) + "USDT"
		sym := domain.NewSymbolUnsafe(name)
		symbols[i] = sym
		volMap[name] = 100
		gainScores[name] = 0.2
		switch i {
		case 0: // rising trend leader
			candlesMap[sym] = mk(sym, 100, 20, 1)
			trendScores[name], sidewaysScores[name] = 1.0, 0.1
		case 1: // falling trend
			candlesMap[sym] = mk(sym, 200, 20, -1)
			trendScores[name], sidewaysScores[name] = 0.95, 0.1
		case 2: // sideways leader
			candlesMap[sym] = mk(sym, 50, 20, 0)
			trendScores[name], sidewaysScores[name] = 0.1, 1.0
		default:
			candlesMap[sym] = mk(sym, 100, 20, 0.01)
			trendScores[name] = 0.1 + float64(i)*0.01
			sidewaysScores[name] = 0.1
		}
	}

	weights := []usecases.ScoreWeight{
		{Calculator: &stubCalculator{name: "Trend Predictability", scores: trendScores}, Weight: 1},
		{Calculator: &stubCalculator{name: "Sideways Consistency", scores: sidewaysScores}, Weight: 1},
		{Calculator: &stubCalculator{name: "Gain/Loss", scores: gainScores}, Weight: 1},
	}

	cap := &capturingBadgeEmitter{}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: symbols},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: volMap},
		NewFakeCandleRepository(candlesMap, nil),
		"http://fake/ei", "http://fake/t",
		20, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	uc.SetSignalEmitter(cap)

	if _, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{Timeframe: tf, Sort: usecases.SortByTotal}); err != nil {
		t.Fatal(err)
	}
	if len(cap.sigs) != 3 {
		t.Fatalf("expected 3 badge signals, got %d", len(cap.sigs))
	}

	bySym := map[string]domainsignal.Signal{}
	for _, s := range cap.sigs {
		bySym[s.Symbol] = s
		if s.Kind != domainsignal.KindBadge {
			t.Fatalf("kind=%s", s.Kind)
		}
		if s.Price <= 0 || s.ATR <= 0 {
			t.Fatalf("%s: want series ATR, price=%v atr=%v", s.Symbol, s.Price, s.ATR)
		}
	}

	if s, ok := bySym["SYMAUSDT"]; !ok || s.Label != "trend_up" {
		t.Fatalf("SYMAUSDT want trend_up, got %#v", s)
	}
	if s, ok := bySym["SYMBUSDT"]; !ok || s.Label != "trend_down" {
		t.Fatalf("SYMBUSDT want trend_down, got %#v (all=%v)", s, bySym)
	}
	if s, ok := bySym["SYMCUSDT"]; !ok || s.Label != "sideways" {
		t.Fatalf("SYMCUSDT want sideways, got %#v", s)
	} else {
		if s.Context["range_low"] == 0 && s.Context["range_high"] == 0 {
			// flat series at 50 — both equal is fine, but keys must exist
		}
		if _, ok := s.Context["range_low"]; !ok {
			t.Fatal("sideways missing range_low")
		}
		if _, ok := s.Context["range_high"]; !ok {
			t.Fatal("sideways missing range_high")
		}
	}
}
