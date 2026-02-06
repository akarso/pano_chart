package scoring

import "../"

// SymbolScoreCalculator evaluates a CandleSeries and returns a normalized score.
type SymbolScoreCalculator interface {
	Name() string
	Score(series domain.CandleSeries) (float64, error)
}
