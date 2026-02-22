package infrastructure

import (
	metrics "pano_chart/backend/infrastructure"
	"sync"
	"sync/atomic"
	"time"
)

type CandleCacheKey struct {
	Symbol    string
	Timeframe string
}

type CachedCandles struct {
	Candles   interface{}
	UpdatedAt time.Time
	TTL       time.Duration
}

type CandleCache struct {
	mu   sync.RWMutex
	data map[CandleCacheKey]CachedCandles
}

func NewCandleCache() *CandleCache {
	return &CandleCache{
		data: make(map[CandleCacheKey]CachedCandles),
	}
}

func (c *CandleCache) Set(key CandleCacheKey, candles interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = CachedCandles{
		Candles:   candles,
		UpdatedAt: time.Now(),
		TTL:       ttl,
	}
}

func (c *CandleCache) Get(key CandleCacheKey) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[key]
	if !ok {
		atomic.AddInt64(&metrics.GlobalMetrics.CacheMisses, 1)
		return nil, false
	}
	if time.Since(entry.UpdatedAt) > entry.TTL {
		atomic.AddInt64(&metrics.GlobalMetrics.CacheMisses, 1)
		return nil, false
	}
	atomic.AddInt64(&metrics.GlobalMetrics.CacheHits, 1)
	return entry.Candles, true
}
