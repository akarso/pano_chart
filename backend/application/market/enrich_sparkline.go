package market

import (
	"math"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

// EnrichFromSparkline derives Bias, Price, ATR, RecentHigh, RecentLow, and
// RecentReturn from a close-price sparkline. Moved here from infrastructure
// so the evaluation store writer (PR-089a) can enrich without crossing
// application → infrastructure.
//
// ATR is Wilder TrueATR(14) when len ≥ tapeMinBars (close-only OHLC); otherwise
// mean |Δclose|. RecentReturn is always full-window / mean|Δclose| so silent
// override and bias override keep their pre-PR-106 scale (Wilder is not used
// as the return denominator). The 8-bar adverse tail used by trend health is
// computed only inside TrendHealthFromSnapshot / ScoreMarketTape.
func EnrichFromSparkline(snap *domain.EvaluationSnapshot, sparkline []float64) {
	n := len(sparkline)
	if n < 2 {
		return
	}

	snap.Price = sparkline[n-1]

	if sparkline[n-1] > sparkline[0] {
		snap.Bias = "up"
	} else if sparkline[n-1] < sparkline[0] {
		snap.Bias = "down"
	} else {
		snap.Bias = "neutral"
	}

	hi, lo := sparkline[0], sparkline[0]
	var absSum float64
	for i := 0; i < n; i++ {
		if sparkline[i] > hi {
			hi = sparkline[i]
		}
		if sparkline[i] < lo {
			lo = sparkline[i]
		}
		if i > 0 {
			absSum += math.Abs(sparkline[i] - sparkline[i-1])
		}
	}
	snap.RecentHigh = hi
	snap.RecentLow = lo

	meanAbs := absSum / float64(n-1)
	atr := meanAbs
	if n >= tapeMinBars {
		if w := scoring.TrueATR(candlesFromCloses(sparkline), tapeATRPeriod); w > 0 {
			atr = w
		}
	}
	snap.ATR = atr

	if meanAbs > 0 {
		snap.RecentReturn = (sparkline[n-1] - sparkline[0]) / meanAbs
	}
}
