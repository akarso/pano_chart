package market

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// CompositeRedisClient abstracts Redis operations for the cache.
type CompositeRedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedComposite is a decorator that caches CompositeIndexService
// results in Redis. Key format: {prefix}:{timeframe}:{limit} for Calculate,
// {prefix}:tape:{timeframe}:{limit} for CalculateTape.
type RedisCachedComposite struct {
	next      *metrics.CompositeIndexService
	redis     CompositeRedisClient
	ttl       time.Duration
	keyPrefix string
	sf        singleflight.Group
}

// NewRedisCachedComposite constructs the cache decorator.
func NewRedisCachedComposite(
	next *metrics.CompositeIndexService,
	redis CompositeRedisClient,
	ttl time.Duration,
	keyPrefix string,
) *RedisCachedComposite {
	return &RedisCachedComposite{
		next:      next,
		redis:     redis,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

// Calculate tries the cache first, otherwise delegates and stores.
func (c *RedisCachedComposite) Calculate(ctx context.Context, timeframe string, limit int) (mkt.CompositeIndex, error) {
	key := fmt.Sprintf("%s:%s:%d", c.keyPrefix, timeframe, limit)

	// 1. Attempt cache hit
	cached, err := c.redis.Get(ctx, key)
	if err == nil && cached != "" {
		var idx mkt.CompositeIndex
		if unmarshalErr := json.Unmarshal([]byte(cached), &idx); unmarshalErr == nil {
			return idx, nil
		}
	}

	// 2. Cache miss — compute
	idx, err := c.next.Calculate(ctx, timeframe, limit)
	if err != nil {
		return mkt.CompositeIndex{}, err
	}

	// 3. Store (best-effort)
	data, marshalErr := json.Marshal(idx)
	if marshalErr == nil {
		_ = c.redis.Set(ctx, key, string(data), c.ttl)
	}

	return idx, nil
}

// tapeCandleDTO is a flat OHLCV bar for Redis serialization.
// domain.CandleSeries has unexported fields and cannot be marshaled directly.
type tapeCandleDTO struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

type tapeCacheDTO struct {
	Index           mkt.CompositeIndex `json:"index"`
	Median          []tapeCandleDTO    `json:"median"`
	Weighted        []tapeCandleDTO    `json:"weighted"`
	PreferredSource string             `json:"preferredSource"`
	Timeframe       string             `json:"timeframe"`
}

// CalculateTape caches the full CompositeTape (index + synthetic series).
// TTL is timeframe-aware: min(baseTTL, tf/2) so short timeframes do not serve
// stale tapes (e.g. 1m → 30s). Concurrent misses for the same key are
// coalesced with singleflight so only one candle fan-out runs per process.
//
// The shared flight uses a context detached from any single caller so the
// first caller's cancellation cannot abort work (or poison the result) for
// siblings still waiting on the same key.
func (c *RedisCachedComposite) CalculateTape(ctx context.Context, timeframe string, limit int) (metrics.CompositeTape, error) {
	key := fmt.Sprintf("%s:tape:%s:%d", c.keyPrefix, timeframe, limit)

	if tape, ok := c.tapeFromCache(ctx, key); ok {
		return tape, nil
	}

	ch := c.sf.DoChan(key, func() (interface{}, error) {
		workCtx := context.WithoutCancel(ctx)

		// Double-check after winning the flight — another caller may have
		// filled Redis while we waited.
		if tape, ok := c.tapeFromCache(workCtx, key); ok {
			return tape, nil
		}

		tape, err := c.next.CalculateTape(workCtx, timeframe, limit)
		if err != nil {
			return metrics.CompositeTape{}, err
		}
		// Do not cache unusable tapes (empty universe / all fetches failed).
		// Caching those would stick Market Pulse on participation until TTL
		// even after candles recover.
		if tapeUsable(tape) {
			if data, marshalErr := marshalTape(tape, timeframe); marshalErr == nil {
				_ = c.redis.Set(workCtx, key, string(data), c.ttlForTimeframe(timeframe))
			}
		}
		return tape, nil
	})

	select {
	case <-ctx.Done():
		return metrics.CompositeTape{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return metrics.CompositeTape{}, res.Err
		}
		return res.Val.(metrics.CompositeTape), nil
	}
}

func (c *RedisCachedComposite) tapeFromCache(ctx context.Context, key string) (metrics.CompositeTape, bool) {
	cached, err := c.redis.Get(ctx, key)
	if err != nil || cached == "" {
		return metrics.CompositeTape{}, false
	}
	tape, ok := unmarshalTape([]byte(cached))
	if !ok || !tapeUsable(tape) {
		return metrics.CompositeTape{}, false
	}
	return tape, true
}

// tapeUsable is true when the preferred series has enough bars for regime
// scoring (MarketStateService requires PreferredSeries().Len() >= 2).
func tapeUsable(t metrics.CompositeTape) bool {
	return t.PreferredSeries().Len() >= 2
}

func (c *RedisCachedComposite) ttlForTimeframe(timeframe string) time.Duration {
	ttl := c.ttl
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return ttl
	}
	half := tf.Duration() / 2
	if half > 0 && half < ttl {
		return half
	}
	return ttl
}

func marshalTape(tape metrics.CompositeTape, timeframe string) ([]byte, error) {
	dto := tapeCacheDTO{
		Index:           tape.Index,
		Median:          seriesToDTO(tape.MedianSeries),
		Weighted:        seriesToDTO(tape.WeightedSeries),
		PreferredSource: tape.PreferredSource,
		Timeframe:       timeframe,
	}
	if dto.Timeframe == "" {
		dto.Timeframe = tape.Index.Timeframe
	}
	return json.Marshal(dto)
}

func unmarshalTape(data []byte) (metrics.CompositeTape, bool) {
	var dto tapeCacheDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return metrics.CompositeTape{}, false
	}
	tfStr := dto.Timeframe
	if tfStr == "" {
		tfStr = dto.Index.Timeframe
	}
	median, err := seriesFromDTO(tfStr, dto.Median)
	if err != nil {
		return metrics.CompositeTape{}, false
	}
	weighted, err := seriesFromDTO(tfStr, dto.Weighted)
	if err != nil {
		return metrics.CompositeTape{}, false
	}
	return metrics.CompositeTape{
		Index:           dto.Index,
		MedianSeries:    median,
		WeightedSeries:  weighted,
		PreferredSource: dto.PreferredSource,
	}, true
}

func seriesToDTO(series domain.CandleSeries) []tapeCandleDTO {
	if series.Len() == 0 {
		return nil
	}
	out := make([]tapeCandleDTO, 0, series.Len())
	for _, c := range series.All() {
		out = append(out, tapeCandleDTO{
			T: c.Timestamp().Unix(),
			O: c.Open(),
			H: c.High(),
			L: c.Low(),
			C: c.Close(),
			V: c.Volume(),
		})
	}
	return out
}

func seriesFromDTO(timeframe string, bars []tapeCandleDTO) (domain.CandleSeries, error) {
	if len(bars) == 0 {
		return domain.CandleSeries{}, nil
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	synth := domain.NewSymbolUnsafe("COMPOSITE")
	candles := make([]domain.Candle, len(bars))
	for i, b := range bars {
		candles[i] = domain.NewCandleUnsafe(
			synth, tf, time.Unix(b.T, 0).UTC(),
			b.O, b.H, b.L, b.C, b.V,
		)
	}
	return domain.NewCandleSeries(synth, tf, candles)
}
