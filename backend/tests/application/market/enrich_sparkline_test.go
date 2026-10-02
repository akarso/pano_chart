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
	// Constant |Δclose|=1 for 20 bars → Wilder ATR14 ≈ 1 (not mean of full window alone).
	spark := make([]float64, 20)
	for i := range spark {
		spark[i] = 100 + float64(i)
	}
	snap := domain.EvaluationSnapshot{}
	appmarket.EnrichFromSparkline(&snap, spark)
	if math.Abs(snap.ATR-1.0) > 0.05 {
		t.Fatalf("expected Wilder ATR≈1.0, got %f", snap.ATR)
	}
}

func TestEnrichFromSparkline_RecentReturnIsFullWindow(t *testing.T) {
	// Full-window return in ATR units — not the 8-bar health tail.
	spark := make([]float64, 20)
	for i := range spark {
		spark[i] = 100 + float64(i) // +19 over window; ATR≈1 → RecentReturn≈19
	}
	snap := domain.EvaluationSnapshot{}
	appmarket.EnrichFromSparkline(&snap, spark)
	want := (spark[19] - spark[0]) / snap.ATR
	if math.Abs(snap.RecentReturn-want) > 0.01 {
		t.Fatalf("RecentReturn=%f want full-window %f", snap.RecentReturn, want)
	}
	if snap.RecentReturn < 10 {
		t.Fatalf("full-window return should be ≫ 8-bar tail scale, got %f", snap.RecentReturn)
	}
}
