package scoring

import (
	"sync"

	"pano_chart/backend/domain"
)

// TimeframeAwareCalculator picks a per-timeframe configured calculator via
// factory, caching one instance per timeframe string. Use this when the
// underlying calculator's config varies by timeframe (e.g. SidewaysV3 RMin/RMax
// or SidewaysV5 IdealATRRange).
//
// Factory may run more than once under a concurrent first miss for the same
// tf; LoadOrStore keeps a single winner. Factories must be cheap and
// idempotent (no unique side effects per call).
type TimeframeAwareCalculator struct {
	name    string
	factory func(tf string) SymbolScoreCalculator
	cache   sync.Map // tf → SymbolScoreCalculator
}

// NewTimeframeAwareCalculator constructs a wrapper with the given public name
// and per-timeframe factory.
func NewTimeframeAwareCalculator(name string, factory func(tf string) SymbolScoreCalculator) *TimeframeAwareCalculator {
	return &TimeframeAwareCalculator{name: name, factory: factory}
}

// Name returns the public calculator name (stable across timeframes).
func (c *TimeframeAwareCalculator) Name() string {
	return c.name
}

// Score delegates to the calculator for series.Timeframe().
func (c *TimeframeAwareCalculator) Score(series domain.CandleSeries) (float64, error) {
	return c.calculatorFor(series.Timeframe().String()).Score(series)
}

func (c *TimeframeAwareCalculator) calculatorFor(tf string) SymbolScoreCalculator {
	if v, ok := c.cache.Load(tf); ok {
		return v.(SymbolScoreCalculator)
	}
	created := c.factory(tf)
	actual, _ := c.cache.LoadOrStore(tf, created)
	return actual.(SymbolScoreCalculator)
}
