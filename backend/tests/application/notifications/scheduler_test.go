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
	summary mkt.Summary
	err     error
}

func (f *fakeMarketProvider) Calculate(_ string) (mkt.Summary, error) {
	return f.summary, f.err
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
	market := &fakeMarketProvider{
		summary: mkt.Summary{
			Timeframe:   "1h",
			State:       mkt.StateTrend,
			Confidence:  0.82,
			SymbolCount: 100,
		},
	}
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
	market := &fakeMarketProvider{
		summary: mkt.Summary{
			State:      mkt.StateSideways,
			Confidence: 0.50,
		},
	}
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
	market := &fakeMarketProvider{
		summary: mkt.Summary{State: mkt.StateTrend, Confidence: 0.9, Timeframe: "1h"},
	}
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
			Symbol:    "BTCUSDT",
			BestSetup: setup.CompressionBreakout,
			Score:     0.85,
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
