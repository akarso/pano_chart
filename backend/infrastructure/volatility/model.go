package volatility

// Candle represents a single 1-minute OHLC candle.
type Candle struct {
	OpenTime int64
	Open     float64
	High     float64
	Low      float64
	Close    float64
}

// Bucket accumulates intraday statistics for one minute-of-day slot.
type Bucket struct {
	MinuteOfDay int
	Count       int
	SumMove     float64
	SpikeCount  int
}

// Result holds the final aggregation output.
type Result struct {
	Buckets []BucketResult `json:"buckets"`
}

// BucketResult is the per-minute output included in JSON.
type BucketResult struct {
	MinuteOfDay int     `json:"minute"`
	AvgMove     float64 `json:"avg_move"`
	SpikeProb   float64 `json:"spike_prob"`
	Normalized  float64 `json:"normalized"`
}

// Timeframe identifies a candle aggregation interval.
type Timeframe string

const (
	TF1m  Timeframe = "1m"
	TF5m  Timeframe = "5m"
	TF15m Timeframe = "15m"
	TF1h  Timeframe = "1h"
	TF4h  Timeframe = "4h"
	TF1d  Timeframe = "1d"
)

// TimeframeResult holds volatility buckets for a single timeframe.
type TimeframeResult struct {
	Timeframe Timeframe      `json:"timeframe"`
	Buckets   []BucketResult `json:"buckets"`
}

// WeeklyResult holds 7-day seasonality buckets.
type WeeklyResult struct {
	Buckets []WeeklyBucket `json:"buckets"`
}

// WeeklyBucket is a single minute-of-week volatility entry.
type WeeklyBucket struct {
	MinuteOfWeek int     `json:"minute"`
	AvgMove      float64 `json:"avg_move"`
	SpikeProb    float64 `json:"spike_prob"`
	Normalized   float64 `json:"normalized"`
}

// FullResult is the combined output for all timeframes plus weekly.
type FullResult struct {
	Intraday []TimeframeResult `json:"intraday"`
	Weekly   WeeklyResult      `json:"weekly"`
}
