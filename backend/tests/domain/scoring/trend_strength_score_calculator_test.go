package scoring_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	fixtures "pano_chart/backend/tests/fixtures/candles"
)

func TestTrendStrength_GoldenFixtures(t *testing.T) {
	calc := &scoring.TrendStrengthScoreCalculator{}

	cases := []struct {
		fixture string
		min     float64
		max     float64
		bias    string // empty = don't check
	}{
		{fixture: "messy_uptrend", min: 0.5, max: math.Inf(1), bias: "up"},
		{fixture: "clean_uptrend", min: 0.7, max: math.Inf(1), bias: "up"},
		{fixture: "tight_range", min: math.Inf(-1), max: 0.2},
		{fixture: "v_reversal", min: math.Inf(-1), max: 0.3},
		{fixture: "clean_downtrend", min: 0.5, max: math.Inf(1), bias: "down"},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			series := fixtures.Load(t, tc.fixture)
			got, gotBias, err := calc.ScoreWithDirection(series)
			if err != nil {
				t.Fatal(err)
			}
			if got < tc.min || got > tc.max {
				t.Fatalf("score=%g want [%g, %g]", got, tc.min, tc.max)
			}
			if tc.bias != "" && gotBias != tc.bias {
				t.Fatalf("bias=%q want %q (score=%g)", gotBias, tc.bias, got)
			}
		})
	}
}

func TestTrendStrength_RandomWalkAverageLow(t *testing.T) {
	calc := &scoring.TrendStrengthScoreCalculator{}
	const seeds = 100
	var sum float64
	for seed := int64(0); seed < seeds; seed++ {
		series := syntheticRandomWalk(t, seed, 110)
		got, _, err := calc.ScoreWithDirection(series)
		if err != nil {
			t.Fatal(err)
		}
		sum += got
	}
	avg := sum / seeds
	if avg > 0.25 {
		t.Fatalf("random-walk average=%g want ≤ 0.25", avg)
	}
}

func TestTrendStrength_NameMatchesPredictabilityKey(t *testing.T) {
	if got := (&scoring.TrendStrengthScoreCalculator{}).Name(); got != "Trend Predictability" {
		t.Fatalf("Name=%q want Trend Predictability for weight-key stability", got)
	}
}

func TestTrendStrength_ScoreMatchesScoreWithDirection(t *testing.T) {
	calc := &scoring.TrendStrengthScoreCalculator{}
	series := fixtures.Load(t, "clean_uptrend")
	plain, err := calc.Score(series)
	if err != nil {
		t.Fatal(err)
	}
	dir, _, err := calc.ScoreWithDirection(series)
	if err != nil {
		t.Fatal(err)
	}
	if plain != dir {
		t.Fatalf("Score=%g ScoreWithDirection=%g", plain, dir)
	}
}

func TestTrendStrength_VsPredictability_DocumentedDelta(t *testing.T) {
	// Side-by-side on the PR-088 messy_uptrend fixture. Strength must clear
	// its ROADMAP floor (≥0.5). Predictability is recorded for scorecard
	// comparison — do not require strength > predictability (current golden
	// already scores high under OLS R²).
	strength := &scoring.TrendStrengthScoreCalculator{}
	pred := &scoring.TrendPredictabilityScoreCalculator{}
	series := fixtures.Load(t, "messy_uptrend")
	s, sBias, err := strength.ScoreWithDirection(series)
	if err != nil {
		t.Fatal(err)
	}
	p, pBias, err := pred.ScoreWithDirection(series)
	if err != nil {
		t.Fatal(err)
	}
	if s < 0.5 {
		t.Fatalf("strength messy_uptrend=%g want ≥ 0.5", s)
	}
	if sBias != "up" || pBias != "up" {
		t.Fatalf("biases strength=%q pred=%q want up", sBias, pBias)
	}
	t.Logf("messy_uptrend: strength=%.4f predictability=%.4f", s, p)
	_ = p // documented companion reading for scorecard harness
}

func TestTrendStrength_TinyMoveNeutralBias(t *testing.T) {
	// Nearly flat series: Mag < 0.2 → bias stays neutral even if score > 0.
	calc := &scoring.TrendStrengthScoreCalculator{}
	series := fixtures.Load(t, "flat_dead")
	score, bias, err := calc.ScoreWithDirection(series)
	if err != nil {
		t.Fatal(err)
	}
	if bias != "neutral" {
		t.Fatalf("bias=%q want neutral (score=%g)", bias, score)
	}
}

func syntheticRandomWalk(t *testing.T, seed int64, n int) domain.CandleSeries {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	price := 100.0
	out := make([]domain.Candle, n)
	base := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < n; i++ {
		delta := (rng.Float64() - 0.5) * 0.02 * price
		open := price
		close := price + delta
		if close <= 0 {
			close = price * 0.99
		}
		high := math.Max(open, close) * (1 + rng.Float64()*0.002)
		low := math.Min(open, close) * (1 - rng.Float64()*0.002)
		out[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), open, high, low, close, 1)
		price = close
	}
	series, err := domain.NewCandleSeries(sym, tf, out)
	if err != nil {
		t.Fatal(err)
	}
	return series
}
