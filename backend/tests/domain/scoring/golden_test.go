package scoring_test

import (
	"math"
	"testing"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/tests/fixtures/candles"
)

// scoreBound pins a calculator output range for a golden fixture.
// Min/Max are inclusive. Use Abs when the calculator is signed (Trend).
type scoreBound struct {
	Name string
	Min  float64 // math.Inf(-1) = no floor
	Max  float64 // math.Inf(+1) = no ceiling
	Abs  bool    // compare |score| against Min/Max
}

func TestGoldenFixtures_ScoreRanges(t *testing.T) {
	cases := []struct {
		fixture string
		bounds  []scoreBound
	}{
		{
			fixture: "clean_uptrend",
			bounds: []scoreBound{
				{Name: "Trend Predictability", Min: 0.6, Max: math.Inf(1), Abs: true},
				{Name: "Sideways Consistency", Min: math.Inf(-1), Max: 0.3},
				{Name: "Gain/Loss", Min: 0.05, Max: 0.20},
			},
		},
		{
			fixture: "messy_uptrend",
			bounds: []scoreBound{
				{Name: "Trend Predictability", Min: 0.5, Max: math.Inf(1), Abs: true},
				{Name: "Sideways Consistency", Min: math.Inf(-1), Max: 0.3},
				{Name: "Gain/Loss", Min: 0.05, Max: 0.15},
			},
		},
		{
			fixture: "clean_downtrend",
			bounds: []scoreBound{
				{Name: "Trend Predictability", Min: 0.6, Max: math.Inf(1), Abs: true},
				{Name: "Sideways Consistency", Min: math.Inf(-1), Max: 0.3},
				{Name: "Gain/Loss", Min: -0.20, Max: -0.05},
			},
		},
		{
			fixture: "tight_range",
			bounds: []scoreBound{
				{Name: "Sideways Consistency", Min: 0.4, Max: math.Inf(1)},
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.3, Abs: true},
				{Name: "Compression", Min: math.Inf(-1), Max: 0.25},
			},
		},
		{
			fixture: "wide_range",
			bounds: []scoreBound{
				{Name: "Sideways Consistency", Min: 0.4, Max: math.Inf(1)},
				{Name: "Compression", Min: math.Inf(-1), Max: 0.25},
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.3, Abs: true},
			},
		},
		{
			fixture: "compression_pre_breakout",
			bounds: []scoreBound{
				{Name: "Compression", Min: 0.3, Max: math.Inf(1)},
				{Name: "Sideways Consistency", Min: math.Inf(-1), Max: 0.35},
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.3, Abs: true},
			},
		},
		{
			fixture: "breakout_up_with_volume",
			bounds: []scoreBound{
				// Channel + piercing bar with volume (DetectBreakout BVS/CCS).
				{Name: "Breakout", Min: 0.2, Max: math.Inf(1)},
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.35, Abs: true},
			},
		},
		{
			fixture: "failed_breakout",
			bounds: []scoreBound{
				// Fake pierce that re-enters — Breakout must stay near zero.
				{Name: "Breakout", Min: math.Inf(-1), Max: 0.05},
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.3, Abs: true},
				{Name: "Gain/Loss", Min: -0.05, Max: 0.05},
			},
		},
		{
			fixture: "v_reversal",
			bounds: []scoreBound{
				// Sharp V: net positive but not a clean linear trend.
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.35, Abs: true},
				{Name: "Gain/Loss", Min: 0.02, Max: 0.15},
				{Name: "Breakout", Min: math.Inf(-1), Max: 0.15},
			},
		},
		{
			fixture: "flat_dead",
			bounds: []scoreBound{
				{Name: "Trend Predictability", Min: math.Inf(-1), Max: 0.2, Abs: true},
				{Name: "Gain/Loss", Min: -0.01, Max: 0.01},
				{Name: "Sideways Consistency", Min: 0.3, Max: math.Inf(1)},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			series := fixtures.Load(t, tc.fixture)
			calcs := goldenCalculators(t, series)
			for _, b := range tc.bounds {
				calc, ok := calcs[b.Name]
				if !ok {
					t.Fatalf("unknown calculator %q", b.Name)
				}
				got, err := calc.Score(series)
				if err != nil {
					t.Fatalf("%s.Score: %v", b.Name, err)
				}
				compare := got
				if b.Abs {
					compare = math.Abs(got)
				}
				if compare < b.Min || compare > b.Max {
					t.Errorf("%s score=%g (compare=%g) want [%g, %g] abs=%v",
						b.Name, got, compare, b.Min, b.Max, b.Abs)
				}
			}
		})
	}
}

func goldenCalculators(t *testing.T, series domain.CandleSeries) map[string]scoring.SymbolScoreCalculator {
	t.Helper()
	tf := series.Timeframe().String()
	comp := &scoring.CompressionScoreCalculator{Config: scoring.DefaultCompressionConfig()}
	compScore, err := comp.Score(series)
	if err != nil {
		t.Fatalf("Compression.Score: %v", err)
	}
	return map[string]scoring.SymbolScoreCalculator{
		"Trend Predictability": &scoring.TrendPredictabilityScoreCalculator{},
		"Sideways Consistency": &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe(tf),
		},
		"Compression": comp,
		"Breakout": &scoring.BreakoutScoreCalculator{
			Config:           scoring.DefaultBreakoutConfig(),
			CompressionScore: compScore,
		},
		"Gain/Loss": &scoring.GainLossScoreCalculator{},
	}
}
