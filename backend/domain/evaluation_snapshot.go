package domain

import "time"

// AlgoVersion is the hardcoded scoring engine version tag.
// It must be updated whenever scoring logic changes materially.
const AlgoVersion = "v5.1.0"

// EvaluationSnapshot captures all regime scores and market state
// at the time of evaluation for a single symbol/timeframe cycle.
// It is a value object — immutable after construction.
type EvaluationSnapshot struct {
	Timestamp time.Time
	Symbol    string
	Timeframe string

	// Core regime scores
	SidewaysScore     float64
	CompressionScore  float64
	BreakoutUpScore   float64
	BreakoutDownScore float64
	TrendScore        float64

	// Directional bias ("up", "down", "neutral")
	Bias string

	// Structural subtype ("parallel", "compression", etc.)
	ChannelType string

	// Market state
	Price  float64
	ATR    float64
	Volume float64

	// Recent price extremes (for trend health computation).
	RecentHigh   float64 // highest high within lookback window
	RecentLow    float64 // lowest low within lookback window
	RecentReturn float64 // recent return in ATR units

	// Meta
	AlgoVersion string
}
