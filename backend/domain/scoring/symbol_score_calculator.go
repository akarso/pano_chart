package scoring

import "pano_chart/backend/domain"

// SymbolScoreCalculator evaluates a CandleSeries and returns a normalized score.
type SymbolScoreCalculator interface {
	Name() string
	Score(series domain.CandleSeries) (float64, error)
}

// WindowHintProvider is an optional SymbolScoreCalculator extension (PR-105).
// GetRankings fetches max(precision, WindowHint()) so percentile compression
// can request 500 bars without changing the default 110-bar sparkline path.
type WindowHintProvider interface {
	WindowHint() int
}

// MaxWindowHint returns the largest WindowHint among calculators that implement
// WindowHintProvider, floored at floor (typically rankings precision).
func MaxWindowHint(floor int, calcs ...SymbolScoreCalculator) int {
	max := floor
	if max < 0 {
		max = 0
	}
	for _, c := range calcs {
		if h, ok := c.(WindowHintProvider); ok {
			if n := h.WindowHint(); n > max {
				max = n
			}
		}
	}
	return max
}

// DirectedScoreCalculator is a SymbolScoreCalculator that can also report
// directional bias (PR-072 / PR-103). Used by setups dominantRegime and
// store-trend overlay so magnitude and bias share one engine.
type DirectedScoreCalculator interface {
	SymbolScoreCalculator
	ScoreWithDirection(series domain.CandleSeries) (score float64, bias string, err error)
}
