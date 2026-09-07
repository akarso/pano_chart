// Package scoring holds infrastructure-level decorators around
// domain/scoring calculators — logging, sampling, and other cross-cutting
// concerns that don't belong in the pure scoring algorithms themselves.
package scoring

import (
	"log"
	"math/rand"

	"pano_chart/backend/domain"
	domainscoring "pano_chart/backend/domain/scoring"
)

// LoggingScoreCalculator wraps a domain SymbolScoreCalculator and logs a
// sampled fraction of its Score() results.
//
// This exists so a calculator's score distribution can be observed in
// production without the domain calculation itself doing any logging or
// using any randomness — see PR-074, where DetectSidewaysV5 originally had
// sampled logging inline, coupling a pure scoring function to process-wide
// logging and making it nondeterministic. The decorator pattern mirrors
// infrastructure/rankings.RedisCachedRankings, which wraps a use case for
// caching the same way this wraps a calculator for observability.
type LoggingScoreCalculator struct {
	inner      domainscoring.SymbolScoreCalculator
	sampleRate float64 // 0..1; fraction of Score() calls that get logged
}

// NewLoggingScoreCalculator constructs the decorator. sampleRate is clamped
// to [0, 1].
func NewLoggingScoreCalculator(inner domainscoring.SymbolScoreCalculator, sampleRate float64) *LoggingScoreCalculator {
	if sampleRate < 0 {
		sampleRate = 0
	}
	if sampleRate > 1 {
		sampleRate = 1
	}
	return &LoggingScoreCalculator{inner: inner, sampleRate: sampleRate}
}

func (c *LoggingScoreCalculator) Name() string {
	return c.inner.Name()
}

// Score delegates to the wrapped calculator and, for a random sample of
// calls, logs the result. rand.Float64() uses the default global Source,
// which is safe for concurrent use, so no locking is needed here even
// though scoring may run across many goroutines (e.g. the ranking
// pipeline's bounded worker pool).
func (c *LoggingScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	score, err := c.inner.Score(series)
	if err == nil && c.sampleRate > 0 && rand.Float64() < c.sampleRate {
		log.Printf("[%s] score=%.4f", c.inner.Name(), score)
	}
	return score, err
}
