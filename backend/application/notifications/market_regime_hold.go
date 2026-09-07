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
	inFlight     map[string]bool
	holdDuration time.Duration

	requestCount uint64
	lastSweep    time.Time
}

func newMarketRegimeHold(holdDuration time.Duration) *marketRegimeHold {
	return &marketRegimeHold{
		changes:      make(map[string]marketRegimeChange),
		inFlight:     make(map[string]bool),
		holdDuration: holdDuration,
	}
}

// reserve reports whether a notification for userID about (label,
// timeframe) at time now should proceed, without yet recording it as the
// new anchor — see commit/release, and PR-075's CR follow-up (Issue 2):
// the anchor must not be committed until Engine.SendToUser has actually
// succeeded, or a failed send would still "use up" the hold window and
// silently swallow the regime change it was theoretically reporting. prev
// is the previously recorded change (zero value if there was none),
// returned so the caller can log what it was superseded by (or held
// against) without this type needing to know anything about logging.
// changed reports whether this call represents an actual (label,
// timeframe) change — the caller must call commit or release only when
// changed is true; when false, reserve has made no reservation and there
// is nothing to commit or release.
//
// This only gates CHANGES, not repeats: a steady (label, timeframe) is
// already correctly deduped once per day by the Engine's own per-key
// dedup (and protected against concurrent duplicate sends by
// Deduplicator's own mutex), and that path is left alone here. Only when
// (label, timeframe) differs from the last recorded change does reserve
// check whether holdDuration has actually elapsed since that change, and
// whether a same-user send for a change is already in flight — a second
// concurrent reserve (e.g. an overlapping Run tick and a manually invoked
// CheckMarketState) must not both proceed and send duplicates. A
// suppressed flap does NOT reset the anchor: if it did, a market that
// keeps changing (but never clearing the hold) could slide the window
// forward indefinitely and never let a real change through. Anchoring
// only to accepted, committed changes guarantees a decision point every
// holdDuration.
func (h *marketRegimeHold) reserve(userID, label, timeframe string, now time.Time) (proceed bool, prev marketRegimeChange, changed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sweep(now)

	if h.inFlight[userID] {
		return false, h.changes[userID], false
	}

	prev, hadPrev := h.changes[userID]
	changed = !hadPrev || prev.label != label || prev.timeframe != timeframe
	if hadPrev && changed && now.Sub(prev.at) < h.holdDuration {
		return false, prev, false
	}
	if changed {
		h.inFlight[userID] = true
	}
	return true, prev, changed
}

// commit records (label, timeframe, now) as userID's new anchor and
// clears its reservation — call only after Engine.SendToUser has
// succeeded for a reserve that reported changed=true.
func (h *marketRegimeHold) commit(userID, label, timeframe string, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.changes[userID] = marketRegimeChange{label: label, timeframe: timeframe, at: now}
	delete(h.inFlight, userID)
}

// release clears userID's reservation without recording a new anchor —
// call after a reserve that reported changed=true when SendToUser then
// fails, so the held state is unchanged and the next check is free to
// retry immediately rather than waiting out the full hold window for a
// change that never actually went out.
func (h *marketRegimeHold) release(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.inFlight, userID)
}

// sweep evicts entries whose last recorded change is older than
// holdDuration. Once that much time has passed, the entry can no longer
// affect any future reserve() decision (holdDuration has already elapsed,
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
