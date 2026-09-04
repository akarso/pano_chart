package scoring_test

import (
	"errors"
	"testing"

	"pano_chart/backend/domain"
	infrascoring "pano_chart/backend/infrastructure/scoring"
)

type fakeCalc struct {
	name  string
	score float64
	err   error
}

func (f *fakeCalc) Name() string { return f.name }

func (f *fakeCalc) Score(_ domain.CandleSeries) (float64, error) {
	return f.score, f.err
}

func TestLoggingScoreCalculator_DelegatesNameAndScore(t *testing.T) {
	inner := &fakeCalc{name: "Sideways Consistency", score: 0.42}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1.0)

	if calc.Name() != "Sideways Consistency" {
		t.Errorf("expected Name() to delegate, got %q", calc.Name())
	}

	score, err := calc.Score(domain.CandleSeries{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0.42 {
		t.Errorf("expected Score() to delegate the inner value, got %v", score)
	}
}

func TestLoggingScoreCalculator_PropagatesInnerError(t *testing.T) {
	innerErr := errors.New("boom")
	inner := &fakeCalc{name: "X", err: innerErr}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 1.0)

	_, err := calc.Score(domain.CandleSeries{})
	if !errors.Is(err, innerErr) {
		t.Errorf("expected the inner error to propagate unchanged, got %v", err)
	}
}

func TestLoggingScoreCalculator_ZeroSampleRateNeverPanics(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}
	calc := infrascoring.NewLoggingScoreCalculator(inner, 0)

	for i := 0; i < 50; i++ {
		if _, err := calc.Score(domain.CandleSeries{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestNewLoggingScoreCalculator_ClampsSampleRate(t *testing.T) {
	inner := &fakeCalc{name: "X", score: 0.5}

	// Out-of-range sample rates must not panic or misbehave — just clamp.
	tooHigh := infrascoring.NewLoggingScoreCalculator(inner, 5.0)
	tooLow := infrascoring.NewLoggingScoreCalculator(inner, -5.0)

	if _, err := tooHigh.Score(domain.CandleSeries{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := tooLow.Score(domain.CandleSeries{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
