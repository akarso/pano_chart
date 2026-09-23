package signal

import (
	"math"
	"strings"
	"time"

	"pano_chart/backend/domain"
)

// Outcome is the graded result of a Signal after its horizon elapses (PR-091).
type Outcome struct {
	SignalID      string
	ResolvedAt    time.Time
	ForwardReturn float64 // (close_h - price)/price, signed
	MaxFavorable  float64 // max favorable excursion in ATR units
	MaxAdverse    float64 // max adverse excursion in ATR units
	Success       bool
	Rule          string // which rule decided Success
}

// Outcome rule names. Rules returned by ExcludedFromHitRate must not enter
// scorecard success-rate denominators (PR-092).
const (
	RuleTrendUp             = "trend_up"
	RuleTrendDown           = "trend_down"
	RuleRangeStay           = "range_stay"
	RuleCompressionExpand   = "compression_expand"
	RuleBreakoutUp          = "breakout_up"
	RuleBreakoutDown        = "breakout_down"
	RuleRegimeTrend         = "regime_trend"
	RuleRegimeSideways      = "regime_sideways"
	RuleTransition          = "transition"
	RuleUnsupported         = "unsupported"
	RuleInvalid             = "invalid"
	RuleInsufficientContext = "insufficient_context"
	RulePathUnavailable     = "path_unavailable"
)

// ExcludedHitRateRules lists administrative outcome rules omitted from
// scorecard / baseline denominators (PR-092). Single source for SQL and Go.
func ExcludedHitRateRules() []string {
	return []string{
		RuleUnsupported,
		RuleInvalid,
		RuleInsufficientContext,
		RulePathUnavailable,
	}
}

// ExcludedFromHitRate reports whether an outcome rule is administrative
// (unsupported label, bad TF, missing context, unrecoverable path) and must
// be omitted from hit-rate / baseline denominators in PR-092.
func ExcludedFromHitRate(rule string) bool {
	for _, r := range ExcludedHitRateRules() {
		if rule == r {
			return true
		}
	}
	return false
}

// PathStats holds forward-path metrics over the evaluation horizon.
type PathStats struct {
	ForwardReturn float64
	MaxFavorable  float64
	MaxAdverse    float64
	Closes        []float64
	RangeHigh     float64 // max high over horizon
	RangeLow      float64 // min low over horizon
}

// HorizonBarsOf returns the effective horizon (default when unset).
func HorizonBarsOf(sig Signal) int {
	if sig.HorizonBars <= 0 {
		return DefaultHorizonBars
	}
	return sig.HorizonBars
}

// HorizonEnd is EmittedAt + HorizonBars×tf. ok is false for unknown TF.
func HorizonEnd(sig Signal) (time.Time, bool) {
	tf, err := domain.NewTimeframe(sig.Timeframe)
	if err != nil || tf.Duration() == 0 {
		return time.Time{}, false
	}
	bars := HorizonBarsOf(sig)
	return sig.EmittedAt.UTC().Add(time.Duration(bars) * tf.Duration()), true
}

// DirectionFromLabel returns +1 (long/up), −1 (short/down), or 0 (neutral).
func DirectionFromLabel(label string) int {
	switch normalizeLabel(label) {
	case "trend_up", "breakout_up":
		return 1
	case "trend_down", "breakout_down":
		return -1
	default:
		return 0
	}
}

// NeedsATR reports whether the label’s success rule uses ATR (MFE/MAE or
// ATR/price thresholds). When ATR is unavailable the evaluator must not grade.
func NeedsATR(label string) bool {
	switch normalizeLabel(label) {
	case "trend_up", "trend_down", "sideways", "range",
		"breakout_up", "breakout_down", "regime:trend", "regime:sideways":
		return true
	default:
		return false
	}
}

// NeedsPath reports whether grading requires a forward candle/tape window.
// False for transition:* (regime history only) and unsupported labels
// (gain, regime:expansion, …) so missing market data cannot wedge them.
func NeedsPath(label string) bool {
	l := normalizeLabel(label)
	if strings.HasPrefix(l, "transition:") {
		return false
	}
	switch l {
	case "trend_up", "trend_down", "sideways", "range", "compression",
		"breakout_up", "breakout_down", "regime:trend", "regime:sideways":
		return true
	default:
		return false
	}
}

// ComputePathStats derives forward return and MFE/MAE (ATR units) from OHLC path.
// direction: +1 long, −1 short, 0 → upside=favorable / downside=adverse.
// When atr <= 0, ForwardReturn / range still fill but MFE/MAE stay 0.
func ComputePathStats(entryPrice, atr float64, direction int, closes, highs, lows []float64) PathStats {
	var s PathStats
	n := len(closes)
	if n == 0 || entryPrice <= 0 {
		return s
	}
	if len(highs) != n || len(lows) != n {
		return s
	}
	s.Closes = append([]float64(nil), closes...)
	s.ForwardReturn = (closes[n-1] - entryPrice) / entryPrice
	s.RangeHigh = highs[0]
	s.RangeLow = lows[0]
	for i := 0; i < n; i++ {
		if highs[i] > s.RangeHigh {
			s.RangeHigh = highs[i]
		}
		if lows[i] < s.RangeLow {
			s.RangeLow = lows[i]
		}
		if atr <= 0 {
			continue
		}
		up := (highs[i] - entryPrice) / atr
		down := (entryPrice - lows[i]) / atr
		var fav, adv float64
		switch {
		case direction > 0:
			fav, adv = up, down
		case direction < 0:
			fav, adv = down, up
		default:
			fav, adv = up, down
		}
		if fav > s.MaxFavorable {
			s.MaxFavorable = fav
		}
		if adv > s.MaxAdverse {
			s.MaxAdverse = adv
		}
	}
	return s
}

// Grade applies the PR-091 success rule for sig.Label using path stats and
// (for transition labels) the regime observed at horizon end.
//
// Callers must not invoke Grade for transition labels when regimeAtHorizon is
// empty — leave those unresolved instead. Unsupported / insufficient-context
// outcomes are Success=false and ExcludedFromHitRate.
func Grade(sig Signal, stats PathStats, regimeAtHorizon string) Outcome {
	o := Outcome{
		SignalID:      sig.ID,
		ForwardReturn: stats.ForwardReturn,
		MaxFavorable:  stats.MaxFavorable,
		MaxAdverse:    stats.MaxAdverse,
	}
	label := normalizeLabel(sig.Label)
	switch {
	case label == "trend_up":
		o.Success, o.Rule = ruleTrendUp(stats.ForwardReturn, stats.MaxAdverse)
	case label == "trend_down":
		o.Success, o.Rule = ruleTrendDown(stats.ForwardReturn, stats.MaxAdverse)
	case label == "sideways" || label == "range":
		lo, hi := contextRange(sig.Context)
		if hi <= lo || sig.ATR <= 0 {
			o.Rule = RuleInsufficientContext
			return o
		}
		o.Success, o.Rule = ruleRangeStay(stats.Closes, lo, hi, sig.ATR)
	case label == "compression":
		lo, hi := contextRange(sig.Context)
		if hi <= lo {
			o.Rule = RuleInsufficientContext
			return o
		}
		o.Success, o.Rule = ruleCompressionExpand(stats.RangeHigh, stats.RangeLow, lo, hi)
	case label == "breakout_up":
		o.Success, o.Rule = ruleBreakoutUp(stats.MaxFavorable, stats.MaxAdverse)
	case label == "breakout_down":
		o.Success, o.Rule = ruleBreakoutDown(stats.MaxFavorable, stats.MaxAdverse)
	case label == "regime:trend":
		bias := sig.Context["bias"]
		o.Success, o.Rule = ruleRegimeTrend(stats.ForwardReturn, sig.Price, sig.ATR, bias)
	case label == "regime:sideways":
		o.Success, o.Rule = ruleRegimeSideways(stats.ForwardReturn, sig.Price, sig.ATR)
	case strings.HasPrefix(label, "transition:"):
		want := strings.TrimPrefix(label, "transition:")
		o.Success, o.Rule = ruleTransition(want, regimeAtHorizon)
	default:
		o.Rule = RuleUnsupported
	}
	return o
}

func normalizeLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func contextRange(ctx map[string]float64) (lo, hi float64) {
	if ctx == nil {
		return 0, 0
	}
	return ctx["range_low"], ctx["range_high"]
}

func ruleTrendUp(fwd, mae float64) (bool, string) {
	return fwd > 0 && mae < 2.0, RuleTrendUp
}

func ruleTrendDown(fwd, mae float64) (bool, string) {
	return fwd < 0 && mae < 2.0, RuleTrendDown
}

func ruleRangeStay(closes []float64, rangeLow, rangeHigh, atr float64) (bool, string) {
	tol := 0.5 * atr
	for _, c := range closes {
		if c < rangeLow-tol || c > rangeHigh+tol {
			return false, RuleRangeStay
		}
	}
	return true, RuleRangeStay
}

func ruleCompressionExpand(horizonHigh, horizonLow, emitLow, emitHigh float64) (bool, string) {
	emitRange := emitHigh - emitLow
	realized := horizonHigh - horizonLow
	return realized >= 1.5*emitRange, RuleCompressionExpand
}

func ruleBreakoutUp(mfe, mae float64) (bool, string) {
	return mfe >= 2.0 && mae < 1.0, RuleBreakoutUp
}

func ruleBreakoutDown(mfe, mae float64) (bool, string) {
	return mfe >= 2.0 && mae < 1.0, RuleBreakoutDown
}

func ruleRegimeTrend(fwd, price, atr, bias float64) (bool, string) {
	if price <= 0 || atr <= 0 || bias == 0 {
		return false, RuleRegimeTrend
	}
	thresh := 0.5 * (atr / price)
	if math.Abs(fwd) <= thresh {
		return false, RuleRegimeTrend
	}
	if bias > 0 {
		return fwd > 0, RuleRegimeTrend
	}
	return fwd < 0, RuleRegimeTrend
}

func ruleRegimeSideways(fwd, price, atr float64) (bool, string) {
	if price <= 0 || atr <= 0 {
		return false, RuleRegimeSideways
	}
	thresh := 0.5 * (atr / price)
	return math.Abs(fwd) < thresh, RuleRegimeSideways
}

func ruleTransition(want, got string) (bool, string) {
	want = strings.ToLower(strings.TrimSpace(want))
	got = strings.ToLower(strings.TrimSpace(got))
	return want != "" && got != "" && want == got, RuleTransition
}
