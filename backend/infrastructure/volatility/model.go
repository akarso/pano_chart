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
