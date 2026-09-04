package notifications

import (
	"sync"
	"time"
)

// marketRegimeChange is the winning (label, timeframe) a user was (or
// would have been) notified about, and when that combination was first
// observed — the anchor for SchedulerConfig.MarketRegimeHoldDuration.
type marketRegimeChange struct {
	label     string
	timeframe string
	at        time.Time
}

// marketRegimeHoldSweepEvery mirrors adapters/http/middleware's
// keyLimiterCleanupEvery: an opportunistic sweep every N evaluations, plus
// a time-based fallback in sweep for low-traffic schedulers that might
// never reach this count.
const marketRegimeHoldSweepEvery = 500

// marketRegimeHold decides whether a per-user market notification should
// be suppressed by a minimum hold duration, and evicts state for users who
// haven't triggered a change in a while — see
// SchedulerConfig.MarketRegimeHoldDuration and PR-075's CR follow-ups
// (regime-flap notification spam, and unbounded per-user memory growth
// over a long-running process's lifetime). Extracted out of
// Scheduler.checkMarketForUser so this small state machine — timing
// decisions, mutation, eviction — is independently testable without the
// rest of the scheduler/engine/config-store machinery.
//
// Keyed on userID alone, not "userID|timeframe": a user's different
// regimes can sit on independently-configured timeframes (Uptrend on 15m,
// Downtrend on 1h, say), and keying on the winning timeframe as well as
// the label let the winning *timeframe* flip carry the same
// notification-spam bug the hold exists to prevent, just on a different
// axis. The user sees one "Market Update" stream regardless of which
// timeframe triggered a given push, so this is a per-user throttle on
// that stream, not a per-timeframe one.
type marketRegimeHold struct {
	mu           sync.Mutex
	changes      map[string]marketRegimeChange
	holdDuration time.Duration

	requestCount uint64
	lastSweep    time.Time
}

func newMarketRegimeHold(holdDuration time.Duration) *marketRegimeHold {
	return &marketRegimeHold{
		changes:      make(map[string]marketRegimeChange),
		holdDuration: holdDuration,
	}
}

// evaluate reports whether a notification for userID about (label,
// timeframe) at time now should be suppressed by the hold, and records it
// as the new anchor when it represents an accepted change. prev is the
// previously recorded change (zero value if there was none), returned so
// the caller can log what it was superseded by (or held against) without
// this type needing to know anything about logging.
//
// This only gates CHANGES, not repeats: a steady (label, timeframe) is
// already correctly deduped once per day by the Engine's own per-key
// dedup, and that path is left alone here. Only when (label, timeframe)
// differs from the last recorded change does evaluate check whether
// holdDuration has actually elapsed since that change — and only a change
// that clears the hold updates the recorded state. A suppressed flap does
// NOT reset the anchor: if it did, a market that keeps changing (but
// never clearing the hold) could slide the window forward indefinitely
// and never let a real change through. Anchoring only to accepted changes
// guarantees a decision point every holdDuration.
func (h *marketRegimeHold) evaluate(userID, label, timeframe string, now time.Time) (suppressed bool, prev marketRegimeChange) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sweep(now)

	prev, hadPrev := h.changes[userID]
	changed := !hadPrev || prev.label != label || prev.timeframe != timeframe
	if hadPrev && changed && now.Sub(prev.at) < h.holdDuration {
		return true, prev
	}
	if changed {
		h.changes[userID] = marketRegimeChange{label: label, timeframe: timeframe, at: now}
	}
	return false, prev
}

// sweep evicts entries whose last recorded change is older than
// holdDuration. Once that much time has passed, the entry can no longer
// affect any future evaluate() decision (holdDuration has already elapsed,
// so any candidate would be accepted regardless of what's recorded) —
// unlike the HTTP rate limiter's eviction (see
// adapters/http/middleware.safeEvictionTTL), which has to worry about
// resetting a token-bucket burst, dropping a hold entry once it's past its
// holdDuration is always safe. Without this, the map would retain one
// entry per distinct user for the lifetime of the process, including
// users who later disable market notifications entirely or lose
// eligibility — unbounded growth with the historical user population on a
// process that otherwise runs indefinitely.
func (h *marketRegimeHold) sweep(now time.Time) {
	h.requestCount++
	if h.requestCount%marketRegimeHoldSweepEvery != 0 && now.Sub(h.lastSweep) <= h.holdDuration {
		return
	}
	for k, v := range h.changes {
		if now.Sub(v.at) > h.holdDuration {
			delete(h.changes, k)
		}
	}
	h.lastSweep = now
}
