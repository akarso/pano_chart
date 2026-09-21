package domain

import "time"

// AlgoVersion is the hardcoded scoring engine version tag.
// It must be updated whenever scoring logic changes materially.
const AlgoVersion = "v5.1.0"

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

	// ComputedAt is unix seconds when this snapshot was written to the store.
	// Zero when the snapshot was built on the fly (not from the store).
	ComputedAt int64 `json:"computedAt,omitempty"`

	// Meta
	AlgoVersion string `json:"algoVersion,omitempty"`
}
