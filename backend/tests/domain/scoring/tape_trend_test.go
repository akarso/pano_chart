package scoring_test

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestOLSSlopeR2_PerfectLine(t *testing.T) {
	y := []float64{1, 2, 3, 4, 5}
	slope, r2, ok := scoring.OLSSlopeR2(y)
	if !ok {
		t.Fatal("expected ok")
	}
	if math.Abs(slope-1) > 1e-9 {
		t.Fatalf("slope=%v", slope)
	}
	if math.Abs(r2-1) > 1e-9 {
		t.Fatalf("r2=%v", r2)
	}
}

func TestTrueATR_ConstantRange(t *testing.T) {
	sym := domain.NewSymbolUnsafe("T")
	tf, _ := domain.NewTimeframe("1h")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 20)
	for i := range candles {
		v := 100.0
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), v, v+1, v-1, v, 1)
	}
	atr := scoring.TrueATR(candles, 14)
	if math.Abs(atr-2) > 0.01 {
		t.Fatalf("atr=%v want ~2", atr)
	}
}

func TestTrueATR_TooShort(t *testing.T) {
	sym := domain.NewSymbolUnsafe("T")
	tf, _ := domain.NewTimeframe("1h")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 10)
	for i := range candles {
		v := 100.0
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), v, v+1, v-1, v, 1)
	}
	if atr := scoring.TrueATR(candles, 14); atr != 0 {
		t.Fatalf("atr=%v want 0", atr)
	}
}

func TestTapeTrend_CleanUptrend(t *testing.T) {
	closes := make([]float64, 110)
	for i := range closes {
		closes[i] = 100 + float64(i)*0.12
	}
	score, bias := scoring.TapeTrend(closes, 0.5)
	if bias != "up" {
		t.Fatalf("bias=%s", bias)
	}
	if score < 0.5 {
		t.Fatalf("score=%.3f want ≥0.5", score)
	}
}

func TestTapeTrend_GateBoundary(t *testing.T) {
	// Near-flat: Mag and ER keep score below the 0.5 gate.
	closes := make([]float64, 40)
	for i := range closes {
		closes[i] = 100 + float64(i)*0.01
	}
	score, _ := scoring.TapeTrend(closes, 5)
	if score >= 0.5 {
		t.Fatalf("score=%.3f want <0.5 for a crawl", score)
	}
}

func TestTapeTrend_ATRZero(t *testing.T) {
	closes := []float64{100, 110, 120}
	score, bias := scoring.TapeTrend(closes, 0)
	if score != 0 || bias != "neutral" {
		t.Fatalf("score=%.3f bias=%s", score, bias)
	}
}

func TestTapeTrend_FlatIsNeutral(t *testing.T) {
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = 100
	}
	score, bias := scoring.TapeTrend(closes, 1)
	if bias != "neutral" || score != 0 {
		t.Fatalf("score=%.3f bias=%s", score, bias)
	}
}

func TestOLSSlopeR2_TooShort(t *testing.T) {
	if _, _, ok := scoring.OLSSlopeR2([]float64{1}); ok {
		t.Fatal("n<2 should fail")
	}
}

func TestOLSSlopeR2_Flat(t *testing.T) {
	if _, _, ok := scoring.OLSSlopeR2([]float64{3, 3, 3}); ok {
		t.Fatal("flat series should fail")
	}
}
