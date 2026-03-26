package setups_test

import (
	"math"
	"testing"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/domain/setup"
)

// --- ComputeBreakoutProbability ---

func TestComputeBreakoutProbability_FullConfidence(t *testing.T) {
	// confidence 1.0 → adjustment = 1.0, probability unchanged.
	got := setups.ComputeBreakoutProbability(0.6, 1.0)
	if math.Abs(got-0.6) > 1e-9 {
		t.Errorf("expected 0.6, got %f", got)
	}
}

func TestComputeBreakoutProbability_HalfConfidence(t *testing.T) {
	// confidence 0.5 → adjustment = 0.75, probability = 0.6 * 0.75 = 0.45.
	got := setups.ComputeBreakoutProbability(0.6, 0.5)
	if math.Abs(got-0.45) > 1e-9 {
		t.Errorf("expected 0.45, got %f", got)
	}
}

func TestComputeBreakoutProbability_ZeroConfidence(t *testing.T) {
	// confidence 0.0 → adjustment = 0.5, then low-confidence floor × 0.8.
	// probability = 0.6 * 0.5 * 0.8 = 0.24.
	got := setups.ComputeBreakoutProbability(0.6, 0.0)
	if math.Abs(got-0.24) > 1e-9 {
		t.Errorf("expected 0.24, got %f", got)
	}
}

func TestComputeBreakoutProbability_LowConfidenceFloor(t *testing.T) {
	// confidence 0.29 (below 0.3) triggers floor penalty.
	// adjustment = 0.5 + 0.5*0.29 = 0.645
	// prob = 0.8 * 0.645 * 0.8 = 0.4128.
	got := setups.ComputeBreakoutProbability(0.8, 0.29)
	expected := 0.8 * (0.5 + 0.5*0.29) * 0.8
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

func TestComputeBreakoutProbability_AboveFloorThreshold(t *testing.T) {
	// confidence 0.3 → no floor penalty.
	// adjustment = 0.5 + 0.5*0.3 = 0.65.
	// prob = 0.8 * 0.65 = 0.52.
	got := setups.ComputeBreakoutProbability(0.8, 0.3)
	if math.Abs(got-0.52) > 1e-9 {
		t.Errorf("expected 0.52, got %f", got)
	}
}

func TestComputeBreakoutProbability_ClampedAtOne(t *testing.T) {
	// Even with base > 1 the result should clamp to 1.0.
	got := setups.ComputeBreakoutProbability(1.5, 1.0)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestComputeBreakoutProbability_ClampedAtZero(t *testing.T) {
	got := setups.ComputeBreakoutProbability(-0.5, 1.0)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestComputeBreakoutProbability_ZeroBase(t *testing.T) {
	got := setups.ComputeBreakoutProbability(0.0, 0.8)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

// --- ApplyBreakoutConfidence ---

func TestApplyBreakoutConfidence_BothDirections(t *testing.T) {
	s := setup.SetupScores{
		TrendHealth: 0.8,
		Confidence:  0.8,
	}
	result := setups.ApplyBreakoutConfidence(s, 0.5, 0.4)
	expectedUp := setups.ComputeBreakoutProbability(0.5, 0.8)
	expectedDown := setups.ComputeBreakoutProbability(0.4, 0.8)
	if math.Abs(result.BreakoutUp-expectedUp) > 1e-9 {
		t.Errorf("BreakoutUp: expected %f, got %f", expectedUp, result.BreakoutUp)
	}
	if math.Abs(result.BreakoutDown-expectedDown) > 1e-9 {
		t.Errorf("BreakoutDown: expected %f, got %f", expectedDown, result.BreakoutDown)
	}
}

func TestApplyBreakoutConfidence_DirectionalBias_WeakTrend(t *testing.T) {
	// When trend health < 0.4, up-breakout is penalised by 0.7.
	s := setup.SetupScores{
		TrendHealth: 0.3,
		Confidence:  1.0,
	}
	result := setups.ApplyBreakoutConfidence(s, 1.0, 1.0)
	// Up penalised: 1.0 * 0.7 = 0.7, then confidence=1 → prob = 0.7.
	if math.Abs(result.BreakoutUp-0.7) > 1e-9 {
		t.Errorf("BreakoutUp: expected 0.7, got %f", result.BreakoutUp)
	}
	// Down not penalised: 1.0, confidence=1 → prob = 1.0.
	if math.Abs(result.BreakoutDown-1.0) > 1e-9 {
		t.Errorf("BreakoutDown: expected 1.0, got %f", result.BreakoutDown)
	}
}

func TestApplyBreakoutConfidence_DirectionalBias_HealthyTrend(t *testing.T) {
	// When trend health >= 0.4, no directional penalty.
	s := setup.SetupScores{
		TrendHealth: 0.4,
		Confidence:  1.0,
	}
	result := setups.ApplyBreakoutConfidence(s, 0.8, 0.8)
	expected := setups.ComputeBreakoutProbability(0.8, 1.0)
	if math.Abs(result.BreakoutUp-expected) > 1e-9 {
		t.Errorf("BreakoutUp: expected %f, got %f", expected, result.BreakoutUp)
	}
	if math.Abs(result.BreakoutDown-expected) > 1e-9 {
		t.Errorf("BreakoutDown: expected %f, got %f", expected, result.BreakoutDown)
	}
}

func TestApplyBreakoutConfidence_PreservesExistingFields(t *testing.T) {
	s := setup.SetupScores{
		Symbol:      "BTC",
		Timeframe:   "4h",
		BestSetup:   setup.CompressionBreakout,
		Score:       0.85,
		TrendHealth: 0.6,
		Confidence:  0.7,
	}
	result := setups.ApplyBreakoutConfidence(s, 0.5, 0.3)
	if result.Symbol != "BTC" {
		t.Errorf("Symbol: expected BTC, got %s", result.Symbol)
	}
	if result.Timeframe != "4h" {
		t.Errorf("Timeframe: expected 4h, got %s", result.Timeframe)
	}
	if result.Score != 0.85 {
		t.Errorf("Score: expected 0.85, got %f", result.Score)
	}
}

func TestApplyBreakoutConfidence_ZeroRawScores(t *testing.T) {
	s := setup.SetupScores{
		TrendHealth: 0.8,
		Confidence:  0.9,
	}
	result := setups.ApplyBreakoutConfidence(s, 0.0, 0.0)
	if result.BreakoutUp != 0.0 {
		t.Errorf("BreakoutUp: expected 0.0, got %f", result.BreakoutUp)
	}
	if result.BreakoutDown != 0.0 {
		t.Errorf("BreakoutDown: expected 0.0, got %f", result.BreakoutDown)
	}
}
