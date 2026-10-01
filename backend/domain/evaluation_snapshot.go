package domain

import (
	"sync"
	"time"
)

// AlgoVersion is the hardcoded scoring engine version tag.
// It must be updated whenever scoring logic changes materially.
const AlgoVersion = "v5.1.0"

const defaultTrendAlgo = "predictability"

var (
	activeTrendAlgoMu sync.RWMutex
	activeTrendAlgo   = defaultTrendAlgo
)

// ConfigureTrendAlgo sets the process-wide trend engine identity stamped on
// and required of evaluation snapshots (PR-103). Empty / unknown → predictability.
// Call once from main after parsing scoring.trend_algo / TREND_ALGO.
func ConfigureTrendAlgo(algo string) {
	if algo == "" {
		algo = defaultTrendAlgo
	}
	activeTrendAlgoMu.Lock()
	activeTrendAlgo = algo
	activeTrendAlgoMu.Unlock()
}

// ActiveTrendAlgo returns the process trend engine identity.
func ActiveTrendAlgo() string {
	activeTrendAlgoMu.RLock()
	defer activeTrendAlgoMu.RUnlock()
	return activeTrendAlgo
}

// EvaluationIdentityOK reports whether a stored snapshot was produced by the
// current scoring engine and trend algorithm. Empty TrendAlgo is treated as
// predictability (pre-PR-103 rows) so flipping to strength invalidates them.
func EvaluationIdentityOK(algoVersion, trendAlgo string) bool {
	if algoVersion != AlgoVersion {
		return false
	}
	got := trendAlgo
	if got == "" {
		got = defaultTrendAlgo
	}
	return got == ActiveTrendAlgo()
}

// EvaluationSnapshot captures all regime scores and market state
// at the time of evaluation for a single symbol/timeframe cycle.
// Treat as a value object: construct via helpers (e.g. SnapshotsFromRankings),
// then copy — do not mutate in place after it leaves the constructing function.
// Store writers may stamp Sparkline / ComputedAt on a copy before Put.
type EvaluationSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`

	// Core regime scores
	SidewaysScore     float64 `json:"sidewaysScore"`
	CompressionScore  float64 `json:"compressionScore"`
	BreakoutUpScore   float64 `json:"breakoutUpScore"`
	BreakoutDownScore float64 `json:"breakoutDownScore"`
	TrendScore        float64 `json:"trendScore"`

	// Directional bias ("up", "down", "neutral")
	Bias string `json:"bias"`

	// Structural subtype ("parallel", "compression", etc.)
	ChannelType string `json:"channelType,omitempty"`

	// Market state
	Price  float64 `json:"price"`
	ATR    float64 `json:"atr"`
	Volume float64 `json:"volume"`

	// Recent price extremes (for trend health computation).
	RecentHigh   float64 `json:"recentHigh"`
	RecentLow    float64 `json:"recentLow"`
	RecentReturn float64 `json:"recentReturn"`

	// Sparkline is the close-price series used for enrichment / UI.
	// Populated by the evaluation store writer (PR-089a); empty when absent.
	Sparkline []float64 `json:"sparkline,omitempty"`

	// TotalScore is the ranked composite score copied from rankings when
	// the snapshot is written (PR-102). Nil when the store row predates
	// this field or the snapshot was built without a rankings score —
	// absent must not unmarshal as a false zero.
	TotalScore *float64 `json:"totalScore,omitempty"`

	// RelativeStrength is log excess return vs the tape when available
	// (PR-096 / PR-102). Nil when RS was not scored for this row.
	RelativeStrength *float64 `json:"rs,omitempty"`

	// ComputedAt is unix seconds when this snapshot was written to the store.
	// Zero when the snapshot was built on the fly (not from the store).
	ComputedAt int64 `json:"computedAt,omitempty"`

	// Meta
	AlgoVersion string `json:"algoVersion,omitempty"`

	// TrendAlgo records which trend engine produced TrendScore / TotalScore
	// (predictability | strength). Empty on pre-PR-103 store rows.
	TrendAlgo string `json:"trendAlgo,omitempty"`
}
