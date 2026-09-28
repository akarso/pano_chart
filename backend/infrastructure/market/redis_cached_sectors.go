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

// sectorCacheDTO is a compact Redis payload (tagged fields).
type sectorCacheDTO struct {
	Timeframe         string           `json:"timeframe"`
	MarketSymbolCount int              `json:"marketSymbolCount"`
	Sectors           []sectorCacheRow `json:"sectors"`
}

type sectorCacheRow struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	SymbolCount int              `json:"symbolCount"`
	Points      []sectorPointDTO `json:"points"`
	Return      float64          `json:"return"`
	RS          float64          `json:"rs"`
	RSAvailable bool             `json:"rsAvailable"`
}

type sectorPointDTO struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}

// sectorsComputeTimeout bounds the shared full-universe calculation so a
// disconnected caller can't leave it running indefinitely.
const sectorsComputeTimeout = 30 * time.Second

// RedisCachedSectors caches sector index results (PR-098).
// Key: {prefix}:{normalizedTimeframe}:{limit}. TTL min(base, tf/2).
type RedisCachedSectors struct {
	next      metrics.SectorsCalculator
	redis     CompositeRedisClient
	ttl       time.Duration
	keyPrefix string
	sf        singleflight.Group
}

// NewRedisCachedSectors constructs the decorator.
func NewRedisCachedSectors(
	next metrics.SectorsCalculator,
	redis CompositeRedisClient,
	ttl time.Duration,
	keyPrefix string,
) *RedisCachedSectors {
	return &RedisCachedSectors{
		next:      next,
		redis:     redis,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

// Calculate tries Redis first, otherwise delegates and stores non-empty results.
func (c *RedisCachedSectors) Calculate(
	ctx context.Context,
	timeframe string,
	limit int,
) (mkt.SectorIndexResult, error) {
	key := fmt.Sprintf("%s:%s:%d", c.keyPrefix, timeframe, limit)

	if result, ok := c.fromCache(ctx, key); ok {
		return result, nil
	}

	ch := c.sf.DoChan(key, func() (interface{}, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sectorsComputeTimeout)
		defer cancel()
		if result, ok := c.fromCache(workCtx, key); ok {
			return result, nil
		}
		result, err := c.next.Calculate(workCtx, timeframe, limit)
		if err != nil {
			return mkt.SectorIndexResult{}, err
		}
		// The candle fetch swallows per-symbol errors, including cancellation, so
		// a fired deadline can still return a "successful" but incomplete result.
		// Treat that as a failure rather than caching a partial universe.
		if workCtx.Err() != nil {
			return mkt.SectorIndexResult{}, workCtx.Err()
		}
		// Cache any non-empty payload, including rsAvailable:false rows —
		// those are deterministic for the current bars.
		if len(result.Sectors) > 0 {
			if data, marshalErr := marshalSectors(result); marshalErr == nil {
				_ = c.redis.Set(workCtx, key, string(data), c.ttlForTimeframe(timeframe))
			}
		}
		return result, nil
	})

	select {
	case <-ctx.Done():
		return mkt.SectorIndexResult{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return mkt.SectorIndexResult{}, res.Err
		}
		return res.Val.(mkt.SectorIndexResult), nil
	}
}

func (c *RedisCachedSectors) fromCache(ctx context.Context, key string) (mkt.SectorIndexResult, bool) {
	cached, err := c.redis.Get(ctx, key)
	if err != nil || cached == "" {
		return mkt.SectorIndexResult{}, false
	}
	result, ok := unmarshalSectors([]byte(cached))
	if !ok || len(result.Sectors) == 0 {
		return mkt.SectorIndexResult{}, false
	}
	return result, true
}

func (c *RedisCachedSectors) ttlForTimeframe(timeframe string) time.Duration {
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

// TTLForTimeframe exposes ttl selection for tests.
func (c *RedisCachedSectors) TTLForTimeframe(timeframe string) time.Duration {
	return c.ttlForTimeframe(timeframe)
}

func marshalSectors(result mkt.SectorIndexResult) ([]byte, error) {
	dto := sectorCacheDTO{
		Timeframe:         result.Timeframe,
		MarketSymbolCount: result.MarketSymbolCount,
		Sectors:           make([]sectorCacheRow, len(result.Sectors)),
	}
	for i, sec := range result.Sectors {
		pts := make([]sectorPointDTO, len(sec.Points))
		for j, p := range sec.Points {
			pts[j] = sectorPointDTO{T: p.Timestamp, V: p.Value}
		}
		dto.Sectors[i] = sectorCacheRow{
			ID:          sec.ID,
			Name:        sec.Name,
			SymbolCount: sec.SymbolCount,
			Points:      pts,
			Return:      sec.Return,
			RS:          sec.RS,
			RSAvailable: sec.RSAvailable,
		}
	}
	return json.Marshal(dto)
}

func unmarshalSectors(data []byte) (mkt.SectorIndexResult, bool) {
	var dto sectorCacheDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return mkt.SectorIndexResult{}, false
	}
	out := mkt.SectorIndexResult{
		Timeframe:         dto.Timeframe,
		MarketSymbolCount: dto.MarketSymbolCount,
		Sectors:           make([]mkt.SectorIndex, len(dto.Sectors)),
	}
	for i, sec := range dto.Sectors {
		pts := make([]mkt.IndexPoint, len(sec.Points))
		for j, p := range sec.Points {
			pts[j] = mkt.IndexPoint{Timestamp: p.T, Value: p.V}
		}
		out.Sectors[i] = mkt.SectorIndex{
			ID:          sec.ID,
			Name:        sec.Name,
			SymbolCount: sec.SymbolCount,
			Points:      pts,
			Return:      sec.Return,
			RS:          sec.RS,
			RSAvailable: sec.RSAvailable,
		}
	}
	return out, true
}
