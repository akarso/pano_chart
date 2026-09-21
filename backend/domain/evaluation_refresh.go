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

// EvaluationStoreFresh reports whether a store timestamp is usable at now:
// age in [0, EvaluationStaleAfter]. Negative age (future / clock skew /
// malformed at) is not fresh.
func EvaluationStoreFresh(at, now time.Time, tf Timeframe) bool {
	if at.IsZero() {
		return false
	}
	age := now.Sub(at)
	if age < 0 {
		return false
	}
	return age <= EvaluationStaleAfter(tf)
}
