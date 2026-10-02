package usecases

import (
	"context"
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

// --- Fakes for GetRankings dependencies ---

type fakeUniverse struct {
	symbols []domain.Symbol
}

func (f *fakeUniverse) Symbols(_ context.Context, _, _ string) ([]domain.Symbol, error) {
	return f.symbols, nil
}

type fakeVolumes struct {
	vols map[string]float64
}

func (f *fakeVolumes) Volumes(_ context.Context) (map[string]float64, error) {
	return f.vols, nil
}

// --- Percentile tests ---

func TestGetRankings_PercentileComputation(t *testing.T) {
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

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
		eth: ethCandles,
		sol: solCandles,
	}, nil)

	calc := &stubCalculator{
		name:   "Gain/Loss",
		scores: map[string]float64{"BTCUSDT": 0.9, "ETHUSDT": 0.5, "SOLUSDT": 0.1},
	}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}

	universe := &fakeUniverse{symbols: []domain.Symbol{btc, eth, sol}}
	volumes := &fakeVolumes{vols: map[string]float64{
		"BTCUSDT": 1000, "ETHUSDT": 500, "SOLUSDT": 100,
	}}

	ranker := usecases.NewDefaultRankSymbols(weights)

	uc := usecases.NewGetRankings(
		universe,
		ranker,
		volumes,
		candleRepo,
		"http://fake/exchangeInfo", "http://fake/ticker",
		2,
		usecases.SidewaysAlgoV1,
		weights,
		4,
		nil,
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

	// Sorted descending by total: BTC (0.9) > ETH (0.5) > SOL (0.1)
	// Percentile: top=1.0, mid=0.5, bottom=0.0
	expected := map[string]float64{
		"BTCUSDT": 1.0,
		"ETHUSDT": 0.5,
		"SOLUSDT": 0.0,
	}

	for _, r := range results {
		exp := expected[r.Symbol.String()]
		if math.Abs(r.Percentile-exp) > 1e-9 {
			t.Errorf("%s: expected percentile %.4f, got %.4f", r.Symbol.String(), exp, r.Percentile)
		}
	}
}

func TestGetRankings_SingleSymbolPercentileIsOne(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	btcCandles, _ := domain.NewCandleSeries(btc, tf, []domain.Candle{
		mustNewCandleAt(btc, tf, base, 50000),
		mustNewCandleAt(btc, tf, base.Add(time.Hour), 51000),
	})

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
	}, nil)

	calc := &stubCalculator{
		name:   "Gain/Loss",
		scores: map[string]float64{"BTCUSDT": 0.9},
	}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	universe := &fakeUniverse{symbols: []domain.Symbol{btc}}
	volumes := &fakeVolumes{vols: map[string]float64{"BTCUSDT": 1000}}

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, candleRepo,
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
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Percentile != 1.0 {
		t.Errorf("single symbol should have percentile 1.0, got %.4f", results[0].Percentile)
	}
}

func TestGetRankings_PercentileInZeroOneRange(t *testing.T) {
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	symbols := make([]domain.Symbol, 10)
	candlesMap := make(map[domain.Symbol]domain.CandleSeries)
	volMap := make(map[string]float64)
	scoreMap := make(map[string]float64)

	for i := 0; i < 10; i++ {
		name := "SYM" + string(rune('A'+i)) + "USDT"
		s := domain.NewSymbolUnsafe(name)
		symbols[i] = s
		candles, _ := domain.NewCandleSeries(s, tf, []domain.Candle{
			mustNewCandleAt(s, tf, base.Add(time.Duration(i)*2*time.Hour), float64(100+i*10)),
			mustNewCandleAt(s, tf, base.Add(time.Duration(i)*2*time.Hour+time.Hour), float64(105+i*10)),
		})
		candlesMap[s] = candles
		volMap[name] = float64(1000 - i*100)
		scoreMap[name] = float64(i) / 10.0
	}

	candleRepo := NewFakeCandleRepository(candlesMap, nil)
	calc := &stubCalculator{name: "Gain/Loss", scores: scoreMap}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := usecases.NewDefaultRankSymbols(weights)
	universe := &fakeUniverse{symbols: symbols}
	volumes := &fakeVolumes{vols: volMap}

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, candleRepo,
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

	for _, r := range results {
		if r.Percentile < 0 || r.Percentile > 1 {
			t.Errorf("%s: percentile %.4f out of [0,1] range", r.Symbol.String(), r.Percentile)
		}
	}

	if results[0].Percentile != 1.0 {
		t.Errorf("top symbol expected percentile 1.0, got %.4f", results[0].Percentile)
	}
	if results[len(results)-1].Percentile != 0.0 {
		t.Errorf("bottom symbol expected percentile 0.0, got %.4f", results[len(results)-1].Percentile)
	}
}

// TestGetRankings_TrendScoreSignedByDirection verifies that the "Trend
// Predictability" score in the response is negative for downtrending symbols
// and positive for uptrending symbols, enabling directional sorting on the
// frontend.
func TestGetRankings_TrendScoreSignedByDirection(t *testing.T) {
	upSym := domain.NewSymbolUnsafe("UPUSDT")
	downSym := domain.NewSymbolUnsafe("DOWNUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Uptrending: 100 → 130 (sparkline goes up)
	upCandles, _ := domain.NewCandleSeries(upSym, tf, []domain.Candle{
		mustNewCandleAt(upSym, tf, base, 100),
		mustNewCandleAt(upSym, tf, base.Add(time.Hour), 115),
		mustNewCandleAt(upSym, tf, base.Add(2*time.Hour), 130),
	})
	// Downtrending: 130 → 100 (sparkline goes down)
	downCandles, _ := domain.NewCandleSeries(downSym, tf, []domain.Candle{
		mustNewCandleAt(downSym, tf, base, 130),
		mustNewCandleAt(downSym, tf, base.Add(time.Hour), 115),
		mustNewCandleAt(downSym, tf, base.Add(2*time.Hour), 100),
	})

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		upSym:   upCandles,
		downSym: downCandles,
	}, nil)

	trendCalc := &scoring.TrendPredictabilityScoreCalculator{}
	weights := []usecases.ScoreWeight{{Calculator: trendCalc, Weight: 1.0}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	universe := &fakeUniverse{symbols: []domain.Symbol{upSym, downSym}}
	volumes := &fakeVolumes{vols: map[string]float64{
		"UPUSDT": 500, "DOWNUSDT": 500,
	}}

	uc := usecases.NewGetRankings(
		universe, ranker, volumes, candleRepo,
		"http://fake/exchangeInfo", "http://fake/ticker",
		3, usecases.SidewaysAlgoV1, weights, 4, nil,
	)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTrend,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		ts := r.Scores["Trend Predictability"]
		sym := r.Symbol.String()
		switch sym {
		case "UPUSDT":
			if ts <= 0 {
				t.Errorf("UPUSDT: expected positive trend score, got %.6f", ts)
			}
		case "DOWNUSDT":
			if ts >= 0 {
				t.Errorf("DOWNUSDT: expected negative trend score, got %.6f", ts)
			}
		}
	}

	// Verify TotalScore is NOT affected by sign (both use abs internally).
	upTotal := findResult(results, "UPUSDT").TotalScore
	downTotal := findResult(results, "DOWNUSDT").TotalScore
	if math.Abs(upTotal-downTotal) > 1e-9 {
		t.Errorf("TotalScore should be identical for symmetric trends: up=%.6f down=%.6f",
			upTotal, downTotal)
	}
}

func findResult(results []usecases.RankedResult, sym string) usecases.RankedResult {
	for _, r := range results {
		if r.Symbol.String() == sym {
			return r
		}
	}
	panic("symbol not found: " + sym)
}

// recordingCandleRepo records GetLastNCandles n for PR-105 window isolation tests.
type recordingCandleRepo struct {
	inner        *FakeCandleRepository
	ns           []int
	seriesCalls  int
	lastSeriesTo time.Time
}

func (r *recordingCandleRepo) GetSeries(ctx context.Context, symbol domain.Symbol, timeframe domain.Timeframe, from, to time.Time) (domain.CandleSeries, error) {
	r.seriesCalls++
	r.lastSeriesTo = to
	return r.inner.GetSeries(ctx, symbol, timeframe, from, to)
}

func (r *recordingCandleRepo) GetLastNCandles(ctx context.Context, symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error) {
	r.ns = append(r.ns, n)
	return r.inner.GetLastNCandles(ctx, symbol, timeframe, n)
}

func TestGetRankings_PercentileDoesNotWidenSharedSeries(t *testing.T) {
	// 500 bars: early declining, late rising — Rank on full history would
	// disagree with Rank on the last 110. Percentile must fetch 500 but trim
	// before Trend/GainLoss/sparkline.
	const precision = 110
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := make([]domain.Candle, 500)
	px := 200.0
	for i := 0; i < 500; i++ {
		if i < 390 {
			px -= 0.2 // early downtrend
		} else {
			px += 0.5 // late uptrend (precision window)
		}
		bars[i] = mustNewCandleAt(sym, tf, base.Add(time.Duration(i)*time.Hour), px)
	}
	series, err := domain.NewCandleSeries(sym, tf, bars)
	if err != nil {
		t.Fatal(err)
	}

	weights := []usecases.ScoreWeight{
		{Calculator: &scoring.TrendPredictabilityScoreCalculator{}, Weight: 1.0},
		{Calculator: &scoring.GainLossScoreCalculator{}, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)
	universe := &fakeUniverse{symbols: []domain.Symbol{sym}}
	volumes := &fakeVolumes{vols: map[string]float64{"BTCUSDT": 1e6}}

	run := func(algo string) (usecases.RankingsResult, []int) {
		rec := &recordingCandleRepo{inner: NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{sym: series}, nil)}
		uc := usecases.NewGetRankings(
			universe, ranker, volumes, rec,
			"http://fake/exchangeInfo", "http://fake/ticker",
			precision, usecases.SidewaysAlgoV5, weights, 4, nil,
		)
		uc.SetCompressionAlgo(algo)
		out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
			Timeframe: tf,
			Sort:      usecases.SortByTotal,
		})
		if err != nil {
			t.Fatalf("%s: %v", algo, err)
		}
		return out, rec.ns
	}

	absOut, absNs := run("absolute")
	pctOut, pctNs := run("percentile")

	if len(absOut.Results) != 1 || len(pctOut.Results) != 1 {
		t.Fatalf("results abs=%d pct=%d", len(absOut.Results), len(pctOut.Results))
	}
	if len(pctNs) == 0 || pctNs[0] != 500 {
		t.Fatalf("percentile fetch n=%v want 500", pctNs)
	}
	if len(absNs) == 0 || absNs[0] != precision {
		t.Fatalf("absolute fetch n=%v want %d", absNs, precision)
	}

	abs := absOut.Results[0]
	pct := pctOut.Results[0]
	if len(pct.Sparkline) != precision {
		t.Fatalf("percentile sparkline len=%d want %d", len(pct.Sparkline), precision)
	}
	if len(abs.Sparkline) != precision {
		t.Fatalf("absolute sparkline len=%d want %d", len(abs.Sparkline), precision)
	}
	if abs.Scores["Trend Predictability"] != pct.Scores["Trend Predictability"] {
		t.Fatalf("Trend diverged: abs=%g pct=%g", abs.Scores["Trend Predictability"], pct.Scores["Trend Predictability"])
	}
	if abs.Scores["Gain/Loss"] != pct.Scores["Gain/Loss"] {
		t.Fatalf("Gain/Loss diverged: abs=%g pct=%g", abs.Scores["Gain/Loss"], pct.Scores["Gain/Loss"])
	}
	// Compression may differ (that is the point of percentile); breakout boost
	// must stay on absolute calibration → Up/Down match across modes.
	if abs.Scores["Breakout Up"] != pct.Scores["Breakout Up"] ||
		abs.Scores["Breakout Down"] != pct.Scores["Breakout Down"] {
		t.Fatalf("breakout diverged: abs up/down=%g/%g pct=%g/%g",
			abs.Scores["Breakout Up"], abs.Scores["Breakout Down"],
			pct.Scores["Breakout Up"], pct.Scores["Breakout Down"])
	}
}

// TestGetRankings_AsOfUsesHistoricalWindow covers ROADMAP PR-112a: with asOf set,
// FetchCandles uses GetSeries ending at that instant; without asOf, live path
// is unchanged (GetLastN).
func TestGetRankings_AsOfUsesHistoricalWindow(t *testing.T) {
	const precision = 20
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2024, 11, 14, 0, 0, 0, 0, time.UTC) // unix ~1731542400
	bars := make([]domain.Candle, 80)
	px := 100.0
	for i := 0; i < 80; i++ {
		if i < 40 {
			px += 1.0 // early uptrend
		} else {
			px -= 1.0 // late downtrend
		}
		bars[i] = mustNewCandleAt(sym, tf, base.Add(time.Duration(i)*time.Hour), px)
	}
	series, err := domain.NewCandleSeries(sym, tf, bars)
	if err != nil {
		t.Fatal(err)
	}

	weights := []usecases.ScoreWeight{
		{Calculator: &scoring.TrendPredictabilityScoreCalculator{}, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)
	universe := &fakeUniverse{symbols: []domain.Symbol{sym}}
	volumes := &fakeVolumes{vols: map[string]float64{"BTCUSDT": 1e6}}

	rec := &recordingCandleRepo{inner: NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{sym: series}, nil)}
	uc := usecases.NewGetRankings(
		universe, ranker, volumes, rec,
		"http://fake/exchangeInfo", "http://fake/ticker",
		precision, usecases.SidewaysAlgoV5, weights, 4, nil,
	)

	asOf := base.Add(40 * time.Hour) // end of uptrend window
	live, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{Timeframe: tf, Sort: usecases.SortByTotal})
	if err != nil {
		t.Fatal(err)
	}
	replayOut, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByTotal, AsOf: &asOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Results) != 1 || len(replayOut.Results) != 1 {
		t.Fatalf("live=%d replay=%d", len(live.Results), len(replayOut.Results))
	}
	liveTrend := live.Results[0].Scores["Trend Predictability"]
	replayTrend := replayOut.Results[0].Scores["Trend Predictability"]
	// Live window is late downtrend; asOf window is early uptrend — signs should differ.
	if liveTrend >= 0 {
		t.Fatalf("live trend score=%g want negative (late downtrend)", liveTrend)
	}
	if replayTrend <= 0 {
		t.Fatalf("asOf trend score=%g want positive (early uptrend)", replayTrend)
	}
	if replayOut.Results[0].Volume != 0 {
		t.Fatalf("replay volume=%g want 0 (no live ticker)", replayOut.Results[0].Volume)
	}

	volSort, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByVolume, AsOf: &asOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if volSort.Sort != usecases.SortByTotal || volSort.RequestedSort != usecases.SortByVolume {
		t.Fatalf("volume under asOf: sort=%s requested=%s want total/volume", volSort.Sort, volSort.RequestedSort)
	}
}
