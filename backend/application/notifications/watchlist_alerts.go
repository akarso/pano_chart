package notifications

import (
	"context"
	"fmt"
	"log"
	"time"

	"pano_chart/backend/application/mtf"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// WatchlistProvider returns the symbols on a user's watchlist, and which
// users have one at all (infrastructure/watchlist.SQLiteStore satisfies
// this — ROADMAP PR-101).
type WatchlistProvider interface {
	Get(ctx context.Context, userID string) ([]string, error)

	// Users returns every user ID with at least one watchlisted symbol.
	// checkWatchlistTransitions scans this set directly (PR-101 CR) rather
	// than NotificationConfigStore.All()'s saved-config rows: a user who
	// has never saved any notification preference — plausible for a
	// brand-new user who goes straight to starring a symbol — has no row
	// there at all, so All() would silently skip them forever even though
	// Get(userID) (used per-user below) correctly defaults
	// WatchlistTransitions to true for exactly that case.
	Users(ctx context.Context) ([]string, error)
}

// RegimeStackProvider builds a symbol's multi-timeframe regime stack.
// application/mtf.Service satisfies this (PR-099) — reused here rather
// than recomputing anything, since it's already "cheap enough to call per
// rankings row" per its own doc.
type RegimeStackProvider interface {
	Calculate(ctx context.Context, symbol string) (mtf.Stack, error)
}

// WatchlistTransitionState is the last dominant regime observed for one
// (user, symbol, timeframe), plus when a transition notification was last
// actually sent for it (zero if never).
type WatchlistTransitionState struct {
	State      mkt.State
	NotifiedAt time.Time
}

// WatchlistStateStore persists WatchlistTransitionState so a scheduler
// restart doesn't lose transition history (which would otherwise treat the
// next observation as a false "first sighting") or dedup state (which
// would let a restart re-fire an alert inside its dedup window).
type WatchlistStateStore interface {
	GetState(ctx context.Context, userID, symbol, timeframe string) (WatchlistTransitionState, bool, error)
	SetState(ctx context.Context, userID, symbol, timeframe string, state WatchlistTransitionState) error

	// LastNotifiedAt returns the most recent NotifiedAt recorded for
	// (userID, symbol) across every timeframe it has ever been tracked
	// under, or the zero Time if never notified (PR-101 CR). The "(user,
	// symbol)" dedup window (see watchlistDedupBars) must not reset just
	// because the user changed their WatchlistTimeframe setting in
	// between — that config is a single per-user value covering every
	// watchlisted symbol, and GetState/SetState above are necessarily
	// scoped per-timeframe too (a "current regime" is only meaningful
	// relative to one specific timeframe), so a plain per-row NotifiedAt
	// would let a timeframe change silently start a fresh, unrelated
	// dedup clock for a symbol that was already notified about very
	// recently under the old setting.
	LastNotifiedAt(ctx context.Context, userID, symbol string) (time.Time, error)
}

// watchlistDedupBars is how many bars of the user's chosen timeframe must
// elapse between two transition notifications for the same (user, symbol)
// — ROADMAP PR-101 spec: "at most one push per (user, symbol) per 4 bars".
const watchlistDedupBars = 4

// SetWatchlistProvider attaches the watchlist symbol source. When unset,
// checkWatchlistTransitions is a no-op.
func (s *Scheduler) SetWatchlistProvider(p WatchlistProvider) { s.watchlists = p }

// SetRegimeStackProvider attaches the per-symbol regime stack source.
func (s *Scheduler) SetRegimeStackProvider(p RegimeStackProvider) { s.regimes = p }

// SetWatchlistStateStore attaches the transition/dedup state store.
func (s *Scheduler) SetWatchlistStateStore(store WatchlistStateStore) { s.watchlistState = store }

// CheckWatchlistTransitions evaluates every watchlisted symbol's dominant
// regime for a tracked transition (exported for direct / test invocation).
func (s *Scheduler) CheckWatchlistTransitions(ctx context.Context) {
	s.checkWatchlistTransitions(ctx)
}

// watchlistStack caches one regimes.Calculate result (or error) for a tick,
// so N users watching the same symbol share one Calculate call instead of
// each triggering their own redundant evaluation-store read.
type watchlistStack struct {
	stack mtf.Stack
	err   error
}

// checkWatchlistTransitions is the per-tick entry point: it only fans out
// — one job — resolving which users/symbols are in scope and dispatching
// each to checkWatchlistSymbol. The gating and caching concerns it used to
// inline are split out below (watchlistEligibility, regimeStackFor) so
// this loop reads as a single pipeline (AGENTS.md: "methods do one thing").
func (s *Scheduler) checkWatchlistTransitions(ctx context.Context) {
	if s.watchlists == nil || s.regimes == nil || s.watchlistState == nil || s.configs == nil {
		return
	}

	userIDs, err := s.watchlists.Users(ctx)
	if err != nil {
		log.Printf("[notify-scheduler] fetch watchlist users error: %v", err)
		return
	}

	now := s.now()
	stacks := make(map[string]watchlistStack)
	for _, userID := range userIDs {
		if ctx.Err() != nil {
			return
		}

		cfg, eligible := s.watchlistEligibility(ctx, userID)
		if !eligible {
			continue
		}

		symbols, err := s.watchlists.Get(ctx, userID)
		if err != nil {
			log.Printf("[notify-scheduler] watchlist symbols user=%s error: %v", userID, err)
			continue
		}

		for _, symbol := range symbols {
			if ctx.Err() != nil {
				return
			}

			stack, err := s.regimeStackFor(ctx, stacks, symbol)
			if err != nil {
				log.Printf("[notify-scheduler] watchlist regime symbol=%s error: %v", symbol, err)
				continue
			}

			s.checkWatchlistSymbol(ctx, cfg, symbol, stack, now)
		}
	}
}

// watchlistEligibility reports whether userID should be scanned this tick,
// returning their config for reuse if so. Get, not All: correctly defaults
// WatchlistTransitions/WatchlistTimeframe for a user with no saved config
// row at all (PR-101 CR) — see WatchlistProvider.Users's doc.
func (s *Scheduler) watchlistEligibility(ctx context.Context, userID string) (NotificationConfig, bool) {
	cfg, err := s.configs.Get(userID)
	if err != nil {
		log.Printf("[notify-scheduler] watchlist config user=%s error: %v", userID, err)
		return NotificationConfig{}, false
	}
	if !cfg.WatchlistTransitions {
		return NotificationConfig{}, false
	}
	if !s.userHasProAccess(ctx, userID) {
		return NotificationConfig{}, false
	}
	return cfg, true
}

// regimeStackFor returns symbol's regime stack for this tick, computing
// and caching it on first request so every user watching the same symbol
// shares one Calculate call.
func (s *Scheduler) regimeStackFor(ctx context.Context, stacks map[string]watchlistStack, symbol string) (mtf.Stack, error) {
	cached, ok := stacks[symbol]
	if !ok {
		stack, err := s.regimes.Calculate(ctx, symbol)
		cached = watchlistStack{stack: stack, err: err}
		stacks[symbol] = cached
	}
	return cached.stack, cached.err
}

// checkWatchlistSymbol compares symbol's current dominant regime (on
// cfg.WatchlistTimeframe, read from stack — the tick's shared, already-
// computed regime stack for this symbol) against the last-observed state,
// notifying on a tracked transition subject to the dedup and quiet-hours
// gates. It is an orchestrator over single-purpose steps below
// (frameFor, evaluateWatchlistTransition, isWithinWatchlistDedupWindow,
// sendWatchlistNotification); the one thing it still owns directly is
// deciding, per return path, whether next (the state to persist) advances
// to frame.Dominant — see the sendWatchlistNotification failure branch for
// the one case where it deliberately doesn't.
func (s *Scheduler) checkWatchlistSymbol(ctx context.Context, cfg NotificationConfig, symbol string, stack mtf.Stack, now time.Time) {
	timeframe := cfg.WatchlistTimeframe

	frame := frameFor(stack, timeframe)
	if frame == nil {
		return // no fresh frame for this timeframe right now
	}

	prev, found, err := s.watchlistState.GetState(ctx, cfg.UserID, symbol, timeframe)
	if err != nil {
		log.Printf("[notify-scheduler] watchlist state read user=%s symbol=%s error: %v", cfg.UserID, symbol, err)
		return
	}

	// Starts equal to prev — i.e. "nothing changed yet". Only advanced to
	// frame.Dominant once we're sure it's safe to stop tracking prev.State
	// as the pending "from" (every return below sets it explicitly except
	// the send-failure branch, which deliberately leaves it alone).
	next := prev
	defer func() {
		if err := s.watchlistState.SetState(ctx, cfg.UserID, symbol, timeframe, next); err != nil {
			log.Printf("[notify-scheduler] watchlist state save user=%s symbol=%s error: %v", cfg.UserID, symbol, err)
		}
	}()

	title, tracked := evaluateWatchlistTransition(prev, found, frame)
	if !tracked {
		next.State = frame.Dominant
		return // first sighting, no change, or not one of the tracked transitions
	}

	deduped, err := s.isWithinWatchlistDedupWindow(ctx, cfg.UserID, symbol, timeframe, now)
	if err != nil {
		log.Printf("[notify-scheduler] watchlist dedup check user=%s symbol=%s error: %v", cfg.UserID, symbol, err)
		return
	}
	if deduped {
		// Already notified for this transition recently — nothing lost by
		// advancing state, we just skip the redundant repeat right now.
		next.State = frame.Dominant
		return
	}

	// Quiet hours: SendToUser would return nil below without actually
	// attempting delivery (Engine's own quiet-hours check), which would
	// otherwise be indistinguishable from a real, successful send. Checked
	// explicitly, before sending, so this transition is deferred (next
	// stays == prev) rather than being marked delivered and lost — same
	// reasoning as the send-failure branch below (PR-101 CR).
	if !s.engine.WithinAllowedHours() {
		log.Printf("[notify-scheduler] watchlist: user=%s symbol=%s deferred (quiet hours)", cfg.UserID, symbol)
		return
	}

	sendErr := s.sendWatchlistNotification(ctx, cfg, symbol, timeframe, title, prev, frame, now)
	if sendErr != nil {
		// Deliberately leave next == prev: the notification never reached
		// the user, so this transition is still "owed". The next tick
		// re-compares frame.Dominant against this same, unchanged
		// prev.State — if the market hasn't moved on to a third state by
		// then, the identical transition is detected again and retried,
		// instead of being silently and permanently lost because one send
		// attempt failed.
		log.Printf("[notify-scheduler] watchlist: user=%s symbol=%s send failed, transition not recorded (will retry next tick): %v", cfg.UserID, symbol, sendErr)
		return
	}
	next.State = frame.Dominant
	next.NotifiedAt = now
}

// frameFor returns the frame in stack matching timeframe, or nil if the
// stack has no fresh frame for it right now.
func frameFor(stack mtf.Stack, timeframe string) *mtf.TFRegime {
	for i := range stack.Frames {
		if stack.Frames[i].Timeframe == timeframe {
			return &stack.Frames[i]
		}
	}
	return nil
}

// evaluateWatchlistTransition decides whether frame's dominant regime is
// one of the three transitions this feature tracks, given the previously
// observed state. tracked=false covers a first sighting (found=false), no
// change, and an untracked transition shape alike — in every one of those
// cases the caller still needs to advance its persisted state to
// frame.Dominant, it just never notifies.
func evaluateWatchlistTransition(prev WatchlistTransitionState, found bool, frame *mtf.TFRegime) (title string, tracked bool) {
	if !found || prev.State == frame.Dominant {
		return "", false
	}
	return watchlistTransitionTitle(prev.State, frame.Dominant, frame.Bias)
}

// isWithinWatchlistDedupWindow reports whether (userID, symbol) was
// notified recently enough, relative to timeframe's bar duration, that a
// new transition should be suppressed rather than sent again.
//
// Checks LastNotifiedAt (cross-timeframe), not a single row's NotifiedAt
// (PR-101 CR): the dedup window is a per-(user,symbol) rule, but
// GetState/SetState are necessarily scoped per-timeframe (a "current
// regime" only means anything relative to one timeframe) — reading only
// the current timeframe's row would let a user reset their own cooldown
// just by changing WatchlistTimeframe, since the new timeframe's row has
// no memory of a very recent send under the old one.
func (s *Scheduler) isWithinWatchlistDedupWindow(ctx context.Context, userID, symbol, timeframe string, now time.Time) (bool, error) {
	// timeframe came from a frame in stack.Frames, which
	// application/mtf.Service only ever populates via domain.NewTimeframe
	// on appeval.DefaultTimeframes — always a valid canonical timeframe, so
	// this error is defensive and should never trigger. Surfaced as an
	// error (not swallowed) so the caller's existing failure path applies
	// uniformly: leave state untouched, retry next tick.
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return false, err
	}

	lastNotified, err := s.watchlistState.LastNotifiedAt(ctx, userID, symbol)
	if err != nil {
		return false, err
	}

	dedupWindow := watchlistDedupBars * tf.Duration()
	return !lastNotified.IsZero() && now.Sub(lastNotified) < dedupWindow, nil
}

// sendWatchlistNotification builds and sends the transition notification.
func (s *Scheduler) sendWatchlistNotification(
	ctx context.Context,
	cfg NotificationConfig,
	symbol, timeframe, title string,
	prev WatchlistTransitionState,
	frame *mtf.TFRegime,
	now time.Time,
) error {
	return s.engine.SendToUser(ctx, cfg.UserID, Notification{
		Type:  TypeWatchlist,
		Title: title,
		Body:  fmt.Sprintf("%s (%s): %s → %s", symbol, timeframe, prev.State, frame.Dominant),
		Data: map[string]string{
			"type":      string(TypeWatchlist),
			"symbol":    symbol,
			"timeframe": timeframe,
			"from":      string(prev.State),
			"to":        string(frame.Dominant),
			"bias":      frame.Bias,
			"score":     fmt.Sprintf("%.4f", frame.Score),
		},
		// Key includes now's Unix timestamp — unlike the other checks'
		// keys (e.g. market's, which deliberately embeds only a calendar
		// date so the *engine's own* 24h dedup collapses same-day
		// repeats), the business rule that should govern repeated
		// watchlist transitions is isWithinWatchlistDedupWindow above,
		// scaled to the user's own timeframe (could be far shorter or
		// longer than 24h). A Key stable across ticks would let the
		// engine's independent, fixed 24h TTL silently re-suppress a
		// second occurrence that check has already decided is legitimate
		// (e.g. the same symbol flapping compression -> expansion twice,
		// 5 bars apart, well inside 24h).
		Key: fmt.Sprintf("watchlist_%s_%s_%s_%s_%d", symbol, timeframe, prev.State, frame.Dominant, now.Unix()),
	})
}

// watchlistTransitionTitle maps a (from, to) dominant-regime transition to
// its notification title, per ROADMAP PR-101. Any other transition
// (including from == to, already filtered by the caller) is untracked.
func watchlistTransitionTitle(from, to mkt.State, bias string) (string, bool) {
	switch {
	case from == mkt.StateCompression && to == mkt.StateExpansion:
		return "Breakout starting", true
	case from == mkt.StateSideways && to == mkt.StateTrend:
		return fmt.Sprintf("Trend starting, bias %s", bias), true
	case from == mkt.StateTrend && (to == mkt.StateSideways || to == mkt.StateCompression):
		return "Trend pausing", true
	default:
		return "", false
	}
}
