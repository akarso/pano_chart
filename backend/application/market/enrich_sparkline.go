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
// mean |Δclose|. RecentReturn stays full-window / ATR so silent override and
// bias override keep their pre-PR-106 shape (Wilder vs mean-|Δ| is a small
// residual unit drift when len ≥ 15). The 8-bar adverse tail used by
// trend health is computed only inside TrendHealthFromSnapshot / ScoreMarketTape.
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
	for i := 0; i < n; i++ {
		if sparkline[i] > hi {
			hi = sparkline[i]
		}
		if sparkline[i] < lo {
			lo = sparkline[i]
		}
	}
	snap.RecentHigh = hi
	snap.RecentLow = lo

	atr := 0.0
	if n >= tapeMinBars {
		atr = scoring.TrueATR(candlesFromCloses(sparkline), tapeATRPeriod)
	}
	if atr <= 0 {
		var atrSum float64
		for i := 1; i < n; i++ {
			atrSum += math.Abs(sparkline[i] - sparkline[i-1])
		}
		atr = atrSum / float64(n-1)
	}
	snap.ATR = atr

	if snap.ATR > 0 {
		snap.RecentReturn = (sparkline[n-1] - sparkline[0]) / snap.ATR
	}
}
