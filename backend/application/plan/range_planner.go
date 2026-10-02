package plan

import (
	"math"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

const (
	// PlanWindowBars is the default swing/channel lookback when Sideways
	// CandleCount is unset (ROADMAP PR-110; matches config.yaml default).
	PlanWindowBars = 110
	planATRPeriod  = 14
	// swingPivot is the half-window for confirmed local extrema (same as
	// trend_strength pivot). Edge bars within pivot of either end cannot
	// confirm, so a wick spike on the last bar does not move High/Low.
	// Interior confirmed pivots — including wick spikes — do move the channel.
	swingPivot = 3

	minRangeQuality = 0.5
	// minWidthATR encodes Mid-target RR≥1.2 for the fixed geometry:
	// RR = widthATR/2.5 − 0.2 ≥ 1.2 ⇒ widthATR ≥ 3.5. There is no separate
	// public "risk reward" reason — width is the binding gate.
	minWidthATR = 3.5
)

// WindowBars returns the plan/channel lookback for timeframe, tied to the
// Sideways V5 CandleCount so quality and geometry describe the same window.
func WindowBars(timeframe string) int {
	n := scoring.NewSidewaysV5ConfigForTimeframe(timeframe).CandleCount
	if n <= 0 {
		return PlanWindowBars
	}
	return n
}

// RangePlan is a long/short range-reversion sketch from swing structure.
type RangePlan struct {
	Symbol, Timeframe                  string
	Low, High, Mid                     float64
	ATR                                float64
	Price                              float64
	LongEntry, LongStop, LongTarget    float64
	ShortEntry, ShortStop, ShortTarget float64
	// LongTargetFull / ShortTargetFull are the aggressive targets
	// (High−0.25×ATR / Low+0.25×ATR); RiskReward uses the conservative Mid targets.
	LongTargetFull  float64
	ShortTargetFull float64
	RiskReward      float64
	RangeQuality    float64
	Position        float64
	Valid           bool
	Reason          string
}

// Size returns units to risk accountRisk (quote) between entry and stop.
// Returns 0 when the stop distance is non-positive or the result is non-finite.
func Size(accountRisk, entry, stop float64) float64 {
	dist := math.Abs(entry - stop)
	if dist <= 0 || math.IsNaN(dist) || math.IsInf(dist, 0) {
		return 0
	}
	if math.IsNaN(accountRisk) || math.IsInf(accountRisk, 0) || accountRisk <= 0 {
		return 0
	}
	out := accountRisk / dist
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return 0
	}
	return out
}

// BuildRangePlan derives a plan from a candle series. RangeQuality is the
// Sideways V5 score (channel-aligned). Lo–MacKinlay MRS is intentionally not
// used (PR-104: tight_range MRS≈0).
func BuildRangePlan(symbol, timeframe string, series domain.CandleSeries) RangePlan {
	plan := RangePlan{Symbol: symbol, Timeframe: timeframe}
	n := series.Len()
	if n < planATRPeriod+1 {
		plan.Reason = "insufficient bars"
		return plan
	}

	window := WindowBars(timeframe)
	if n < window {
		window = n
	}
	candles := series.All()
	start := len(candles) - window
	slice := candles[start:]

	low, high, ok := channelFromSwings(slice)
	if !ok || high <= low {
		plan.Reason = "degenerate channel"
		return plan
	}
	mid := (low + high) / 2
	// ATR on the same window as the channel (not a longer parent series).
	atr := scoring.TrueATR(slice, planATRPeriod)
	price := 0.0
	if last, err := series.At(n - 1); err == nil {
		price = last.Close()
	}

	quality := sidewaysQuality(slice, timeframe)

	return finalizePlan(symbol, timeframe, low, high, mid, atr, price, quality)
}

// BuildRangePlanFromBounds is a pure geometry+quality constructor for tests.
func BuildRangePlanFromBounds(symbol, timeframe string, low, high, price, atr, quality float64) RangePlan {
	mid := (low + high) / 2
	return finalizePlan(symbol, timeframe, low, high, mid, atr, price, quality)
}

func finalizePlan(symbol, timeframe string, low, high, mid, atr, price, quality float64) RangePlan {
	plan := RangePlan{
		Symbol:       symbol,
		Timeframe:    timeframe,
		Low:          low,
		High:         high,
		Mid:          mid,
		ATR:          atr,
		Price:        price,
		RangeQuality: quality,
	}
	if atr <= 0 || math.IsNaN(atr) || math.IsInf(atr, 0) {
		plan.Reason = "atr unavailable"
		return plan
	}
	if high <= low {
		plan.Reason = "degenerate channel"
		return plan
	}

	plan.Position = clamp01((price - low) / (high - low))

	plan.LongEntry = low + 0.25*atr
	plan.LongStop = low - 1.0*atr
	plan.LongTarget = mid
	plan.LongTargetFull = high - 0.25*atr

	plan.ShortEntry = high - 0.25*atr
	plan.ShortStop = high + 1.0*atr
	plan.ShortTarget = mid
	plan.ShortTargetFull = low + 0.25*atr

	entryStop := plan.LongEntry - plan.LongStop // always 1.25×ATR when atr>0
	plan.RiskReward = (plan.LongTarget - plan.LongEntry) / entryStop
	if math.IsNaN(plan.RiskReward) || math.IsInf(plan.RiskReward, 0) {
		plan.RiskReward = 0
	}

	if plan.LongStop <= 0 {
		plan.Reason = "non-positive stop"
		clearLevels(&plan)
		return plan
	}

	widthATR := (high - low) / atr
	switch {
	case quality < minRangeQuality:
		plan.Reason = "range quality"
	case widthATR < minWidthATR:
		plan.Reason = "channel width"
	default:
		plan.Valid = true
	}
	if !plan.Valid {
		clearLevels(&plan)
	}
	return plan
}

// clearLevels zeros trade levels for invalid plans. RiskReward and Position
// are kept for diagnostics (clients must not size off cleared levels).
func clearLevels(p *RangePlan) {
	p.LongEntry, p.LongStop, p.LongTarget, p.LongTargetFull = 0, 0, 0, 0
	p.ShortEntry, p.ShortStop, p.ShortTarget, p.ShortTargetFull = 0, 0, 0, 0
}

// channelFromSwings sets Low/High from confirmed swing lows/highs (pivot=3).
// Uses plateau-tolerant pivots so equal high/low touches within the window
// still confirm (aligned with Sideways V5 extrema). Falls back to false when
// fewer than one swing of each side exists.
func channelFromSwings(candles []domain.Candle) (low, high float64, ok bool) {
	n := len(candles)
	if n < 2*swingPivot+1 {
		return 0, 0, false
	}
	highs := make([]float64, n)
	lows := make([]float64, n)
	for i, c := range candles {
		highs[i] = c.High()
		lows[i] = c.Low()
	}
	var swingHighs, swingLows []float64
	for i := swingPivot; i < n-swingPivot; i++ {
		if scoring.IsPivotHighAllowEqual(highs, i, swingPivot) {
			swingHighs = append(swingHighs, highs[i])
		}
		if scoring.IsPivotLowAllowEqual(lows, i, swingPivot) {
			swingLows = append(swingLows, lows[i])
		}
	}
	if len(swingHighs) == 0 || len(swingLows) == 0 {
		return 0, 0, false
	}
	high = swingHighs[0]
	for _, v := range swingHighs[1:] {
		if v > high {
			high = v
		}
	}
	low = swingLows[0]
	for _, v := range swingLows[1:] {
		if v < low {
			low = v
		}
	}
	return low, high, true
}

func clamp01(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// sidewaysQuality scores the same candle window used for the channel/ATR.
func sidewaysQuality(window []domain.Candle, timeframe string) float64 {
	if len(window) == 0 {
		return 0
	}
	sym := window[0].Symbol()
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		tf = window[0].Timeframe()
	}
	series, err := domain.NewCandleSeries(sym, tf, window)
	if err != nil {
		return 0
	}
	calc := &scoring.SidewaysV5ScoreCalculator{
		Config: scoring.NewSidewaysV5ConfigForTimeframe(timeframe),
	}
	score, err := calc.Score(series)
	if err != nil || math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}
