package market

// State represents the dominant market regime across the symbol universe.
type State string

const (
	StateSideways    State = "sideways"
	StateCompression State = "compression"
	StateExpansion   State = "expansion"
	StateTrend       State = "trend"
	StateSilent      State = "silent"
	StateIndecisive  State = "indecisive"
)

// Breadth holds the proportionally-weighted breadth for each regime.
// Each symbol distributes its scores continuously across the four
// buckets, so the values reflect the true market character.
// All values are in [0, 1] and sum to ~1.
type Breadth struct {
	Sideways    float64
	Compression float64
	Expansion   float64
	Trend       float64
}

// Summary is the aggregate market state for a given timeframe.
type Summary struct {
	Timeframe   string
	State       State
	Confidence  float64
	Breadth     Breadth // participation across tokens (metrics panel)
	SymbolCount int
	// Bias indicates the dominant direction when State is trend:
	// "up", "down", or "neutral".
	Bias string

	// Structure is the composite-tape score mix that drives State/Confidence
	// when RegimeSource is a composite_* value (PR-084). Zero/empty when the
	// service fell back to participation-only classification.
	Structure Breadth

	// RegimeSource explains how State was chosen:
	// "composite_volume_weighted", "composite_median", or "participation".
	RegimeSource string

	// Trend health (additive — zero values are backward compatible).
	// On the composite-tape path these describe the tape itself (health of
	// the scored series; BreakdownRate is 0 or 1). On the participation
	// fallback they remain averages / fractions across trending tokens.
	EffectiveTrend float64
	BreakdownRate  float64
	Label          string // human-readable quality label

	// Candle-derived market-wide metrics (additive — see PR-073). Populated
	// only when MarketStateService has a CandleProvider configured;
	// otherwise VolatilityExpansion defaults to 1.0 ("normal") and
	// Dispersion to 0.
	VolatilityExpansion float64 // median short/long ATR ratio across symbols
	Dispersion          float64 // mean absolute deviation of asset returns from the market return

	// DataQuality flags whether this Summary reflects a real market read or
	// a degraded/missing input set — see PR-074. Without it, a full
	// evaluation-source outage looks identical to a genuinely quiet market
	// (State=sideways, Confidence=0), which is misleading to show as-is.
	DataQuality DataQuality

	// WindowBars is the number of candles scored for the headline when
	// RegimeSource is composite_*; 0 on participation fallback / unavailable.
	WindowBars int
	// TrendScore is the raw TapeTrend score (0–1) of the composite when
	// RegimeSource is composite_*; zero on the participation fallback.
	// Distinct from EvaluationSnapshot.TrendScore (Trend Predictability).
	TrendScore float64
	// Participation counts tokens by stored eval Trend Predictability
	// (|TrendScore| ≥ 0.5) and sparkline Bias — not TapeTrend.
	Participation Participation
}

// Participation counts tokens whose stored evaluation looks up, down, or
// ranging. Up/down require |EvaluationSnapshot.TrendScore| ≥ 0.5 (Trend
// Predictability) and Bias up/down from the sparkline net move. Distinct
// from Breadth (average per-token score mix) and from response trendScore
// (composite TapeTrend).
type Participation struct {
	Up      int
	Down    int
	Ranging int
	Total   int
}

// DataQuality describes how much of the expected symbol universe actually
// contributed to a Summary.
type DataQuality string

const (
	// DataQualityOK means a normal number of symbols were evaluated.
	DataQualityOK DataQuality = "ok"
	// DataQualityDegraded means evaluations came in, but for meaningfully
	// fewer symbols than expected (partial fetch failures, a struggling
	// upstream) — the reading is real but less reliable than usual.
	DataQualityDegraded DataQuality = "degraded"
	// DataQualityUnavailable means zero usable evaluations — the pipeline
	// has nothing to say about the market, as opposed to having looked and
	// found it quiet.
	DataQualityUnavailable DataQuality = "unavailable"
)
