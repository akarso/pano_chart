package notifications_test

import (
	"context"
	"testing"
	"time"

	"pano_chart/backend/application/mtf"
	"pano_chart/backend/application/notifications"
	mkt "pano_chart/backend/domain/market"
)

// ── fakes ────────────────────────────────────────────────────────────────

type fakeWatchlistProvider struct {
	symbols map[string][]string // keyed by userID
	err     error
}

func (f *fakeWatchlistProvider) Get(_ context.Context, userID string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.symbols[userID], nil
}

// fakeRegimeStackProvider returns a fixed dominant/bias for a symbol on a
// single timeframe. Tests set stacks[symbol] to the frame they want back —
// only Timeframe/Dominant/Bias/Score matter to checkWatchlistSymbol. calls
// counts invocations per symbol, to verify the scheduler shares one
// Calculate call across every user watching the same symbol in a tick.
type fakeRegimeStackProvider struct {
	stacks map[string]mtf.Stack
	err    error
	calls  map[string]int
}

func (f *fakeRegimeStackProvider) Calculate(_ context.Context, symbol string) (mtf.Stack, error) {
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[symbol]++
	if f.err != nil {
		return mtf.Stack{}, f.err
	}
	return f.stacks[symbol], nil
}

// oneFrameStack builds a single-frame Stack for symbol on timeframe with the
// given dominant state and bias — enough to drive checkWatchlistSymbol,
// which only ever looks at the one frame matching cfg.WatchlistTimeframe.
func oneFrameStack(symbol, timeframe string, dominant mkt.State, bias string) mtf.Stack {
	return mtf.Stack{
		Symbol: symbol,
		Frames: []mtf.TFRegime{
			{Timeframe: timeframe, Dominant: dominant, Bias: bias, Score: 0.8},
		},
	}
}

// memWatchlistStateStore is an in-memory notifications.WatchlistStateStore.
type memWatchlistStateStore struct {
	states map[string]notifications.WatchlistTransitionState
}

func newMemWatchlistStateStore() *memWatchlistStateStore {
	return &memWatchlistStateStore{states: make(map[string]notifications.WatchlistTransitionState)}
}

func (m *memWatchlistStateStore) key(userID, symbol, timeframe string) string {
	return userID + "|" + symbol + "|" + timeframe
}

func (m *memWatchlistStateStore) GetState(_ context.Context, userID, symbol, timeframe string) (notifications.WatchlistTransitionState, bool, error) {
	s, ok := m.states[m.key(userID, symbol, timeframe)]
	return s, ok, nil
}

func (m *memWatchlistStateStore) SetState(_ context.Context, userID, symbol, timeframe string, state notifications.WatchlistTransitionState) error {
	m.states[m.key(userID, symbol, timeframe)] = state
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────

func newWatchlistScheduler(
	spy *spySender,
	now time.Time,
	watchlists *fakeWatchlistProvider,
	regimes *fakeRegimeStackProvider,
	states *memWatchlistStateStore,
	cfgStore *memConfigStore,
) *notifications.Scheduler {
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time { return now })
	sched := notifications.NewScheduler(eng, nil, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.SetWatchlistProvider(watchlists)
	sched.SetRegimeStackProvider(regimes)
	sched.SetWatchlistStateStore(states)
	return sched
}

func watchlistCfg(userID, timeframe string) notifications.NotificationConfig {
	return notifications.NotificationConfig{
		UserID:               userID,
		WatchlistTransitions: true,
		WatchlistTimeframe:   timeframe,
	}
}

// ── tests ────────────────────────────────────────────────────────────────

// First observation of a symbol has nothing to compare against — must not
// be treated as a "transition into" the observed state.
func TestScheduler_Watchlist_FirstSighting_NoNotification(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateTrend, "up"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification on first sighting, got %d", spy.userCount())
	}
	state, found, _ := states.GetState(context.Background(), "u1", "BTCUSDT", "1h")
	if !found || state.State != mkt.StateTrend {
		t.Fatalf("expected first-sighting state to be persisted as trend, got %+v (found=%v)", state, found)
	}
}

// compression -> expansion is one of the three tracked transitions.
func TestScheduler_Watchlist_CompressionToExpansion_Notifies(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background()) // seeds compression, no notification yet

	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 watchlist notification, got %d", spy.userCount())
	}
	send := spy.lastUserSend()
	if send.userID != "u1" {
		t.Fatalf("expected notification for u1, got %s", send.userID)
	}
	if send.n.Type != notifications.TypeWatchlist {
		t.Fatalf("expected TypeWatchlist, got %s", send.n.Type)
	}
	if send.n.Title != "Breakout starting" {
		t.Fatalf("unexpected title: %s", send.n.Title)
	}
	if send.n.Data["symbol"] != "BTCUSDT" || send.n.Data["from"] != "compression" || send.n.Data["to"] != "expansion" {
		t.Fatalf("unexpected data payload: %+v", send.n.Data)
	}
}

// sideways -> trend includes the bias in the title.
func TestScheduler_Watchlist_SidewaysToTrend_TitleIncludesBias(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"ETHUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"ETHUSDT": oneFrameStack("ETHUSDT", "1h", mkt.StateSideways, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())

	regimes.stacks["ETHUSDT"] = oneFrameStack("ETHUSDT", "1h", mkt.StateTrend, "down")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 watchlist notification, got %d", spy.userCount())
	}
	if got := spy.lastUserSend().n.Title; got != "Trend starting, bias down" {
		t.Fatalf("unexpected title: %s", got)
	}
}

// trend -> sideways and trend -> compression both read as "Trend pausing".
func TestScheduler_Watchlist_TrendPausing_BothTargets(t *testing.T) {
	for _, to := range []mkt.State{mkt.StateSideways, mkt.StateCompression} {
		t.Run(string(to), func(t *testing.T) {
			spy := &spySender{}
			now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

			cfgStore := newMemConfigStore()
			_ = cfgStore.Save(watchlistCfg("u1", "1h"))

			watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"SOLUSDT"}}}
			regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
				"SOLUSDT": oneFrameStack("SOLUSDT", "1h", mkt.StateTrend, "up"),
			}}
			states := newMemWatchlistStateStore()

			sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
			sched.CheckWatchlistTransitions(context.Background())

			regimes.stacks["SOLUSDT"] = oneFrameStack("SOLUSDT", "1h", to, "neutral")
			sched.CheckWatchlistTransitions(context.Background())

			if spy.userCount() != 1 {
				t.Fatalf("expected 1 watchlist notification, got %d", spy.userCount())
			}
			if got := spy.lastUserSend().n.Title; got != "Trend pausing" {
				t.Fatalf("unexpected title: %s", got)
			}
		})
	}
}

// An untracked transition (e.g. expansion -> sideways) must not notify, but
// still updates the persisted state so the next real comparison is correct.
func TestScheduler_Watchlist_UntrackedTransition_NoNotification(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())

	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateSideways, "neutral")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification for an untracked transition, got %d", spy.userCount())
	}
	state, found, _ := states.GetState(context.Background(), "u1", "BTCUSDT", "1h")
	if !found || state.State != mkt.StateSideways {
		t.Fatalf("expected state to still track the latest observation, got %+v (found=%v)", state, found)
	}
}

// Same state repeated (no real transition) must not notify.
func TestScheduler_Watchlist_NoChange_NoNotification(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateTrend, "up"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification for an unchanged regime, got %d", spy.userCount())
	}
}

// A repeated tracked transition inside the 4-bar dedup window must be
// suppressed; once the window elapses, it may fire again.
func TestScheduler_Watchlist_DedupWindow(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h")) // 4 bars * 1h = 4h dedup window

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	clock := now
	eng.SetClock(func() time.Time { return clock })
	sched := notifications.NewScheduler(eng, nil, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return clock })
	sched.SetConfigStore(cfgStore)
	sched.SetWatchlistProvider(watchlists)
	sched.SetRegimeStackProvider(regimes)
	sched.SetWatchlistStateStore(states)

	sched.CheckWatchlistTransitions(context.Background()) // seed compression

	// First transition: compression -> expansion -> notifies.
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())
	if spy.userCount() != 1 {
		t.Fatalf("expected 1 notification after first transition, got %d", spy.userCount())
	}

	// Flap back to compression, then to expansion again within the 4h
	// window — the second compression->expansion must be deduped.
	clock = clock.Add(1 * time.Hour)
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral")
	sched.CheckWatchlistTransitions(context.Background())

	clock = clock.Add(1 * time.Hour)
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())
	if spy.userCount() != 1 {
		t.Fatalf("expected the repeat transition inside the dedup window to be suppressed, got %d", spy.userCount())
	}

	// Elapse past the 4-bar (4h) window from the first notification, then
	// flap again — this one must fire.
	clock = clock.Add(3 * time.Hour) // total 5h since first notification
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral")
	sched.CheckWatchlistTransitions(context.Background())

	clock = clock.Add(1 * time.Minute)
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())
	if spy.userCount() != 2 {
		t.Fatalf("expected a second notification once the dedup window elapsed, got %d", spy.userCount())
	}
}

// WatchlistTransitions=false must skip the user entirely, even with
// symbols and a matching regime transition.
func TestScheduler_Watchlist_FlagOff_NoNotification(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	cfg := watchlistCfg("u1", "1h")
	cfg.WatchlistTransitions = false
	_ = cfgStore.Save(cfg)

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification when WatchlistTransitions is off, got %d", spy.userCount())
	}
}

// A free-tier user is gated out, same as every other pro-only notification
// type.
func TestScheduler_Watchlist_SubscriptionGating_FreeUserSuppressed(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("free-user", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"free-user": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.SetSubscriptionChecker(&fakeSubscriptionChecker{active: map[string]bool{"free-user": false}})
	sched.CheckWatchlistTransitions(context.Background())

	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification for a free-tier user, got %d", spy.userCount())
	}
}

// No frame at the user's configured timeframe (e.g. stale/missing store
// entry) must be skipped, not panic or misfire.
func TestScheduler_Watchlist_NoFrameForTimeframe_Skipped(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "4h")) // configured tf has no frame below

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification when the configured timeframe has no frame, got %d", spy.userCount())
	}
}

// Nil providers must be a no-op, not a panic — matches
// TestScheduler_NilProviders_NoPanic's coverage for the other check types.
func TestScheduler_Watchlist_NilProviders_NoPanic(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	sched := notifications.NewScheduler(eng, nil, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckWatchlistTransitions(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notifications with nil watchlist providers")
	}
}

// Two users watching the same symbol must share one Calculate call per
// tick (PR-101 CR) — N users watching the same symbol previously triggered
// N redundant evaluation-store reads.
func TestScheduler_Watchlist_SharesRegimeCallAcrossUsers(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))
	_ = cfgStore.Save(watchlistCfg("u2", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{
		"u1": {"BTCUSDT"},
		"u2": {"BTCUSDT"},
	}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateTrend, "up"),
	}}
	states := newMemWatchlistStateStore()

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.CheckWatchlistTransitions(context.Background())

	if got := regimes.calls["BTCUSDT"]; got != 1 {
		t.Fatalf("expected exactly 1 shared Calculate call for BTCUSDT across 2 users, got %d", got)
	}
}

// A failed SendToUser must not advance the persisted state to the new
// dominant regime — otherwise the next tick would compare against the
// post-transition state and never detect (or retry) the same transition
// again, permanently losing it to one transient send failure (PR-101 CR).
func TestScheduler_Watchlist_SendFailure_RetriesNextTick(t *testing.T) {
	sender := &failThenSucceedSender{failUserSends: 1}
	eng := notifications.NewEngine(sender, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	sched := notifications.NewScheduler(eng, nil, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.SetWatchlistProvider(watchlists)
	sched.SetRegimeStackProvider(regimes)
	sched.SetWatchlistStateStore(states)

	sched.CheckWatchlistTransitions(context.Background()) // seeds compression

	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background()) // send fails (failUserSends: 1)

	if sender.userSendCalls != 1 {
		t.Fatalf("expected exactly 1 send attempt on the failing tick, got %d", sender.userSendCalls)
	}
	state, found, _ := states.GetState(context.Background(), "u1", "BTCUSDT", "1h")
	if !found || state.State != mkt.StateCompression {
		t.Fatalf("expected state to still read compression after the failed send (transition not consumed), got %+v (found=%v)", state, found)
	}

	// Regime hasn't moved on — the identical transition must be retried
	// and this time succeed.
	sched.CheckWatchlistTransitions(context.Background())
	if sender.userSendCalls != 2 {
		t.Fatalf("expected a retried send attempt, got %d total calls", sender.userSendCalls)
	}
	state, found, _ = states.GetState(context.Background(), "u1", "BTCUSDT", "1h")
	if !found || state.State != mkt.StateExpansion {
		t.Fatalf("expected state to advance to expansion once the retry succeeded, got %+v (found=%v)", state, found)
	}
}
