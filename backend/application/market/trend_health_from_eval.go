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
//
// ok is false when there is no volatility baseline (flat long sparkline,
// missing ATR, or non-trend state) — callers must skip those rows so they do
// not count as breakdowns.
func TrendHealthFromSnapshot(state string, e domain.EvaluationSnapshot) (health float64, ok bool) {
	if state != "uptrend" && state != "downtrend" {
		return 0, false
	}
	lookForLow := state == "downtrend"
	if len(e.Sparkline) >= tapeMinBars {
		candles := candlesFromCloses(e.Sparkline)
		atr14 := scoring.TrueATR(candles, tapeATRPeriod)
		if atr14 <= 0 {
			return 0, false
		}
		price := e.Sparkline[len(e.Sparkline)-1]
		hi, lo, extremeBars := ohlcWindowExtreme(candles, lookForLow)
		recentReturn := tailReturnATR(e.Sparkline, atr14, crashTailBars)
		return ComputeTrendHealthV2(state, price, hi, lo, atr14, recentReturn, extremeBars), true
	}
	if e.ATR <= 0 {
		return 0, false
	}
	return ComputeTrendHealthV2(state, e.Price, e.RecentHigh, e.RecentLow, e.ATR, e.RecentReturn, 0), true
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
