package scoring

import (
	"../"
	"fmt"
)

// GainLossScoreCalculator scores based on net gain/loss over the series.
type GainLossScoreCalculator struct{}

func (c *GainLossScoreCalculator) Name() string {
	return "Gain/Loss"
}

func (c *GainLossScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	if series.Len() < 2 {
		return 0, fmt.Errorf("at least 2 candles required")
	}
	first, _ := series.First()
	last, _ := series.Last()
	if first.Close() == 0 {
		return 0, fmt.Errorf("first close is zero, cannot normalize")
	}
	return (last.Close() - first.Close()) / first.Close(), nil
}
