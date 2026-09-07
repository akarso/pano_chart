package notifications

import (
	"testing"
	"time"
)

// reserveAndCommit mimics the pre-PR-075-CR-follow-up evaluate() API — a
// single call that both decides and (on an accepted change) immediately
// commits the new anchor — for tests below that only care about the
// suppression decision, not the reserve/commit/release split itself
// (which scheduler.go now only completes after Engine.SendToUser
// succeeds; see checkMarketForUser).
func (h *marketRegimeHold) reserveAndCommit(userID, label, timeframe string, now time.Time) (suppressed bool, prev marketRegimeChange) {
	proceed, prev, changed := h.reserve(userID, label, timeframe, now)
	if !proceed {
		return true, prev
	}
	if changed {
		h.commit(userID, label, timeframe, now)
	}
	return false, prev
}

// TestMarketRegimeHold_FirstChangeAlwaysAllowed confirms a user with no
// prior state is never held back.
func TestMarketRegimeHold_FirstChangeAlwaysAllowed(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	suppressed, _ := h.reserveAndCommit("u1", "Uptrend", "1h", now)
	if suppressed {
		t.Fatal("expected the first-ever change for a user to never be suppressed")
	}
}

// TestMarketRegimeHold_SameLabelAndTimeframe_NeverSuppressed confirms a
// repeat of the exact same (label, timeframe) is left to the Engine's own
// per-key dedup — reserve itself must not suppress it (it's not a
// "change" at all).
func TestMarketRegimeHold_SameLabelAndTimeframe_NeverSuppressed(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "1h", now)
	for i := 0; i < 5; i++ {
		now = now.Add(time.Minute)
		if suppressed, _ := h.reserveAndCommit("u1", "Uptrend", "1h", now); suppressed {
			t.Fatalf("repeat #%d of the same (label, timeframe) must not be suppressed by the hold itself", i+1)
		}
	}
}

// TestMarketRegimeHold_ChangeWithinWindow_Suppressed is the core
// regression case: a label (or timeframe) change within holdDuration of
// the last accepted change is held.
func TestMarketRegimeHold_ChangeWithinWindow_Suppressed(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "1h", now)

	now = now.Add(time.Minute)
	suppressed, prev := h.reserveAndCommit("u1", "Downtrend", "1h", now)
	if !suppressed {
		t.Fatal("expected a label change 1 minute after the last accepted change to be held")
	}
	if prev.label != "Uptrend" || prev.timeframe != "1h" {
		t.Errorf("expected prev to be the last accepted change, got %+v", prev)
	}
}

// TestMarketRegimeHold_TimeframeOnlyChange_Suppressed covers the
// second-round CR fix: a change in timeframe alone (same label) must be
// treated as a change, not silently ignored.
func TestMarketRegimeHold_TimeframeOnlyChange_Suppressed(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "15m", now)

	now = now.Add(time.Minute)
	suppressed, _ := h.reserveAndCommit("u1", "Uptrend", "1h", now)
	if !suppressed {
		t.Fatal("expected a timeframe-only change within the hold window to be held")
	}
}

// TestMarketRegimeHold_ChangeAfterWindow_Allowed confirms a genuine change
// well past holdDuration since the last accepted one goes through.
func TestMarketRegimeHold_ChangeAfterWindow_Allowed(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "1h", now)

	now = now.Add(16 * time.Minute)
	suppressed, _ := h.reserveAndCommit("u1", "Downtrend", "1h", now)
	if suppressed {
		t.Fatal("expected a change past the hold window to be allowed")
	}
}

// TestMarketRegimeHold_SuppressedFlapDoesNotResetAnchor is the regression
// test for a bug caught while implementing the hold itself: if a
// suppressed flap updated the anchor's timestamp, a market that keeps
// changing (but never clearing the hold) could slide the window forward
// indefinitely and never let a real change through.
func TestMarketRegimeHold_SuppressedFlapDoesNotResetAnchor(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "1h", now) // t=0, accepted

	// Flap every minute for 14 minutes between two labels that BOTH differ
	// from the accepted anchor ("Uptrend") — reverting to exactly the
	// anchor's label would correctly read as "not a change" (left to the
	// Engine's own dedup) rather than "suppressed by the hold", which
	// would make this loop's assertion meaningless. All within the
	// window, all must be suppressed, and none may move the anchor.
	for i := 0; i < 14; i++ {
		now = now.Add(time.Minute)
		label := "Downtrend"
		if i%2 == 0 {
			label = "Sideways"
		}
		if suppressed, _ := h.reserveAndCommit("u1", label, "1h", now); !suppressed {
			t.Fatalf("flap at minute %d: expected suppression within the hold window", i+1)
		}
	}

	// Now at t=15 (15 minutes after the ORIGINAL accepted change at t=0),
	// a change must be allowed — if a suppressed flap had reset the
	// anchor, this would still be held.
	now = now.Add(time.Minute)
	suppressed, _ := h.reserveAndCommit("u1", "Downtrend", "1h", now)
	if suppressed {
		t.Fatal("expected the hold to clear exactly holdDuration after the last ACCEPTED change, not after the last flap attempt")
	}
}

// TestMarketRegimeHold_UsersAreIndependent confirms one user's hold state
// never affects another's.
func TestMarketRegimeHold_UsersAreIndependent(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("u1", "Uptrend", "1h", now)

	suppressed, _ := h.reserveAndCommit("u2", "Uptrend", "1h", now)
	if suppressed {
		t.Fatal("expected u2's first change to be unaffected by u1's hold state")
	}
}

// TestMarketRegimeHold_SweepEvictsStaleEntries is the regression test for
// the "lastMarketChange never expires" CR finding: an entry older than
// holdDuration must eventually be evicted rather than retained for the
// life of the process, once enough reserve() calls (across any users)
// have accumulated to trigger the count-based sweep.
func TestMarketRegimeHold_SweepEvictsStaleEntries(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("stale-user", "Uptrend", "1h", now)

	// Advance well past holdDuration, then drive enough calls (from an
	// unrelated user) to cross the count-based sweep threshold.
	now = now.Add(time.Hour)
	for i := 0; i < marketRegimeHoldSweepEvery+1; i++ {
		h.reserveAndCommit("active-user", "Uptrend", "1h", now)
	}

	h.mu.Lock()
	_, stillPresent := h.changes["stale-user"]
	h.mu.Unlock()
	if stillPresent {
		t.Error("expected the stale user's entry to be evicted by the sweep")
	}
}

// TestMarketRegimeHold_SweepDoesNotEvictFreshEntries confirms the sweep is
// selective — an entry within holdDuration survives even when the sweep
// runs.
func TestMarketRegimeHold_SweepDoesNotEvictFreshEntries(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	h.reserveAndCommit("fresh-user", "Uptrend", "1h", now)

	for i := 0; i < marketRegimeHoldSweepEvery+1; i++ {
		h.reserveAndCommit("active-user", "Uptrend", "1h", now)
	}

	h.mu.Lock()
	_, stillPresent := h.changes["fresh-user"]
	h.mu.Unlock()
	if !stillPresent {
		t.Error("expected a fresh (within-window) entry to survive the sweep")
	}
}

// TestMarketRegimeHold_ReserveDoesNotCommitUntilCommitIsCalled is the core
// regression test for PR-075 CR follow-up (Issue 2): reserve() alone must
// not record the new anchor — only an explicit commit() does, mirroring
// checkMarketForUser only committing after Engine.SendToUser succeeds.
func TestMarketRegimeHold_ReserveDoesNotCommitUntilCommitIsCalled(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	proceed, _, changed := h.reserve("u1", "Uptrend", "1h", now)
	if !proceed || !changed {
		t.Fatalf("expected the first-ever change to proceed and be marked changed, got proceed=%v changed=%v", proceed, changed)
	}

	h.mu.Lock()
	_, committed := h.changes["u1"]
	h.mu.Unlock()
	if committed {
		t.Fatal("expected reserve() alone to not have committed the anchor yet")
	}
}

// TestMarketRegimeHold_ConcurrentReserveIsBlocked confirms a second
// reserve for the same user while the first is still in flight (not yet
// committed or released) is refused — the scenario the finding calls out:
// an overlapping Run tick and a manually invoked CheckMarketState must not
// both proceed to send for the same change.
func TestMarketRegimeHold_ConcurrentReserveIsBlocked(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	proceed1, _, changed1 := h.reserve("u1", "Uptrend", "1h", now)
	if !proceed1 || !changed1 {
		t.Fatalf("expected the first reserve to proceed, got proceed=%v changed=%v", proceed1, changed1)
	}

	proceed2, _, _ := h.reserve("u1", "Uptrend", "1h", now)
	if proceed2 {
		t.Fatal("expected a second concurrent reserve for the same user to be refused while the first is still in flight")
	}
}

// TestMarketRegimeHold_ReleaseAllowsImmediateRetry is the failed-send
// case: release() (called after SendToUser fails) must not commit an
// anchor, and must clear the in-flight marker so the very next reserve for
// the same change can proceed immediately rather than waiting out the full
// hold window for a change that never actually went out.
func TestMarketRegimeHold_ReleaseAllowsImmediateRetry(t *testing.T) {
	h := newMarketRegimeHold(15 * time.Minute)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	proceed, _, changed := h.reserve("u1", "Uptrend", "1h", now)
	if !proceed || !changed {
		t.Fatalf("expected the reserve to proceed, got proceed=%v changed=%v", proceed, changed)
	}
	h.release("u1")

	h.mu.Lock()
	_, committed := h.changes["u1"]
	h.mu.Unlock()
	if committed {
		t.Fatal("expected release() to not commit an anchor")
	}

	// Retry a moment later (well within what would have been the hold
	// window had the first attempt committed) — must proceed, since
	// nothing was actually recorded.
	now = now.Add(time.Second)
	proceed, _, changed = h.reserve("u1", "Uptrend", "1h", now)
	if !proceed || !changed {
		t.Fatalf("expected an immediate retry after release() to proceed, got proceed=%v changed=%v", proceed, changed)
	}
}
