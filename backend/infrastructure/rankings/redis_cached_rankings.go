package rankings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RedisClient abstracts Redis operations needed by the cache decorator.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedRankings is a decorator that caches RankingsUseCase results in Redis.
// The full sorted result is cached per timeframe+sort combination.
// Pagination is NOT cached — it is applied after retrieval by the handler.
//
// On cache hits, badge signals are re-emitted so a multi-candle TTL does not
// skip Track B logging (dedupe suppresses same-candle duplicates).
type RedisCachedRankings struct {
	next          usecases.RankingsUseCase
	redis         RedisClient
	ttl           time.Duration
	keyPrefix     string
	signalEmitter ports.SignalEmitter // optional — PR-090
}

// NewRedisCachedRankings constructs the decorator.
func NewRedisCachedRankings(next usecases.RankingsUseCase, redis RedisClient, ttl time.Duration, keyPrefix string) *RedisCachedRankings {
	return &RedisCachedRankings{
		next:      next,
		redis:     redis,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

// SetSignalEmitter attaches an optional signal logger for cache-hit badge emits.
func (r *RedisCachedRankings) SetSignalEmitter(e ports.SignalEmitter) {
	r.signalEmitter = e
}

// Execute implements RankingsUseCase.
func (r *RedisCachedRankings) Execute(ctx context.Context, req usecases.GetRankingsRequest) ([]usecases.RankedResult, error) {
	key := r.buildKey(req)

	// 1. Attempt Redis GET
	cached, err := r.redis.Get(ctx, key)
	if err == nil && cached != "" {
		var items []cachedRankedResult
		if unmarshalErr := json.Unmarshal([]byte(cached), &items); unmarshalErr == nil {
			out, convErr := fromCached(items)
			if convErr == nil {
				// Cache hit bypasses GetRankings.Execute — still log badges for
				// the current candle (Emitter bar-dedupes repeats).
				usecases.EmitBadgeSignals(ctx, r.signalEmitter, req.Timeframe.String(), out)
				return out, nil
			}
		}
	}

	// 2. Cache miss — call underlying use case (emits badges itself)
	results, err := r.next.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	// 3. Store in Redis (best-effort, never fail the request)
	cacheItems := toCached(results)
	data, marshalErr := json.Marshal(cacheItems)
	if marshalErr == nil {
		_ = r.redis.Set(ctx, key, string(data), r.ttl)
	}

	return results, nil
}

func (r *RedisCachedRankings) buildKey(req usecases.GetRankingsRequest) string {
	algo := string(req.SidewaysAlgo)
	if algo == "" {
		algo = "default"
	}
	return fmt.Sprintf("%s:%s:%s:%s", r.keyPrefix, req.Timeframe.String(), string(req.Sort), algo)
}

// cachedRankedResult is the JSON-serialisable representation of RankedResult.
type cachedRankedResult struct {
	Symbol             string             `json:"symbol"`
	TotalScore         float64            `json:"totalScore"`
	Percentile         float64            `json:"percentile"`
	Scores             map[string]float64 `json:"scores"`
	Volume             float64            `json:"volume"`
	Sparkline          []float64          `json:"sparkline"`
	TrendPercentile    float64            `json:"trendPercentile"`
	SidewaysPercentile float64            `json:"sidewaysPercentile"`
	GainPercentile     float64            `json:"gainPercentile"`
	MaxPercentile      float64            `json:"maxPercentile"`
	DominantComponent  string             `json:"dominantComponent"`
	BadgeComponent     string             `json:"badgeComponent"`
	SignalPrice        float64            `json:"signalPrice,omitempty"`
	SignalATR          float64            `json:"signalATR,omitempty"`
}

func toCached(results []usecases.RankedResult) []cachedRankedResult {
	out := make([]cachedRankedResult, len(results))
	for i, r := range results {
		out[i] = cachedRankedResult{
			Symbol:             r.Symbol.String(),
			TotalScore:         r.TotalScore,
			Percentile:         r.Percentile,
			Scores:             r.Scores,
			Volume:             r.Volume,
			Sparkline:          r.Sparkline,
			TrendPercentile:    r.TrendPercentile,
			SidewaysPercentile: r.SidewaysPercentile,
			GainPercentile:     r.GainPercentile,
			MaxPercentile:      r.MaxPercentile,
			DominantComponent:  r.DominantComponent,
			BadgeComponent:     r.BadgeComponent,
			SignalPrice:        r.SignalPrice,
			SignalATR:          r.SignalATR,
		}
	}
	return out
}

func fromCached(items []cachedRankedResult) ([]usecases.RankedResult, error) {
	out := make([]usecases.RankedResult, len(items))
	for i, c := range items {
		sym, err := domain.NewSymbol(c.Symbol)
		if err != nil {
			return nil, fmt.Errorf("invalid cached symbol %q: %w", c.Symbol, err)
		}
		out[i] = usecases.RankedResult{
			Symbol:             sym,
			TotalScore:         c.TotalScore,
			Percentile:         c.Percentile,
			Scores:             c.Scores,
			Volume:             c.Volume,
			Sparkline:          c.Sparkline,
			TrendPercentile:    c.TrendPercentile,
			SidewaysPercentile: c.SidewaysPercentile,
			GainPercentile:     c.GainPercentile,
			MaxPercentile:      c.MaxPercentile,
			DominantComponent:  c.DominantComponent,
			BadgeComponent:     c.BadgeComponent,
			SignalPrice:        c.SignalPrice,
			SignalATR:          c.SignalATR,
		}
	}
	return out, nil
}
