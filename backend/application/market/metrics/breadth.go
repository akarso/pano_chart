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
	breakout    float64
}

// scoreBreadth computes market-wide breadth values by running the real
// domain/scoring calculators on each symbol's candle window and averaging.
//
// This replaces the former proxyBreadth heuristic with the exact same
// algorithms used by the overview/rankings pipeline.
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

		// Sideways: already [0, 1].
		ss, err := sidewaysCalc.Score(cs)
		if err == nil {
			sidSum += ss
		}

		// Compression + Breakout: already [0, 1].
		candles := cs.All()
		compResult := scoring.DetectCompression(candles, compCfg)
		compSum += compResult.Score

		breakResult := scoring.DetectBreakout(candles, breakCfg, compResult.Score)
		breakSum += math.Max(breakResult.UpScore, breakResult.DownScore)
	}

	if count == 0 {
		return breadthValues{}
	}

	n := float64(count)
	return breadthValues{
		trend:       trendSum / n,
		sideways:    sidSum / n,
		compression: compSum / n,
		breakout:    breakSum / n,
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
