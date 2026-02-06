package usecases

import (
	"pano_chart/backend/domain"
	"testing"
)

type stubCalculator struct {
	name  string
	value float64
}

func (s *stubCalculator) Name() string                 { return s.name }
func (s *stubCalculator) Score(domain.CandleSeries) (float64, error) { return s.value, nil }

func makeSymbol(name string) domain.Symbol {
	s, _ := domain.NewSymbol(name)
	return s
}

func makeSeries() domain.CandleSeries {
	// Not used by stub, just needs to be non-nil
	return domain.CandleSeries{}
}

func TestRankSymbols_SingleCalculatorUniformWeight(t *testing.T) {
	calc := &stubCalculator{"gain", 1.0}
	weights := []ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{
		makeSymbol("A"): makeSeries(),
		makeSymbol("B"): makeSeries(),
	}
	res, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	for _, r := range res {
		if r.TotalScore != 1.0 {
			t.Errorf("expected total score 1.0, got %v", r.TotalScore)
		}
	}
}

func TestRankSymbols_MultipleCalculatorsDifferentWeights(t *testing.T) {
	gain := &stubCalculator{"gain", 2.0}
	trend := &stubCalculator{"trend", 1.0}
	weights := []ScoreWeight{
		{Calculator: gain, Weight: 0.5},
		{Calculator: trend, Weight: 2.0},
	}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{
		makeSymbol("A"): makeSeries(),
	}
	res, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	exp := 2.0*0.5 + 1.0*2.0
	if res[0].TotalScore != exp {
		t.Errorf("expected %v, got %v", exp, res[0].TotalScore)
	}
}

func TestRankSymbols_ConflictingScores(t *testing.T) {
	pos := &stubCalculator{"pos", 1.0}
	neg := &stubCalculator{"neg", -1.0}
	weights := []ScoreWeight{
		{Calculator: pos, Weight: 1.0},
		{Calculator: neg, Weight: 1.0},
	}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{
		makeSymbol("A"): makeSeries(),
	}
	res, _ := ranker.Rank(series)
	if res[0].TotalScore != 0 {
		t.Errorf("expected 0, got %v", res[0].TotalScore)
	}
}

func TestRankSymbols_DeterministicOrderingWithEqualScores(t *testing.T) {
	calc := &stubCalculator{"gain", 1.0}
	weights := []ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{
		makeSymbol("B"): makeSeries(),
		makeSymbol("A"): makeSeries(),
	}
	res, _ := ranker.Rank(series)
	if res[0].Symbol.String() != "A" {
		t.Errorf("expected A first, got %s", res[0].Symbol.String())
	}
}

func TestRankSymbols_IgnoreZeroWeightCalculators(t *testing.T) {
	calc := &stubCalculator{"gain", 1.0}
	weights := []ScoreWeight{{Calculator: calc, Weight: 0.0}}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{
		makeSymbol("A"): makeSeries(),
	}
	res, _ := ranker.Rank(series)
	if res[0].TotalScore != 0 {
		t.Errorf("expected 0, got %v", res[0].TotalScore)
	}
}

func TestRankSymbols_EmptyInput(t *testing.T) {
	calc := &stubCalculator{"gain", 1.0}
	weights := []ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := NewDefaultRankSymbols(weights)
	series := map[domain.Symbol]domain.CandleSeries{}
	res, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected empty result, got %d", len(res))
	}
}
