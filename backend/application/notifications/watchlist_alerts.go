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

// WatchlistProvider returns the symbols on a user's watchlist
// (infrastructure/watchlist.SQLiteStore satisfies this — ROADMAP PR-101).
type WatchlistProvider interface {
	Get(ctx context.Context, userID string) ([]string, error)
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

func (s *Scheduler) checkWatchlistTransitions(ctx context.Context) {
	if s.watchlists == nil || s.regimes == nil || s.watchlistState == nil || s.configs == nil {
		return
	}

	configs, err := s.configs.All()
	if err != nil {
		log.Printf("[notify-scheduler] fetch configs for watchlist error: %v", err)
		return
	}

	now := s.now()
	stacks := make(map[string]watchlistStack)
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}
		if !cfg.WatchlistTransitions {
			continue
		}
		if !s.userHasProAccess(ctx, cfg.UserID) {
			continue
		}

		symbols, err := s.watchlists.Get(ctx, cfg.UserID)
		if err != nil {
			log.Printf("[notify-scheduler] watchlist symbols user=%s error: %v", cfg.UserID, err)
			continue
		}

		for _, symbol := range symbols {
			if ctx.Err() != nil {
				return
			}

			cached, ok := stacks[symbol]
			if !ok {
				stack, err := s.regimes.Calculate(ctx, symbol)
				cached = watchlistStack{stack: stack, err: err}
				stacks[symbol] = cached
			}
			if cached.err != nil {
				log.Printf("[notify-scheduler] watchlist regime symbol=%s error: %v", symbol, cached.err)
				continue
			}

			s.checkWatchlistSymbol(ctx, cfg, symbol, cached.stack, now)
		}
	}
}

// checkWatchlistSymbol compares symbol's current dominant regime (on
// cfg.WatchlistTimeframe, read from stack — the tick's shared, already-
// computed regime stack for this symbol) against the last-observed state,
// notifying on a tracked transition subject to the dedup window. The
// current state is persisted whenever it's safe to stop tracking the old
// "from" — see the SendToUser failure branch below for the one case where
// it deliberately isn't.
func (s *Scheduler) checkWatchlistSymbol(ctx context.Context, cfg NotificationConfig, symbol string, stack mtf.Stack, now time.Time) {
	timeframe := cfg.WatchlistTimeframe

	var frame *mtf.TFRegime
	for i := range stack.Frames {
		if stack.Frames[i].Timeframe == timeframe {
			frame = &stack.Frames[i]
			break
		}
	}
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
	// the SendToUser failure branch, which deliberately leaves it alone).
	next := prev
	defer func() {
		if err := s.watchlistState.SetState(ctx, cfg.UserID, symbol, timeframe, next); err != nil {
			log.Printf("[notify-scheduler] watchlist state save user=%s symbol=%s error: %v", cfg.UserID, symbol, err)
		}
	}()

	if !found || prev.State == frame.Dominant {
		next.State = frame.Dominant
		return // first sighting (nothing to compare against), or no change
	}

	title, ok := watchlistTransitionTitle(prev.State, frame.Dominant, frame.Bias)
	if !ok {
		next.State = frame.Dominant
		return // not one of the three transitions this feature tracks
	}

	// frame.Timeframe (== timeframe) came from stack.Frames, which
	// application/mtf.Service only ever populates via domain.NewTimeframe
	// on appeval.DefaultTimeframes — always a valid canonical timeframe.
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return // defensive; should never happen — leaves next == prev, retried next tick
	}
	dedupWindow := watchlistDedupBars * tf.Duration()
	if !prev.NotifiedAt.IsZero() && now.Sub(prev.NotifiedAt) < dedupWindow {
		// Already notified for this transition recently — nothing lost by
		// advancing state, we just skip the redundant repeat right now.
		next.State = frame.Dominant
		return
	}

	sendErr := s.engine.SendToUser(ctx, cfg.UserID, Notification{
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
		// watchlist transitions is the explicit dedupWindow check above,
		// scaled to the user's own timeframe (could be far shorter or
		// longer than 24h). A Key stable across ticks would let the
		// engine's independent, fixed 24h TTL silently re-suppress a
		// second occurrence this dedupWindow check has already decided
		// is legitimate (e.g. the same symbol flapping compression ->
		// expansion twice, 5 bars apart, well inside 24h).
		Key: fmt.Sprintf("watchlist_%s_%s_%s_%s_%d", symbol, timeframe, prev.State, frame.Dominant, now.Unix()),
	})
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
