package market

import (
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

// TrendHealthFromSnapshot scores trend health for participation fallback (PR-106).
// With a long enough sparkline it uses Wilder ATR14, window extremes, staleness,
// and the same 8-bar adverse tail as the tape path. Otherwise it uses enriched
// snapshot fields with staleness disabled.
func TrendHealthFromSnapshot(state string, e domain.EvaluationSnapshot) float64 {
	if state != "uptrend" && state != "downtrend" {
		return 0
	}
	lookForLow := state == "downtrend"
	if len(e.Sparkline) >= tapeMinBars {
		candles := candlesFromCloses(e.Sparkline)
		atr14 := scoring.TrueATR(candles, tapeATRPeriod)
		if atr14 <= 0 {
			return 0
		}
		price := e.Sparkline[len(e.Sparkline)-1]
		hi, lo, extremeBars := ohlcWindowExtreme(candles, lookForLow)
		recentReturn := tailReturnATR(e.Sparkline, atr14, crashTailBars)
		return ComputeTrendHealthV2(state, price, hi, lo, atr14, recentReturn, extremeBars)
	}
	if e.ATR <= 0 {
		return 0
	}
	return ComputeTrendHealthV2(state, e.Price, e.RecentHigh, e.RecentLow, e.ATR, e.RecentReturn, 0)
}

// hasTrendHealthBaseline reports whether a snapshot can produce a usable
// volatility baseline for V2 health. Flat long sparklines yield TrueATR 0 and
// must be skipped (not counted as breakdowns).
func hasTrendHealthBaseline(e domain.EvaluationSnapshot) bool {
	if len(e.Sparkline) >= tapeMinBars {
		return scoring.TrueATR(candlesFromCloses(e.Sparkline), tapeATRPeriod) > 0
	}
	return e.ATR > 0
}

func candlesFromCloses(closes []float64) []domain.Candle {
	out := make([]domain.Candle, len(closes))
	for i, c := range closes {
		out[i] = domain.NewCandleUnsafe(
			domain.Symbol("SPARK"), domain.Timeframe("1h"), time.Unix(int64(i), 0),
			c, c, c, c, 0,
		)
	}
	return out
}
