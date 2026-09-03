package usecases

import (
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

func TestBuildSnapshot_PopulatesFieldsFromScoresAndSeries(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	candles, err := domain.NewCandleSeries(sym, tf, []domain.Candle{
		mustNewCandleAt(sym, tf, base, 50000),
		mustNewCandleAt(sym, tf, base.Add(time.Hour), 51000),
		mustNewCandleAt(sym, tf, base.Add(2*time.Hour), 52000),
	})
	if err != nil {
		t.Fatalf("failed to create candle series: %v", err)
	}

	scores := map[string]float64{
		"Sideways Consistency": 0.75,
		"Trend Predictability": 0.60,
		"Gain/Loss":            0.40,
	}

	snap := usecases.BuildSnapshot(sym, tf, scores, candles, 5000.0, "v5.0.0")

	if snap.Symbol != "BTCUSDT" {
		t.Errorf("want Symbol BTCUSDT, got %s", snap.Symbol)
	}
	if snap.Timeframe != "1h" {
		t.Errorf("want Timeframe 1h, got %s", snap.Timeframe)
	}
	if snap.SidewaysScore != 0.75 {
		t.Errorf("want SidewaysScore 0.75, got %f", snap.SidewaysScore)
	}
	if snap.TrendScore != 0.60 {
		t.Errorf("want TrendScore 0.60, got %f", snap.TrendScore)
	}
	if snap.AlgoVersion != "v5.0.0" {
		t.Errorf("want AlgoVersion v5.0.0, got %s", snap.AlgoVersion)
	}
	if snap.Volume != 5000.0 {
		t.Errorf("want Volume 5000, got %f", snap.Volume)
	}
	if snap.Price == 0 {
		t.Error("Price should not be 0")
	}
	if snap.ATR == 0 {
		t.Error("ATR should not be 0 for a series with multiple candles")
	}
	if time.Since(snap.Timestamp) > time.Second {
		t.Errorf("Timestamp should be recent, got %v", snap.Timestamp)
	}
}

func TestBuildSnapshot_EmptySeries(t *testing.T) {
	sym := domain.NewSymbolUnsafe("ETHUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	emptySeries, _ := domain.NewCandleSeries(sym, tf, nil)

	scores := map[string]float64{
		"Sideways Consistency": 0.50,
	}

	snap := usecases.BuildSnapshot(sym, tf, scores, emptySeries, 0, "v5.0.0")

	if snap.Price != 0 {
		t.Errorf("expected Price 0 for empty series, got %f", snap.Price)
	}
	if snap.ATR != 0 {
		t.Errorf("expected ATR 0 for empty series, got %f", snap.ATR)
	}
	if snap.SidewaysScore != 0.50 {
		t.Errorf("expected SidewaysScore 0.50, got %f", snap.SidewaysScore)
	}
}

func TestBuildSnapshot_MissingScoreKeysDefaultToZero(t *testing.T) {
	sym := domain.NewSymbolUnsafe("SOLUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	emptySeries, _ := domain.NewCandleSeries(sym, tf, nil)

	snap := usecases.BuildSnapshot(sym, tf, map[string]float64{}, emptySeries, 0, "v5.0.0")

	if snap.SidewaysScore != 0 {
		t.Errorf("expected 0, got %f", snap.SidewaysScore)
	}
	if snap.TrendScore != 0 {
		t.Errorf("expected 0, got %f", snap.TrendScore)
	}
	if snap.CompressionScore != 0 {
		t.Errorf("expected 0, got %f", snap.CompressionScore)
	}
	if snap.BreakoutUpScore != 0 {
		t.Errorf("expected 0, got %f", snap.BreakoutUpScore)
	}
	if snap.BreakoutDownScore != 0 {
		t.Errorf("expected 0, got %f", snap.BreakoutDownScore)
	}
}

func TestBuildSnapshot_CompressionBreakoutFromScores(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("4h")
	emptySeries, _ := domain.NewCandleSeries(sym, tf, nil)

	scores := map[string]float64{
		"Sideways Consistency": 0.5,
		"Trend Predictability": 0.3,
		"Compression":          0.82,
		"Breakout Up":          0.15,
		"Breakout Down":        0.05,
	}

	snap := usecases.BuildSnapshot(sym, tf, scores, emptySeries, 0, "v5.0.0")

	if snap.CompressionScore != 0.82 {
		t.Errorf("want CompressionScore 0.82, got %f", snap.CompressionScore)
	}
	if snap.BreakoutUpScore != 0.15 {
		t.Errorf("want BreakoutUpScore 0.15, got %f", snap.BreakoutUpScore)
	}
	if snap.BreakoutDownScore != 0.05 {
		t.Errorf("want BreakoutDownScore 0.05, got %f", snap.BreakoutDownScore)
	}
}
