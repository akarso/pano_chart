package infrastructure

import (
	"log"
	"sync/atomic"
	"time"
)

type Metrics struct {
	TokenAcquires  int64
	CacheHits      int64
	CacheMisses    int64
	FetchErrors    int64
	Fetch429s      int64
	FetchSuccesses int64
}

var GlobalMetrics Metrics

func StartMetricsLogger() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			tokenAcquires := atomic.LoadInt64(&GlobalMetrics.TokenAcquires)
			cacheHits := atomic.LoadInt64(&GlobalMetrics.CacheHits)
			cacheMisses := atomic.LoadInt64(&GlobalMetrics.CacheMisses)
			fetchErrors := atomic.LoadInt64(&GlobalMetrics.FetchErrors)
			fetch429s := atomic.LoadInt64(&GlobalMetrics.Fetch429s)
			fetchSuccesses := atomic.LoadInt64(&GlobalMetrics.FetchSuccesses)
			log.Printf("[metrics] token_acquires=%d cache_hits=%d cache_misses=%d fetch_errors=%d fetch_429s=%d fetch_successes=%d", tokenAcquires, cacheHits, cacheMisses, fetchErrors, fetch429s, fetchSuccesses)
		}
	}()
}
