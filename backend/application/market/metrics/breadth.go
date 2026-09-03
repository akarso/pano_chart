package metrics

import (
	"math"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

// breadthValues holds market-wide average scores for each regime.
// All values are normalised to [0, 1].
type breadthValues struct {
	trend       float64
	sideways    float64
	compression float64
	expansion   float64
}

// scoreBreadth computes market-wide breadth values by running the real
// domain/scoring calculators on each symbol's candle window and averaging.
//
// Direction agreement: if tokens disagree on trend direction (some up,
// some down), the trend breadth is dampened.  Mixed-direction trends
// are not a market trend — they indicate indecision or rotation.
func scoreBreadth(seriesList []domain.CandleSeries, timeframe string) breadthValues {
	if len(seriesList) == 0 {
		return breadthValues{}
	}

	trendCalc := &scoring.TrendPredictabilityScoreCalculator{}
	sidewaysCalc := &scoring.SidewaysV5ScoreCalculator{
		Config: scoring.NewSidewaysV5ConfigForTimeframe(timeframe),
	}
	compCfg := scoring.DefaultCompressionConfig()
	breakCfg := scoring.DefaultBreakoutConfig()

	var trendSum, sidSum, compSum, breakSum float64
	var upDir, downDir int
	count := 0

	for _, cs := range seriesList {
		n := cs.Len()
		if n < 2 {
			continue
		}
		count++

		// Trend: already normalised to [0, 1] by the calculator
		// (raw score × (N-1) → perfect linear ≈ 1.0).
		ts, err := trendCalc.Score(cs)
		if err == nil {
			trendSum += math.Abs(ts)
		}

		// Track direction from period return.
		candles := cs.All()
		first := candles[0].Close()
		last := candles[len(candles)-1].Close()
		if last > first {
			upDir++
		} else if last < first {
			downDir++
		}

		// Sideways: already [0, 1].
		ss, err := sidewaysCalc.Score(cs)
		if err == nil {
			sidSum += ss
		}

		// Compression + Breakout: already [0, 1].
		compResult := scoring.DetectCompression(candles, compCfg)
		compSum += compResult.Score

		breakResult := scoring.DetectBreakout(candles, breakCfg, compResult.Score)
		breakSum += math.Max(breakResult.UpScore, breakResult.DownScore)
	}

	if count == 0 {
		return breadthValues{}
	}

	n := float64(count)

	// Direction agreement: 1.0 = all tokens agree, 0.0 = perfect 50/50 split.
	// Mixed-direction trends dampen trend breadth.
	agreement := scoring.DirectionAgreement(upDir, downDir)
	rawTrend := trendSum / n
	dampened := rawTrend * agreement
	// Redistribute only half the lost trend to sideways — the other half
	// stays as undirected trend energy so trend is not fully erased during
	// normal market rotation.
	lost := rawTrend - dampened
	dampened += lost * 0.5
	sidSum += lost * 0.5 * n

	return breadthValues{
		trend:       dampened,
		sideways:    sidSum / n,
		compression: compSum / n,
		expansion:   breakSum / n,
	}
}

// scoreBreadthFromCandles constructs CandleSeries from raw candle windows
// and delegates to scoreBreadth.  Used by the backfiller which works with
// [][]domain.Candle rather than []domain.CandleSeries.
func scoreBreadthFromCandles(windows [][]domain.Candle, timeframe string) breadthValues {
	seriesList := make([]domain.CandleSeries, 0, len(windows))
	for _, w := range windows {
		if len(w) < 2 {
			continue
		}
		cs, err := domain.NewCandleSeries(w[0].Symbol(), w[0].Timeframe(), w)
		if err != nil {
			continue
		}
		seriesList = append(seriesList, cs)
	}
	return scoreBreadth(seriesList, timeframe)
}
