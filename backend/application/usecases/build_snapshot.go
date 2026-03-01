package usecases

import (
	"math"
	"time"

	"pano_chart/backend/domain"
)

// BuildSnapshot constructs an EvaluationSnapshot from scored results
// and the candle series that was evaluated.
//
// scores maps calculator names to their computed scores.
// series provides market state (price, volume, ATR from last candle).
// algoVersion identifies the scoring engine version.
func BuildSnapshot(
	symbol domain.Symbol,
	timeframe domain.Timeframe,
	scores map[string]float64,
	series domain.CandleSeries,
	volume float64,
	algoVersion string,
) domain.EvaluationSnapshot {
	now := time.Now().UTC()

	price := lastClose(series)
	atr := simpleATR(series, 14)

	return domain.EvaluationSnapshot{
		Timestamp:         now,
		Symbol:            symbol.String(),
		Timeframe:         timeframe.String(),
		SidewaysScore:     scores["Sideways Consistency"],
		CompressionScore:  0, // not yet computed; placeholder for future
		BreakoutUpScore:   0, // not yet computed
		BreakoutDownScore: 0, // not yet computed
		TrendScore:        scores["Trend Predictability"],
		Bias:              "neutral", // placeholder — no directional bias calc yet
		ChannelType:       "",        // placeholder — structural subtype not computed yet
		Price:             price,
		ATR:               atr,
		Volume:            volume,
		AlgoVersion:       algoVersion,
	}
}

// lastClose returns the close price of the last candle, or 0 if empty.
func lastClose(series domain.CandleSeries) float64 {
	c, err := series.Last()
	if err != nil {
		return 0
	}
	return c.Close()
}

// simpleATR computes a simple ATR (average true range) over the last n candles.
// Returns 0 if insufficient data.
func simpleATR(series domain.CandleSeries, n int) float64 {
	length := series.Len()
	if length < 2 || n <= 0 {
		return 0
	}
	if n > length-1 {
		n = length - 1
	}

	sum := 0.0
	start := length - n
	for i := start; i < length; i++ {
		curr, err := series.At(i)
		if err != nil {
			continue
		}
		prev, err := series.At(i - 1)
		if err != nil {
			continue
		}
		tr := math.Max(curr.High()-curr.Low(),
			math.Max(math.Abs(curr.High()-prev.Close()),
				math.Abs(curr.Low()-prev.Close())))
		sum += tr
	}
	return sum / float64(n)
}
