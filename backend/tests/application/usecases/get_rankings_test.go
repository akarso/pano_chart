package usecases

import (
	"context"
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
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
	)

	results, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
		2, usecases.SidewaysAlgoV1, weights, 4,
	)

	results, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
		2, usecases.SidewaysAlgoV1, weights, 4,
	)

	results, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
