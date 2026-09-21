package domain

import "time"

// EvaluationRefreshInterval is max(30s, tf/4). Shared by the eval store
// writer, Redis TTL, and (PR-089b) reader stale checks — keep one definition.
func EvaluationRefreshInterval(tf Timeframe) time.Duration {
	quarter := tf.Duration() / 4
	const floor = 30 * time.Second
	if quarter < floor {
		return floor
	}
	return quarter
}

// EvaluationStoreTTL is how long Put keys live: 2× refresh interval, floor 10m,
// so a reader using "stale after 2× interval" still sees data between ticks.
func EvaluationStoreTTL(tf Timeframe) time.Duration {
	const floor = 10 * time.Minute
	ttl := 2 * EvaluationRefreshInterval(tf)
	if ttl < floor {
		return floor
	}
	return ttl
}

// EvaluationStaleAfter is how old a store Put may be before readers fall back
// to live compute (PR-089b): exactly 2 × refresh interval.
func EvaluationStaleAfter(tf Timeframe) time.Duration {
	return 2 * EvaluationRefreshInterval(tf)
}
