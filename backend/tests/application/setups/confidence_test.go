package setups_test

import (
	"math"
	"testing"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/domain/setup"
)

// --- VolatilityFit ---

func TestVolatilityFit_Trend_ModerateIsIdeal(t *testing.T) {
	// Peak at 0.35 for trend regimes.
	got := setups.VolatilityFit("uptrend", 0.35)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestVolatilityFit_Trend_HighVolPenalised(t *testing.T) {
	got := setups.VolatilityFit("downtrend", 0.85)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestVolatilityFit_Trend_ZeroVolPenalised(t *testing.T) {
	got := setups.VolatilityFit("uptrend", 0.0)
	if got >= 0.5 {
		t.Errorf("expected below 0.5, got %f", got)
	}
}

func TestVolatilityFit_Sideways_ModerateIsIdeal(t *testing.T) {
	got := setups.VolatilityFit("sideways", 0.5)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestVolatilityFit_Sideways_ExtremesReduceFit(t *testing.T) {
	low := setups.VolatilityFit("sideways", 0.0)
	high := setups.VolatilityFit("sideways", 1.0)
	if low >= 0.5 || high >= 0.5 {
		t.Errorf("expected low fit at extremes, got low=%f high=%f", low, high)
	}
}

func TestVolatilityFit_Compression_LowVolIdeal(t *testing.T) {
	got := setups.VolatilityFit("compression", 0.0)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestVolatilityFit_Compression_HighVolBad(t *testing.T) {
	got := setups.VolatilityFit("compression", 1.0)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestVolatilityFit_Unknown_ReturnsHalf(t *testing.T) {
	got := setups.VolatilityFit("", 0.5)
	if got != 0.5 {
		t.Errorf("expected 0.5, got %f", got)
	}
}

// --- SeasonalityFit ---

func TestSeasonalityFit_ZeroSpikeProbIsIdeal(t *testing.T) {
	got := setups.SeasonalityFit(0.0)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestSeasonalityFit_AtReferenceThresholdIsZero(t *testing.T) {
	got := setups.SeasonalityFit(0.3) // referenceSpikeProb
	if math.Abs(got-0.0) > 1e-9 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestSeasonalityFit_BeyondThresholdClampsToZero(t *testing.T) {
	got := setups.SeasonalityFit(0.9)
	if got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestSeasonalityFit_MidpointIsHalf(t *testing.T) {
	got := setups.SeasonalityFit(0.15) // half of referenceSpikeProb
	if math.Abs(got-0.5) > 1e-9 {
		t.Errorf("expected 0.5, got %f", got)
	}
}

func TestSeasonalityFit_MonotonicallyDecreasing(t *testing.T) {
	// Unlike VolatilityFit's bell curve, low spike probability is always
	// at least as good as high — never a middle-is-ideal shape.
	low := setups.SeasonalityFit(0.05)
	mid := setups.SeasonalityFit(0.15)
	high := setups.SeasonalityFit(0.25)
	if !(low > mid && mid > high) {
		t.Errorf("expected strictly decreasing fit as spike probability rises: low=%f mid=%f high=%f", low, mid, high)
	}
}

// --- ComputeConfidence: uptrend/downtrend weights ---

func TestComputeConfidence_Trend_AllPerfect(t *testing.T) {
	s := setup.SetupScores{
		Regime:          "uptrend",
		TrendHealth:     1.0,
		MarketEffective: 1.0,
		Crowding:        0.0, // inverted: (1-0)=1
		VolatilityFit:   1.0,
		SeasonalityFit:  1.0,
	}
	got := setups.ComputeConfidence(s)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestComputeConfidence_Trend_AllZero(t *testing.T) {
	s := setup.SetupScores{
		Regime:          "downtrend",
		TrendHealth:     0.0,
		MarketEffective: 0.0,
		Crowding:        1.0, // inverted: (1-1)=0
		VolatilityFit:   0.0,
	}
	got := setups.ComputeConfidence(s)
	if math.Abs(got-0.0) > 1e-9 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestComputeConfidence_Trend_Weights(t *testing.T) {
	// trend=0.4, market=0.3, crowding=0.2, volatility=0.07, seasonality=0.03
	// (PR-082 split the old flat 0.1 "volatility" weight 2:1 between the
	// realized and seasonality signals).
	s := setup.SetupScores{
		Regime:          "uptrend",
		TrendHealth:     0.8,
		MarketEffective: 0.6,
		Crowding:        0.3, // inverted: 0.7
		VolatilityFit:   0.5,
		SeasonalityFit:  0.4,
	}
	expected := 0.4*0.8 + 0.3*0.6 + 0.2*0.7 + 0.07*0.5 + 0.03*0.4
	got := setups.ComputeConfidence(s)
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

// --- ComputeConfidence: sideways weights ---

func TestComputeConfidence_Sideways_Weights(t *testing.T) {
	// trend=0.1, market=0.3, crowding=0.3, volatility=0.2, seasonality=0.1
	// (PR-082 split the old flat 0.3 "volatility" weight 2:1).
	s := setup.SetupScores{
		Regime:          "sideways",
		TrendHealth:     0.5,
		MarketEffective: 0.7,
		Crowding:        0.2, // inverted: 0.8
		VolatilityFit:   0.9,
		SeasonalityFit:  0.6,
	}
	expected := 0.1*0.5 + 0.3*0.7 + 0.3*0.8 + 0.2*0.9 + 0.1*0.6
	got := setups.ComputeConfidence(s)
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

// --- ComputeConfidence: compression weights ---

func TestComputeConfidence_Compression_Weights(t *testing.T) {
	// trend=0.2, market=0.3, crowding=0.2, volatility=0.2, seasonality=0.1
	// (PR-082 split the old flat 0.3 "volatility" weight 2:1).
	s := setup.SetupScores{
		Regime:          "compression",
		TrendHealth:     0.3,
		MarketEffective: 0.5,
		Crowding:        0.4, // inverted: 0.6
		VolatilityFit:   0.8,
		SeasonalityFit:  0.7,
	}
	expected := 0.2*0.3 + 0.3*0.5 + 0.2*0.6 + 0.2*0.8 + 0.1*0.7
	got := setups.ComputeConfidence(s)
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

// --- ComputeConfidence: seasonality (PR-082) ---

// TestComputeConfidence_HighSeasonalityRisk_LowersConfidence is the
// regression test PR-082's own spec called for: proving a setup with
// otherwise-identical scores reads as less confident when the current
// time-of-day's historical spike probability is high than when it's low.
func TestComputeConfidence_HighSeasonalityRisk_LowersConfidence(t *testing.T) {
	base := setup.SetupScores{
		Regime:          "sideways",
		TrendHealth:     0.5,
		MarketEffective: 0.6,
		Crowding:        0.2,
		VolatilityFit:   0.8,
		SeasonalityFit:  1.0, // historically calm right now
	}
	risky := base
	risky.SeasonalityFit = 0.0 // historically one of the riskiest moments of the day

	confHigh := setups.ComputeConfidence(base)
	confRisky := setups.ComputeConfidence(risky)
	if confRisky >= confHigh {
		t.Errorf("high seasonality risk should lower confidence: calm=%f risky=%f", confHigh, confRisky)
	}
}

// --- ComputeConfidence: unknown regime ---

func TestComputeConfidence_Unknown_ReturnsHalf(t *testing.T) {
	s := setup.SetupScores{Regime: ""}
	got := setups.ComputeConfidence(s)
	if got != 0.5 {
		t.Errorf("expected 0.5, got %f", got)
	}
}

// --- ComputeConfidence: clamping ---

func TestComputeConfidence_ClampsToZeroOne(t *testing.T) {
	s := setup.SetupScores{
		Regime:          "uptrend",
		TrendHealth:     1.5, // edge: above 1
		MarketEffective: 1.0,
		Crowding:        -0.5, // edge: below 0 → inverted = 1.5
		VolatilityFit:   1.0,
	}
	got := setups.ComputeConfidence(s)
	if got > 1.0 {
		t.Errorf("expected clamped to 1.0, got %f", got)
	}
}

// --- ConfidenceLabel ---

func TestConfidenceLabel_High(t *testing.T) {
	if got := setups.ConfidenceLabel(0.8); got != "High" {
		t.Errorf("expected High, got %s", got)
	}
}

func TestConfidenceLabel_Medium(t *testing.T) {
	if got := setups.ConfidenceLabel(0.6); got != "Medium" {
		t.Errorf("expected Medium, got %s", got)
	}
}

func TestConfidenceLabel_Low(t *testing.T) {
	if got := setups.ConfidenceLabel(0.4); got != "Low" {
		t.Errorf("expected Low, got %s", got)
	}
}

// --- Crowding inversion ---

func TestComputeConfidence_HighCrowding_LowersConfidence(t *testing.T) {
	base := setup.SetupScores{
		Regime:          "uptrend",
		TrendHealth:     0.8,
		MarketEffective: 0.7,
		Crowding:        0.0,
		VolatilityFit:   0.8,
	}
	crowded := base
	crowded.Crowding = 0.9

	confLow := setups.ComputeConfidence(crowded)
	confHigh := setups.ComputeConfidence(base)
	if confLow >= confHigh {
		t.Errorf("high crowding should lower confidence: low=%f high=%f", confLow, confHigh)
	}
}
