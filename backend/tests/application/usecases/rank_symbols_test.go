package usecases

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// stubCalculator returns a fixed score per symbol for testing.
type stubCalculator struct {
	name   string
	scores map[string]float64 // symbol → raw score
}

func (s *stubCalculator) Name() string { return s.name }

func (s *stubCalculator) Score(series domain.CandleSeries) (float64, error) {
	first, _ := series.First()
	sym := first.Symbol().String()
	return s.scores[sym], nil
}

// helper to build a minimal CandleSeries for a symbol.
func stubSeries(sym domain.Symbol) domain.CandleSeries {
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c1, _ := domain.NewCandle(sym, tf, base, 90, 110, 80, 100, 1000)
	c2, _ := domain.NewCandle(sym, tf, base.Add(time.Hour), 90, 110, 80, 100, 1000)
	cs, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{c1, c2})
	return cs
}

func sym(name string) domain.Symbol {
	s, _ := domain.NewSymbol(name)
	return s
}

func TestRankSymbols_ReturnsRawWeightedScores(t *testing.T) {
	btc := sym("BTCUSDT")
	eth := sym("ETHUSDT")

	sideways := &stubCalculator{
		name:   "Sideways",
		scores: map[string]float64{"BTCUSDT": 0.8, "ETHUSDT": 0.4},
	}
	gain := &stubCalculator{
		name:   "Gain",
		scores: map[string]float64{"BTCUSDT": 0.01, "ETHUSDT": 0.02},
	}

	weights := []usecases.ScoreWeight{
		{Calculator: sideways, Weight: 1.0},
		{Calculator: gain, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)

	series := map[domain.Symbol]domain.CandleSeries{
		btc: stubSeries(btc),
		eth: stubSeries(eth),
	}

	result, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scoreMap := make(map[string]usecases.RankedSymbol)
	for _, r := range result {
		scoreMap[r.Symbol.String()] = r
	}

	// BTC: 0.8*1.0 + 0.01*1.0 = 0.81
	btcExpected := 0.81
	if math.Abs(scoreMap["BTCUSDT"].TotalScore-btcExpected) > 1e-9 {
		t.Errorf("BTC expected total %.4f, got %.4f", btcExpected, scoreMap["BTCUSDT"].TotalScore)
	}

	// ETH: 0.4*1.0 + 0.02*1.0 = 0.42
	ethExpected := 0.42
	if math.Abs(scoreMap["ETHUSDT"].TotalScore-ethExpected) > 1e-9 {
		t.Errorf("ETH expected total %.4f, got %.4f", ethExpected, scoreMap["ETHUSDT"].TotalScore)
	}
}

func TestRankSymbols_RawScoresPreserved(t *testing.T) {
	btc := sym("BTCUSDT")
	eth := sym("ETHUSDT")
	sol := sym("SOLUSDT")

	calc := &stubCalculator{
		name:   "Test",
		scores: map[string]float64{"BTCUSDT": 100.0, "ETHUSDT": 50.0, "SOLUSDT": 0.0},
	}

	weights := []usecases.ScoreWeight{
		{Calculator: calc, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)

	series := map[domain.Symbol]domain.CandleSeries{
		btc: stubSeries(btc),
		eth: stubSeries(eth),
		sol: stubSeries(sol),
	}

	result, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scoreMap := make(map[string]float64)
	for _, r := range result {
		scoreMap[r.Symbol.String()] = r.Scores["Test"]
	}

	if scoreMap["BTCUSDT"] != 100.0 {
		t.Errorf("BTC raw score expected 100.0, got %.4f", scoreMap["BTCUSDT"])
	}
	if scoreMap["ETHUSDT"] != 50.0 {
		t.Errorf("ETH raw score expected 50.0, got %.4f", scoreMap["ETHUSDT"])
	}
	if scoreMap["SOLUSDT"] != 0.0 {
		t.Errorf("SOL raw score expected 0.0, got %.4f", scoreMap["SOLUSDT"])
	}
}

func TestRankSymbols_PreservesRelativeOrderByTotalScore(t *testing.T) {
	btc := sym("BTCUSDT")
	eth := sym("ETHUSDT")
	sol := sym("SOLUSDT")

	calc := &stubCalculator{
		name:   "Trend",
		scores: map[string]float64{"BTCUSDT": 10.0, "ETHUSDT": 5.0, "SOLUSDT": 1.0},
	}

	weights := []usecases.ScoreWeight{
		{Calculator: calc, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)

	series := map[domain.Symbol]domain.CandleSeries{
		btc: stubSeries(btc),
		eth: stubSeries(eth),
		sol: stubSeries(sol),
	}

	result, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be sorted descending by total score
	if result[0].Symbol.String() != "BTCUSDT" {
		t.Errorf("expected BTC first, got %s", result[0].Symbol.String())
	}
	if result[1].Symbol.String() != "ETHUSDT" {
		t.Errorf("expected ETH second, got %s", result[1].Symbol.String())
	}
	if result[2].Symbol.String() != "SOLUSDT" {
		t.Errorf("expected SOL third, got %s", result[2].Symbol.String())
	}
}

func TestRankSymbols_EmptySeriesReturnsNil(t *testing.T) {
	calc := &stubCalculator{name: "X", scores: map[string]float64{}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	ranker := usecases.NewDefaultRankSymbols(weights)

	result, err := ranker.Rank(map[domain.Symbol]domain.CandleSeries{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty series, got %v", result)
	}
}

func TestRankSymbols_WeightsAppliedToRawScores(t *testing.T) {
	btc := sym("BTCUSDT")
	eth := sym("ETHUSDT")

	calcA := &stubCalculator{
		name:   "A",
		scores: map[string]float64{"BTCUSDT": 10.0, "ETHUSDT": 0.0},
	}
	calcB := &stubCalculator{
		name:   "B",
		scores: map[string]float64{"BTCUSDT": 0.0, "ETHUSDT": 10.0},
	}

	// Weight A twice as much
	weights := []usecases.ScoreWeight{
		{Calculator: calcA, Weight: 2.0},
		{Calculator: calcB, Weight: 1.0},
	}
	ranker := usecases.NewDefaultRankSymbols(weights)

	series := map[domain.Symbol]domain.CandleSeries{
		btc: stubSeries(btc),
		eth: stubSeries(eth),
	}

	result, err := ranker.Rank(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scoreMap := make(map[string]usecases.RankedSymbol)
	for _, r := range result {
		scoreMap[r.Symbol.String()] = r
	}

	// BTC: A=10*2 + B=0*1 = 20
	// ETH: A=0*2 + B=10*1 = 10
	if scoreMap["BTCUSDT"].TotalScore != 20.0 {
		t.Errorf("BTC expected 20.0, got %.4f", scoreMap["BTCUSDT"].TotalScore)
	}
	if scoreMap["ETHUSDT"].TotalScore != 10.0 {
		t.Errorf("ETH expected 10.0, got %.4f", scoreMap["ETHUSDT"].TotalScore)
	}

	// BTC should rank higher
	if result[0].Symbol.String() != "BTCUSDT" {
		t.Errorf("BTC should rank first, got %s", result[0].Symbol.String())
	}
}
