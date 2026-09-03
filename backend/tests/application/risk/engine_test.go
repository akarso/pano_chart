package risk_test

import (
	"math"
	"testing"

	"pano_chart/backend/application/risk"
	domainrisk "pano_chart/backend/domain/risk"
)

const tolerance = 0.0001

func approx(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s: expected %.4f, got %.4f", label, want, got)
	}
}

// --- Funding extremeness ---

func TestFundingExtremeness_Zero(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 100)
	approx(t, "funding=0", c.FundingExtremeness, 0)
}

func TestFundingExtremeness_MaxPositive(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0.01, nil, 0.5, 100, 100)
	approx(t, "funding=0.01", c.FundingExtremeness, 1)
}

func TestFundingExtremeness_MaxNegative(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(-0.01, nil, 0.5, 100, 100)
	approx(t, "funding=-0.01", c.FundingExtremeness, 1)
}

func TestFundingExtremeness_HalfRate(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0.005, nil, 0.5, 100, 100)
	approx(t, "funding=0.005", c.FundingExtremeness, 0.5)
}

func TestFundingExtremeness_CappedAbove(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0.05, nil, 0.5, 100, 100)
	approx(t, "funding=0.05 capped", c.FundingExtremeness, 1)
}

// --- OI expansion ---

func TestOIExpansion_TooFewPoints(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, []float64{1, 2, 3}, 0.5, 100, 100)
	approx(t, "short series", c.OIExpansion, 0)
}

func TestOIExpansion_NoGrowth(t *testing.T) {
	eng := risk.NewEngine()
	series := make([]float64, 10)
	for i := range series {
		series[i] = 100
	}
	c := eng.Calculate(0, series, 0.5, 100, 100)
	approx(t, "flat OI", c.OIExpansion, 0)
}

func TestOIExpansion_FiftyPercentGrowth(t *testing.T) {
	eng := risk.NewEngine()
	series := []float64{100, 105, 110, 115, 120, 125, 130, 135, 140, 150}
	c := eng.Calculate(0, series, 0.5, 100, 100)
	// growth = (150-100)/100 = 0.5
	approx(t, "50% growth", c.OIExpansion, 0.5)
}

func TestOIExpansion_Decline(t *testing.T) {
	eng := risk.NewEngine()
	series := []float64{100, 95, 90, 85, 80, 75, 70, 65, 60, 50}
	c := eng.Calculate(0, series, 0.5, 100, 100)
	approx(t, "declining OI", c.OIExpansion, 0)
}

func TestOIExpansion_CappedAtOne(t *testing.T) {
	eng := risk.NewEngine()
	series := []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 300}
	c := eng.Calculate(0, series, 0.5, 100, 100)
	approx(t, "200% growth capped", c.OIExpansion, 1)
}

func TestOIExpansion_StartZero(t *testing.T) {
	eng := risk.NewEngine()
	series := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90}
	c := eng.Calculate(0, series, 0.5, 100, 100)
	approx(t, "start=0", c.OIExpansion, 0)
}

// --- Imbalance ---

func TestImbalance_Balanced(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 100)
	approx(t, "balanced", c.LongShortImbalance, 0)
}

func TestImbalance_AllLong(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 1.0, 100, 100)
	approx(t, "all long", c.LongShortImbalance, 1)
}

func TestImbalance_AllShort(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.0, 100, 100)
	approx(t, "all short", c.LongShortImbalance, 1)
}

func TestImbalance_SlightlyLong(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.7, 100, 100)
	// diff=0.2; 0.2*2 = 0.4
	approx(t, "0.7 long", c.LongShortImbalance, 0.4)
}

// --- Liquidation proximity ---

func TestLiquidationProximity_AtCluster(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 100)
	approx(t, "price=cluster", c.LiquidationProximity, 1)
}

func TestLiquidationProximity_FarAway(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 120)
	// distance=20, ratio=0.2, score=1-2=0 → clamped
	approx(t, "far cluster", c.LiquidationProximity, 0)
}

func TestLiquidationProximity_FivePercent(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 105)
	// distance=5, ratio=0.05, score=1-0.5=0.5
	approx(t, "5% away", c.LiquidationProximity, 0.5)
}

func TestLiquidationProximity_PriceZero(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 0, 100)
	approx(t, "price=0", c.LiquidationProximity, 0)
}

func TestLiquidationProximity_ClusterBelow(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0, nil, 0.5, 100, 95)
	// distance=5, ratio=0.05, score=1-0.5=0.5
	approx(t, "cluster below", c.LiquidationProximity, 0.5)
}

// --- FinalScore ---

func TestFinalScore_AllZero(t *testing.T) {
	c := domainrisk.FragilityComponents{}
	approx(t, "all zero", risk.FinalScore(c), 0)
}

func TestFinalScore_AllOne(t *testing.T) {
	c := domainrisk.FragilityComponents{
		FundingExtremeness:   1,
		OIExpansion:          1,
		LongShortImbalance:   1,
		LiquidationProximity: 1,
	}
	approx(t, "all one", risk.FinalScore(c), 1)
}

func TestFinalScore_WeightedMix(t *testing.T) {
	c := domainrisk.FragilityComponents{
		FundingExtremeness:   0.5,
		OIExpansion:          0.8,
		LongShortImbalance:   0.3,
		LiquidationProximity: 0.6,
	}
	// 0.5*0.25 + 0.8*0.30 + 0.3*0.20 + 0.6*0.25 = 0.125 + 0.24 + 0.06 + 0.15 = 0.575
	approx(t, "weighted mix", risk.FinalScore(c), 0.575)
}

// --- RiskLevel ---

func TestRiskLevel_Low(t *testing.T) {
	if got := risk.RiskLevel(0.2); got != "low" {
		t.Errorf("expected low, got %s", got)
	}
}

func TestRiskLevel_Medium(t *testing.T) {
	if got := risk.RiskLevel(0.5); got != "medium" {
		t.Errorf("expected medium, got %s", got)
	}
}

func TestRiskLevel_High(t *testing.T) {
	if got := risk.RiskLevel(0.8); got != "high" {
		t.Errorf("expected high, got %s", got)
	}
}

func TestRiskLevel_BoundaryHigh(t *testing.T) {
	if got := risk.RiskLevel(0.7); got != "medium" {
		t.Errorf("expected medium at 0.7, got %s", got)
	}
	if got := risk.RiskLevel(0.71); got != "high" {
		t.Errorf("expected high at 0.71, got %s", got)
	}
}

func TestRiskLevel_BoundaryMedium(t *testing.T) {
	if got := risk.RiskLevel(0.4); got != "low" {
		t.Errorf("expected low at 0.4, got %s", got)
	}
	if got := risk.RiskLevel(0.41); got != "medium" {
		t.Errorf("expected medium at 0.41, got %s", got)
	}
}

// --- DominantSide ---

func TestDominantSide_Long(t *testing.T) {
	if got := risk.DominantSide(0.005, 0.7); got != "long" {
		t.Errorf("expected long, got %s", got)
	}
}

func TestDominantSide_Short(t *testing.T) {
	if got := risk.DominantSide(-0.005, 0.3); got != "short" {
		t.Errorf("expected short, got %s", got)
	}
}

func TestDominantSide_NeutralBalanced(t *testing.T) {
	if got := risk.DominantSide(0.005, 0.5); got != "neutral" {
		t.Errorf("expected neutral, got %s", got)
	}
}

func TestDominantSide_NeutralZeroFunding(t *testing.T) {
	if got := risk.DominantSide(0, 0.8); got != "neutral" {
		t.Errorf("expected neutral, got %s", got)
	}
}

func TestDominantSide_NeutralMixedSignals(t *testing.T) {
	// Positive funding but short-heavy ratio → conflicting → neutral
	if got := risk.DominantSide(0.005, 0.3); got != "neutral" {
		t.Errorf("expected neutral, got %s", got)
	}
}

func TestDominantSide_BoundaryLong(t *testing.T) {
	// longRatio exactly 0.6 is NOT > 0.6
	if got := risk.DominantSide(0.001, 0.6); got != "neutral" {
		t.Errorf("expected neutral at boundary 0.6, got %s", got)
	}
	if got := risk.DominantSide(0.001, 0.61); got != "long" {
		t.Errorf("expected long at 0.61, got %s", got)
	}
}

func TestDominantSide_BoundaryShort(t *testing.T) {
	// longRatio exactly 0.4 is NOT < 0.4
	if got := risk.DominantSide(-0.001, 0.4); got != "neutral" {
		t.Errorf("expected neutral at boundary 0.4, got %s", got)
	}
	if got := risk.DominantSide(-0.001, 0.39); got != "short" {
		t.Errorf("expected short at 0.39, got %s", got)
	}
}

// --- SqueezeRisk ---

func TestSqueezeRisk_LongSqueeze(t *testing.T) {
	if got := risk.SqueezeRisk("long"); got != "long_squeeze" {
		t.Errorf("expected long_squeeze, got %s", got)
	}
}

func TestSqueezeRisk_ShortSqueeze(t *testing.T) {
	if got := risk.SqueezeRisk("short"); got != "short_squeeze" {
		t.Errorf("expected short_squeeze, got %s", got)
	}
}

func TestSqueezeRisk_None(t *testing.T) {
	if got := risk.SqueezeRisk("neutral"); got != "none" {
		t.Errorf("expected none, got %s", got)
	}
}

// --- Engine.Calculate integration ---

func TestEngineCalculate_ReturnsAllComponents(t *testing.T) {
	eng := risk.NewEngine()
	c := eng.Calculate(0.005, []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 200}, 0.7, 100, 105)

	approx(t, "funding", c.FundingExtremeness, 0.5)
	approx(t, "oi", c.OIExpansion, 1.0) // (200-100)/100 = 1.0
	approx(t, "imbalance", c.LongShortImbalance, 0.4)
	approx(t, "liquidation", c.LiquidationProximity, 0.5)
}

// --- All components in [0,1] ---

func TestAllComponentsInRange(t *testing.T) {
	eng := risk.NewEngine()
	cases := []struct {
		name    string
		funding float64
		oi      []float64
		lr      float64
		price   float64
		cluster float64
	}{
		{"extreme", 0.1, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 100}, 1.0, 100, 100},
		{"zero", 0, nil, 0.5, 100, 100},
		{"negative", -0.02, []float64{100, 90, 80, 70, 60, 50, 40, 30, 20, 10}, 0.0, 100, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := eng.Calculate(tc.funding, tc.oi, tc.lr, tc.price, tc.cluster)
			for _, v := range []float64{c.FundingExtremeness, c.OIExpansion, c.LongShortImbalance, c.LiquidationProximity} {
				if v < 0 || v > 1 {
					t.Errorf("component out of [0,1]: %f", v)
				}
			}
		})
	}
}
