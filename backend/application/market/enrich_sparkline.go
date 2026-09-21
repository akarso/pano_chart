package market

import (
	"math"

	"pano_chart/backend/domain"
)

// EnrichFromSparkline derives Bias, Price, ATR, RecentHigh, RecentLow, and
// RecentReturn from a close-price sparkline. Moved here from infrastructure
// so the evaluation store writer (PR-089a) can enrich without crossing
// application → infrastructure.
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
	var atrSum float64
	for i := 0; i < n; i++ {
		if sparkline[i] > hi {
			hi = sparkline[i]
		}
		if sparkline[i] < lo {
			lo = sparkline[i]
		}
		if i > 0 {
			atrSum += math.Abs(sparkline[i] - sparkline[i-1])
		}
	}
	snap.RecentHigh = hi
	snap.RecentLow = lo
	snap.ATR = atrSum / float64(n-1)

	if snap.ATR > 0 {
		snap.RecentReturn = (sparkline[n-1] - sparkline[0]) / snap.ATR
	}
}
