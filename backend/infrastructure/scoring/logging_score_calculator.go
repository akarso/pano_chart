// Package scoring holds infrastructure-level decorators around
// domain/scoring calculators — logging, sampling, and other cross-cutting
// concerns that don't belong in the pure scoring algorithms themselves.
package scoring

import (
	"context"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"pano_chart/backend/application/ports"
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
	sampleRate float64 // 0..1; fraction of Score() calls that are observed
	// sink is swapped atomically. SetSink may run concurrently with Score.
	sink atomic.Pointer[sinkSlot]
}

type sinkSlot struct {
	sink ports.ScoreSampleSink
}

// SampleRateFromEnv parses PC_SCORE_SAMPLE_RATE. Empty, non-finite, and
// otherwise invalid values use the default 0.1. The constructor still clamps
// a finite result to [0, 1].
func SampleRateFromEnv(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0.1
	}
	rate, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0.1
	}
	return rate
}

// SetSink records samples instead of printing them. A nil sink restores logging.
// Safe to call concurrently with Score.
func (c *LoggingScoreCalculator) SetSink(sink ports.ScoreSampleSink) {
	if c == nil {
		return
	}
	if sink == nil {
		c.sink.Store(nil)
		return
	}
	c.sink.Store(&sinkSlot{sink: sink})
}

// NewLoggingScoreCalculator constructs the decorator. sampleRate is clamped
// to [0, 1].
func NewLoggingScoreCalculator(inner domainscoring.SymbolScoreCalculator, sampleRate float64) *LoggingScoreCalculator {
	if math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) || sampleRate < 0 {
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
// calls, either records the result or logs it. A configured sink replaces
// logging. rand.Float64() uses the default global Source, which is safe for
// concurrent use, so no locking is needed here even though scoring may run
// across many goroutines (e.g. the ranking pipeline's bounded worker pool).
func (c *LoggingScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	score, err := c.inner.Score(series)
	if err != nil || !shouldSample(c.sampleRate) {
		return score, err
	}
	if slot := c.sink.Load(); slot != nil && slot.sink != nil {
		recErr := slot.sink.Record(
			context.Background(),
			c.inner.Name(),
			series.Symbol().String(),
			series.Timeframe().String(),
			score,
			time.Now().UTC(),
		)
		if recErr != nil {
			log.Printf("[%s] score sample: %v", c.inner.Name(), recErr)
		}
		return score, nil
	}
	log.Printf("[%s] score=%.4f", c.inner.Name(), score)
	return score, nil
}

func shouldSample(rate float64) bool {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return rand.Float64() < rate
}
