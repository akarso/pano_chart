package infra

import (
	"encoding/json"
	"fmt"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// MinimalRedisClient is the minimal interface required by the decorator.
type MinimalRedisClient interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
}

// RedisCandleRepository is a caching decorator that implements ports.CandleRepositoryPort.
type RedisCandleRepository struct {
	client  MinimalRedisClient
	wrapped ports.CandleRepositoryPort
	ttl     time.Duration
}

// NewRedisCandleRepository constructs the decorator. TTL must be > 0.
func NewRedisCandleRepository(client MinimalRedisClient, wrapped ports.CandleRepositoryPort, ttl time.Duration) *RedisCandleRepository {
	if ttl <= 0 {
		panic("ttl must be > 0")
	}
	return &RedisCandleRepository{client: client, wrapped: wrapped, ttl: ttl}
}

// cacheKey builds a deterministic cache key for the request.
func cacheKey(symbol domain.Symbol, tf domain.Timeframe, from, to time.Time) string {
	return fmt.Sprintf("%s|%s|%s|%s", symbol.String(), tf.String(), from.Format(time.RFC3339), to.Format(time.RFC3339))
}

// payloadItem is the serialized form for a candle in the cache.
type payloadItem struct {
	Timestamp string  `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
}

// GetSeries implements the ports.CandleRepositoryPort interface.
func (r *RedisCandleRepository) GetSeries(symbol domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	key := cacheKey(symbol, tf, from.UTC(), to.UTC())
	fmt.Printf("[RedisCandleRepository] GetSeries: symbol=%s, tf=%s, from=%s, to=%s\n", symbol.String(), tf.String(), from.Format(time.RFC3339), to.Format(time.RFC3339))

	// Try cache
	if r.client != nil {
		b, err := r.client.Get(key)
		if err == nil && len(b) > 0 {
			// unmarshal and reconstruct
			var items []payloadItem
			if err := json.Unmarshal(b, &items); err == nil {
				candles := make([]domain.Candle, 0, len(items))
				for _, it := range items {
					ts, perr := time.Parse(time.RFC3339, it.Timestamp)
					if perr != nil {
						// treat as cache miss/fallback
						break
					}
					c, cerr := domain.NewCandle(symbol, tf, ts.UTC(), it.Open, it.High, it.Low, it.Close, it.Volume)
					if cerr != nil {
						// treat as cache miss/fallback
						break
					}
					candles = append(candles, c)
				}
				// If successfully reconstructed all candles, return series
				if len(candles) == len(items) {
					fmt.Printf("[RedisCandleRepository] cache hit: symbol=%s, tf=%s, count=%d\n", symbol.String(), tf.String(), len(candles))
					return domain.NewCandleSeries(symbol, tf, candles)
				}
			}
		}
	}

	// Cache miss or client absent -> delegate to wrapped repository
	fmt.Printf("[RedisCandleRepository] cache miss or client absent: symbol=%s, tf=%s\n", symbol.String(), tf.String())
	series, err := r.wrapped.GetSeries(symbol, tf, from, to)
	if err != nil {
		fmt.Printf("[RedisCandleRepository] wrapped repo error: symbol=%s, tf=%s, err=%v\n", symbol.String(), tf.String(), err)
		return domain.CandleSeries{}, err
	}
	all := series.All()
	fmt.Printf("[RedisCandleRepository] wrapped repo returned: symbol=%s, tf=%s, count=%d\n", symbol.String(), tf.String(), len(all))

	// Attempt to cache the result; ignore cache errors
	if r.client != nil {
		// ...existing code...
		items := make([]payloadItem, 0, len(all))
		for _, c := range all {
			items = append(items, payloadItem{
				Timestamp: c.Timestamp().Format(time.RFC3339),
				Open:      c.Open(),
				High:      c.High(),
				Low:       c.Low(),
				Close:     c.Close(),
				Volume:    c.Volume(),
			})
		}
		if len(items) > 0 {
			if b, merr := json.Marshal(items); merr == nil {
				_ = r.client.Set(key, b, r.ttl)
			}
		}
	}

	return series, nil
}

// GetLastNCandles retrieves the last N completed candles for a given symbol and timeframe.
// Delegates to wrapped repository.
func (r *RedisCandleRepository) GetLastNCandles(symbol domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	if n <= 0 {
		return domain.NewCandleSeries(symbol, tf, []domain.Candle{})
	}

	fmt.Printf("[RedisCandleRepository] GetLastNCandles: symbol=%s, tf=%s, n=%d\n", symbol.String(), tf.String(), n)

	// Delegate to wrapped repository (no caching for now, as candles are time-sensitive)
	return r.wrapped.GetLastNCandles(symbol, tf, n)
}
