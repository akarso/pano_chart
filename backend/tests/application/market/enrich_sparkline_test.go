package market_test

import (
	"math"
	"testing"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
)

func TestEnrichFromSparkline_BasicUptrend(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{100, 101, 102, 103, 104, 105}
	appmarket.EnrichFromSparkline(&snap, sparkline)

	if snap.Price != 105 {
		t.Errorf("expected Price=105, got %f", snap.Price)
	}
	if snap.Bias != "up" {
		t.Errorf("expected Bias=up, got %q", snap.Bias)
	}
	if snap.RecentHigh != 105 {
		t.Errorf("expected RecentHigh=105, got %f", snap.RecentHigh)
	}
	if snap.RecentLow != 100 {
		t.Errorf("expected RecentLow=100, got %f", snap.RecentLow)
	}
	if snap.ATR <= 0 {
		t.Errorf("expected positive ATR, got %f", snap.ATR)
	}
	if snap.RecentReturn <= 0 {
		t.Errorf("expected positive RecentReturn for uptrend, got %f", snap.RecentReturn)
	}
}

func TestEnrichFromSparkline_Downtrend(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{105, 103, 101, 99, 97}
	appmarket.EnrichFromSparkline(&snap, sparkline)

	if snap.Bias != "down" {
		t.Errorf("expected Bias=down, got %q", snap.Bias)
	}
	if snap.RecentReturn >= 0 {
		t.Errorf("expected negative RecentReturn for downtrend, got %f", snap.RecentReturn)
	}
}

func TestEnrichFromSparkline_FlatMarket(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{100, 100, 100}
	appmarket.EnrichFromSparkline(&snap, sparkline)

	if snap.Bias != "neutral" {
		t.Errorf("expected Bias=neutral, got %q", snap.Bias)
	}
	if snap.ATR != 0 {
		t.Errorf("expected ATR=0 for flat market, got %f", snap.ATR)
	}
}

func TestEnrichFromSparkline_TooShort(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	appmarket.EnrichFromSparkline(&snap, []float64{100})

	if snap.Price != 0 {
		t.Errorf("expected no enrichment for single-point sparkline, got Price=%f", snap.Price)
	}
}

func TestEnrichFromSparkline_ATRComputation(t *testing.T) {
	// Sparkline: 100, 102, 100, 102 → moves: 2, 2, 2 → mean |Δ| ATR = 2.0
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{100, 102, 100, 102}
	appmarket.EnrichFromSparkline(&snap, sparkline)

	if math.Abs(snap.ATR-2.0) > 0.01 {
		t.Errorf("expected ATR≈2.0, got %f", snap.ATR)
	}
}

func TestEnrichFromSparkline_WilderATR14WhenLong(t *testing.T) {
	// Early large steps, then tiny ones: mean |Δ| stays elevated; Wilder
	// decays toward recent small TRs — formulas must diverge.
	spark := make([]float64, 30)
	spark[0] = 100
	for i := 1; i < 30; i++ {
		step := 0.1
		if i <= 5 {
			step = 10
		}
		spark[i] = spark[i-1] + step
	}
	var absSum float64
	for i := 1; i < len(spark); i++ {
		absSum += math.Abs(spark[i] - spark[i-1])
	}
	meanAbs := absSum / float64(len(spark)-1)

	snap := domain.EvaluationSnapshot{}
	appmarket.EnrichFromSparkline(&snap, spark)
	if math.Abs(snap.ATR-meanAbs) < 0.4 {
		t.Fatalf("Wilder ATR=%f should diverge from mean|Δ|=%f", snap.ATR, meanAbs)
	}
	if snap.ATR <= 0 || snap.ATR >= meanAbs {
		t.Fatalf("expected 0 < Wilder ATR < mean|Δ|, got ATR=%f mean=%f", snap.ATR, meanAbs)
	}
}

func TestEnrichFromSparkline_RecentReturnUsesMeanAbsDenom(t *testing.T) {
	// Full-window return / mean|Δ| — not Wilder ATR (silent/bias scale).
	spark := make([]float64, 30)
	spark[0] = 100
	for i := 1; i < 30; i++ {
		step := 0.1
		if i <= 5 {
			step = 10
		}
		spark[i] = spark[i-1] + step
	}
	var absSum float64
	for i := 1; i < len(spark); i++ {
		absSum += math.Abs(spark[i] - spark[i-1])
	}
	meanAbs := absSum / float64(len(spark)-1)

	snap := domain.EvaluationSnapshot{}
	appmarket.EnrichFromSparkline(&snap, spark)
	want := (spark[len(spark)-1] - spark[0]) / meanAbs
	if math.Abs(snap.RecentReturn-want) > 0.01 {
		t.Fatalf("RecentReturn=%f want mean|Δ| scale %f (ATR=%f)", snap.RecentReturn, want, snap.ATR)
	}
	wilderReturn := (spark[len(spark)-1] - spark[0]) / snap.ATR
	if math.Abs(snap.RecentReturn-wilderReturn) < 0.2 {
		t.Fatalf("RecentReturn should differ from Wilder-scaled %f, got %f", wilderReturn, snap.RecentReturn)
	}
}
