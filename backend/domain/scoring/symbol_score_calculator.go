package scoring

import "pano_chart/backend/domain"

// SymbolScoreCalculator evaluates a CandleSeries and returns a normalized score.
type SymbolScoreCalculator interface {
	Name() string
	Score(series domain.CandleSeries) (float64, error)
}

// DirectedScoreCalculator is a SymbolScoreCalculator that can also report
// directional bias (PR-072 / PR-103). Used by setups dominantRegime and
// store-trend overlay so magnitude and bias share one engine.
type DirectedScoreCalculator interface {
	SymbolScoreCalculator
	ScoreWithDirection(series domain.CandleSeries) (score float64, bias string, err error)
}
