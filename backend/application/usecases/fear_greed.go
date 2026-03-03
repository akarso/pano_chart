package usecases

import "context"

// FearGreedResult holds the current Fear & Greed Index data.
type FearGreedResult struct {
	Value               int    // 0-100
	ValueClassification string // e.g. "Extreme Fear", "Fear", "Neutral", "Greed", "Extreme Greed"
	Timestamp           int64  // Unix epoch seconds
}

// FearGreedUseCase fetches the current Fear & Greed Index.
type FearGreedUseCase interface {
	Execute(ctx context.Context) (*FearGreedResult, error)
}
