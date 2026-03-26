package market_test

import (
	"math"
	"testing"

	"pano_chart/backend/domain"
	inframarket "pano_chart/backend/infrastructure/market"
)

func TestEnrichFromSparkline_BasicUptrend(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{100, 101, 102, 103, 104, 105}
	inframarket.EnrichFromSparkline(&snap, sparkline)

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
	inframarket.EnrichFromSparkline(&snap, sparkline)

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
	inframarket.EnrichFromSparkline(&snap, sparkline)

	if snap.Bias != "neutral" {
		t.Errorf("expected Bias=neutral, got %q", snap.Bias)
	}
	if snap.ATR != 0 {
		t.Errorf("expected ATR=0 for flat market, got %f", snap.ATR)
	}
}

func TestEnrichFromSparkline_TooShort(t *testing.T) {
	snap := domain.EvaluationSnapshot{}
	inframarket.EnrichFromSparkline(&snap, []float64{100})

	if snap.Price != 0 {
		t.Errorf("expected no enrichment for single-point sparkline, got Price=%f", snap.Price)
	}
}

func TestEnrichFromSparkline_ATRComputation(t *testing.T) {
	// Sparkline: 100, 102, 100, 102 → moves: 2, 2, 2 → ATR = 2.0
	snap := domain.EvaluationSnapshot{}
	sparkline := []float64{100, 102, 100, 102}
	inframarket.EnrichFromSparkline(&snap, sparkline)

	if math.Abs(snap.ATR-2.0) > 0.01 {
		t.Errorf("expected ATR≈2.0, got %f", snap.ATR)
	}
}
