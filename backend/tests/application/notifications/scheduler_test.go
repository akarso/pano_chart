package notifications_test

import (
	"context"
	"testing"
	"time"

	"pano_chart/backend/application/notifications"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/setup"
)

type fakeMarketProvider struct {
	summaries map[string]mkt.RegimeSummary // keyed by timeframe
	err       error
}

func (f *fakeMarketProvider) CalculateRegime(_ context.Context, tf string) (mkt.RegimeSummary, error) {
	if f.err != nil {
		return mkt.RegimeSummary{}, f.err
	}
	if s, ok := f.summaries[tf]; ok {
		return s, nil
	}
	// Fallback: return first entry (backwards compat for single-tf tests).
	for _, s := range f.summaries {
		return s, nil
	}
	return mkt.RegimeSummary{}, nil
}

// singleMarket is a helper that builds a fakeMarketProvider for one timeframe.
func singleMarket(tf string, s mkt.RegimeSummary) *fakeMarketProvider {
	s.Timeframe = tf
	return &fakeMarketProvider{summaries: map[string]mkt.RegimeSummary{tf: s}}
}

type fakeSetupProvider struct {
	scores setup.SetupScores
	err    error
}

func (f *fakeSetupProvider) BestSetup(_ context.Context, _ string) (setup.SetupScores, error) {
	return f.scores, f.err
}

type fakeEventProvider struct {
	events []domain.Event
	err    error
}

func (f *fakeEventProvider) FetchEvents(_ context.Context, _, _ time.Time) ([]domain.Event, error) {
	return f.events, f.err
}

func TestScheduler_MarketState_HighConfidence(t *testing.T) {
	spy := &spySender{}
	cfg := notifications.DefaultEngineConfig()
	eng := notifications.NewEngine(spy, cfg)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	market := singleMarket("1h", mkt.RegimeSummary{
		Regime:     mkt.RegimeTrend,
		Prevalence: 0.82,
	})
	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())
	if spy.count() != 1 {
		t.Fatalf("expected 1 market notification, got %d", spy.count())
	}
	if spy.last().Type != notifications.TypeMarket {
		t.Fatalf("expected TypeMarket, got %s", spy.last().Type)
	}
	if spy.last().Body != "Market trending today" {
		t.Fatalf("unexpected body: %s", spy.last().Body)
	}
}

func TestScheduler_MarketState_LowConfidence_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	market := singleMarket("1h", mkt.RegimeSummary{
		Regime:     mkt.RegimeSideways,
		Prevalence: 0.50,
	})
	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notification for low-confidence market state")
	}
}

func TestScheduler_MarketState_OncePerDay(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	market := singleMarket("1h", mkt.RegimeSummary{
		Regime: mkt.RegimeTrend, Prevalence: 0.9,
	})
	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())
	sched.CheckMarketState(context.Background())
	if spy.count() != 1 {
		t.Fatalf("expected exactly 1 (once-per-day), got %d", spy.count())
	}
}

func TestScheduler_SetupOfDay_HighScore(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "BTCUSDT",
			BestSetup:  setup.CompressionBreakout,
			Score:      0.85,
			Confidence: 0.7,
		},
	}
	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 1 {
		t.Fatalf("expected 1 setup notification, got %d", spy.count())
	}
	if spy.last().Type != notifications.TypeSetup {
		t.Fatalf("expected TypeSetup, got %s", spy.last().Type)
	}
}

func TestScheduler_SetupOfDay_LowScore_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	setups := &fakeSetupProvider{
		scores: setup.SetupScores{Symbol: "ETHUSDT", Score: 0.50},
	}
	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notification for low-score setup")
	}
}

func TestScheduler_MacroEvents_Upcoming(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	ev, _ := domain.NewEvent(
		"fomc_1", "US", "FOMC Rate Decision",
		domain.EventImpactHigh, now.Add(28*time.Minute),
	)
	events := &fakeEventProvider{events: []domain.Event{ev}}
	sched := notifications.NewScheduler(eng, nil, nil, events, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMacroEvents(context.Background())
	if spy.count() != 1 {
		t.Fatalf("expected 1 macro notification, got %d", spy.count())
	}
	if spy.last().Type != notifications.TypeMacro {
		t.Fatalf("expected TypeMacro, got %s", spy.last().Type)
	}
}

func TestScheduler_MacroEvents_TooFarAway_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	ev, _ := domain.NewEvent(
		"cpi_1", "US", "CPI Release",
		domain.EventImpactHigh, now.Add(3*time.Hour),
	)
	events := &fakeEventProvider{events: []domain.Event{ev}}
	sched := notifications.NewScheduler(eng, nil, nil, events, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMacroEvents(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notification for far-away event")
	}
}

func TestScheduler_NilProviders_NoPanic(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	sched := notifications.NewScheduler(eng, nil, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckMacroEvents(context.Background())
	sched.CheckMarketState(context.Background())
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notifications with nil providers")
	}
}

// ── Subscription gating tests ───────────────────────────────────────────────

type fakeSubscriptionChecker struct {
	active map[string]bool
	err    error
}

func (f *fakeSubscriptionChecker) IsActive(_ context.Context, userID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.active[userID], nil
}

func TestScheduler_SubscriptionGating_MarketSuppressedForFreeUser(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.RegimeSummary{
		Scores: mkt.RegimeScores{Trend: 0.85},
		Bias:   "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "free-user",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	subs := &fakeSubscriptionChecker{active: map[string]bool{"free-user": false}}

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetSubscriptionChecker(subs)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatal("expected no market notification for free user")
	}
}

func TestScheduler_SubscriptionGating_MarketSentToProUser(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.RegimeSummary{
		Scores: mkt.RegimeScores{Trend: 0.85},
		Bias:   "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "pro-user",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	subs := &fakeSubscriptionChecker{active: map[string]bool{"pro-user": true}}

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetSubscriptionChecker(subs)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 market notification for pro user, got %d", spy.userCount())
	}
}

func TestScheduler_SubscriptionGating_SetupSuppressedForFreeUser(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	setups := &fakeSetupProvider{
		scores: setup.SetupScores{Symbol: "BTCUSDT", Score: 0.85},
	}

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:         "free-user",
		SetupOfDay:     true,
		SetupMinScore:  0.75,
		SetupTimeframe: "1h",
	})

	subs := &fakeSubscriptionChecker{active: map[string]bool{"free-user": false}}

	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetSubscriptionChecker(subs)
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())

	if spy.userCount() != 0 {
		t.Fatal("expected no setup notification for free user")
	}
}

func TestScheduler_SubscriptionGating_MixedUsers(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.RegimeSummary{
		Scores: mkt.RegimeScores{Trend: 0.85},
		Bias:   "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID: "pro", Uptrend: true, UptrendMinDominance: 0.75, UptrendTimeframe: "1h",
	})
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID: "free", Uptrend: true, UptrendMinDominance: 0.75, UptrendTimeframe: "1h",
	})

	subs := &fakeSubscriptionChecker{active: map[string]bool{"pro": true, "free": false}}

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetSubscriptionChecker(subs)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 notification (pro only), got %d", spy.userCount())
	}
	if spy.lastUserSend().userID != "pro" {
		t.Fatalf("expected pro user to be notified, got %s", spy.lastUserSend().userID)
	}
}

func TestScheduler_SubscriptionGating_NilChecker_AllowsAll(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.RegimeSummary{
		Scores: mkt.RegimeScores{Trend: 0.85},
		Bias:   "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID: "u1", Uptrend: true, UptrendMinDominance: 0.75, UptrendTimeframe: "1h",
	})

	// No subscription checker set — backwards-compatible, all users treated as pro.
	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatal("expected notification when no subscription checker is set")
	}
}

func TestScheduler_SetupOfDay_ConfidenceGate_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "BTCUSDT",
			BestSetup:  setup.TrendContinuation,
			Score:      0.85,
			Confidence: 0.5, // below 0.6 gate
		},
	}
	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notification when confidence is below gate")
	}
}

func TestScheduler_SetupOfDay_ConfidenceGate_Passes(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "BTCUSDT",
			BestSetup:  setup.TrendContinuation,
			Score:      0.85,
			Confidence: 0.7, // above 0.6 gate
		},
	}
	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 1 {
		t.Fatalf("expected 1 setup notification for confident setup, got %d", spy.count())
	}
}

func TestScheduler_SetupOfDay_ConfidenceGate_AppliesToAllRegimes(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "ETHUSDT",
			BestSetup:  setup.CompressionBreakout,
			Score:      0.80,
			Confidence: 0.4, // low confidence, even for non-trend
		},
	}
	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())
	if spy.count() != 0 {
		t.Fatal("expected no notification for non-trend setup with low confidence")
	}
}
