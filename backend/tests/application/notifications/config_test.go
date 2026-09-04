package notifications_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"pano_chart/backend/application/notifications"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/setup"
	infranotify "pano_chart/backend/infrastructure/notifications"

	_ "modernc.org/sqlite"
)

// ── SQLiteConfigStore tests ─────────────────────────────────────────────────

func openTestConfigStore(t *testing.T) *infranotify.SQLiteConfigStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store, err := infranotify.NewSQLiteConfigStore(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store
}

func TestConfigStore_GetReturnsDefaultsForUnknownUser(t *testing.T) {
	store := openTestConfigStore(t)
	cfg, err := store.Get("user-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UserID != "user-new" {
		t.Fatalf("expected user-new, got %s", cfg.UserID)
	}
	if !cfg.Uptrend || !cfg.Downtrend || !cfg.Sideways {
		t.Fatal("expected all market toggles enabled by default")
	}
	if cfg.UptrendMinDominance != 0.35 {
		t.Fatalf("expected 0.35 default, got %f", cfg.UptrendMinDominance)
	}
	if cfg.SetupMinScore != 0.75 {
		t.Fatalf("expected 0.75 setup score, got %f", cfg.SetupMinScore)
	}
}

func TestConfigStore_SaveAndGet(t *testing.T) {
	store := openTestConfigStore(t)
	cfg := notifications.NotificationConfig{
		UserID:                "u1",
		Social:                false,
		MacroHigh:             true,
		MacroModerate:         false,
		News:                  false,
		Uptrend:               true,
		Downtrend:             false,
		Sideways:              true,
		SetupOfDay:            true,
		UptrendMinDominance:   0.80,
		DowntrendMinDominance: 0.60,
		SidewaysMinDominance:  0.70,
		SetupMinScore:         0.85,
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save error: %v", err)
	}
	got, err := store.Get("u1")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got.Social != false || got.MacroHigh != true || got.MacroModerate != false || got.News != false {
		t.Fatal("toggle mismatch")
	}
	if got.UptrendMinDominance != 0.80 {
		t.Fatalf("expected 0.80, got %f", got.UptrendMinDominance)
	}
	if got.Downtrend != false {
		t.Fatal("expected downtrend disabled")
	}
	if got.SetupMinScore != 0.85 {
		t.Fatalf("expected 0.85 setup, got %f", got.SetupMinScore)
	}
}

func TestConfigStore_SaveUpserts(t *testing.T) {
	store := openTestConfigStore(t)
	cfg := notifications.DefaultNotificationConfig("u1")
	_ = store.Save(cfg)
	cfg.UptrendMinDominance = 0.90
	_ = store.Save(cfg)
	got, _ := store.Get("u1")
	if got.UptrendMinDominance != 0.90 {
		t.Fatalf("expected 0.90 after upsert, got %f", got.UptrendMinDominance)
	}
}

func TestConfigStore_All(t *testing.T) {
	store := openTestConfigStore(t)
	_ = store.Save(notifications.DefaultNotificationConfig("u1"))
	_ = store.Save(notifications.DefaultNotificationConfig("u2"))
	all, err := store.All()
	if err != nil {
		t.Fatalf("all error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(all))
	}
}

func TestConfigStore_AllEmptyWhenNoConfigs(t *testing.T) {
	store := openTestConfigStore(t)
	all, err := store.All()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 configs, got %d", len(all))
	}
}

// ── Per-user market scheduler tests ─────────────────────────────────────────

type memConfigStore struct {
	configs map[string]notifications.NotificationConfig
}

func newMemConfigStore() *memConfigStore {
	return &memConfigStore{configs: make(map[string]notifications.NotificationConfig)}
}

func (m *memConfigStore) Get(userID string) (notifications.NotificationConfig, error) {
	if c, ok := m.configs[userID]; ok {
		return c, nil
	}
	return notifications.DefaultNotificationConfig(userID), nil
}

func (m *memConfigStore) Save(cfg notifications.NotificationConfig) error {
	m.configs[cfg.UserID] = cfg
	return nil
}

func (m *memConfigStore) All() ([]notifications.NotificationConfig, error) {
	var out []notifications.NotificationConfig
	for _, c := range m.configs {
		out = append(out, c)
	}
	return out, nil
}

// TestCheckMarketForUser_RegimeChangeSameDay_SendsSecondNotification is the
// regression test for PR-075: the per-user market dedup key used to be
// market_<timeframe>_<dateKey> — no regime component — so a genuine
// intraday regime flip (e.g. Uptrend to Downtrend) produced no further
// notification until the next calendar day. The key now includes the
// winning candidate's label (Uptrend/Downtrend/Sideways/Silent), so a
// label change re-arms the notification even on the same day, while a
// steady regime still only fires once (Engine's 24h per-key dedup).
func TestCheckMarketForUser_RegimeChangeSameDay_SendsSecondNotification(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Timeframe: "1h",
		Breadth:   mkt.Breadth{Trend: 0.82, Sideways: 0.10},
		Bias:      "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:                "u1",
		Uptrend:               true,
		Downtrend:             true,
		UptrendMinDominance:   0.35,
		DowntrendMinDominance: 0.35,
		UptrendTimeframe:      "1h",
		DowntrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })

	sched.CheckMarketState(context.Background())
	if spy.userCount() != 1 {
		t.Fatalf("expected 1 notification after the initial uptrend check, got %d", spy.userCount())
	}

	// Re-checking with no change must stay deduped (same day, same label).
	sched.CheckMarketState(context.Background())
	if spy.userCount() != 1 {
		t.Fatalf("expected the steady regime to stay deduped, got %d", spy.userCount())
	}

	// Regime flips intraday: uptrend -> downtrend, same calendar day.
	market.summaries["1h"] = mkt.Summary{
		Timeframe: "1h",
		Breadth:   mkt.Breadth{Trend: 0.82, Sideways: 0.10},
		Bias:      "down",
	}

	sched.CheckMarketState(context.Background())
	if spy.userCount() != 2 {
		t.Fatalf("expected a second notification after the same-day regime change, got %d", spy.userCount())
	}
}

func TestScheduler_PerUser_MarketUptrend(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.82, Sideways: 0.10},
		Bias:    "up",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		Downtrend:           false,
		Sideways:            false,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
		SetupOfDay:          true,
		SetupMinScore:       0.75,
		SetupTimeframe:      "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 per-user notification, got %d", spy.userCount())
	}
	rec := spy.lastUserSend()
	if rec.userID != "u1" {
		t.Fatalf("expected user u1, got %s", rec.userID)
	}
	if rec.n.Type != notifications.TypeMarket {
		t.Fatalf("expected TypeMarket, got %s", rec.n.Type)
	}
	if rec.n.Body != "Market is Uptrend (82%, 1h)" {
		t.Fatalf("unexpected body: %s", rec.n.Body)
	}
}

func TestScheduler_PerUser_MarketDowntrend_FiresOnBearishRegime(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.82, Sideways: 0.10},
		Bias:    "down",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:                "u1",
		Downtrend:             true,
		DowntrendMinDominance: 0.75,
		DowntrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 per-user downtrend notification, got %d", spy.userCount())
	}
	rec := spy.lastUserSend()
	if rec.userID != "u1" {
		t.Fatalf("expected user u1, got %s", rec.userID)
	}
	if rec.n.Body != "Market is Downtrend (82%, 1h)" {
		t.Fatalf("unexpected body: %s", rec.n.Body)
	}
}

func TestScheduler_PerUser_MarketDowntrend_DoesNotFireOnExpansion(t *testing.T) {
	// Regression test for the PR-072 bug: Downtrend used to read
	// Scores.Expansion (breakout activity) with no direction check at
	// all — a high-Expansion, bullish (or even neutral) regime must NOT
	// trigger a "Downtrend" alert.
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Expansion: 0.90, Trend: 0.10},
		Bias:    "up", // strong breakout, but to the upside
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:                "u1",
		Downtrend:             true,
		DowntrendMinDominance: 0.75,
		DowntrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no downtrend notification for a bullish expansion regime, got %d", spy.userCount())
	}
}

func TestScheduler_PerUser_MarketUptrend_DoesNotFireOnBearishRegime(t *testing.T) {
	// Symmetric regression test: a strong trend score with a bearish bias
	// must not satisfy an Uptrend subscription.
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.90, Sideways: 0.05},
		Bias:    "down",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no uptrend notification during a confirmed decline, got %d", spy.userCount())
	}
}

func TestScheduler_PerUser_NeutralBias_NeitherUptrendNorDowntrendFires(t *testing.T) {
	// Regression test: a regime with no real direction (Bias == "neutral")
	// must satisfy neither an Uptrend nor a Downtrend subscription, even
	// with both configured and their dominance thresholds at 0.
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.90, Sideways: 0.05},
		Bias:    "neutral",
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:                "u1",
		Uptrend:               true,
		UptrendMinDominance:   0,
		UptrendTimeframe:      "1h",
		Downtrend:             true,
		DowntrendMinDominance: 0,
		DowntrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatalf("expected no notification for a neutral-bias regime, got %d", spy.userCount())
	}
}

func TestScheduler_PerUser_BelowThreshold_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.60, Sideways: 0.30},
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:               "u1",
		Uptrend:              true,
		Sideways:             true,
		UptrendMinDominance:  0.75,
		SidewaysMinDominance: 0.75,
		UptrendTimeframe:     "1h",
		SidewaysTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatal("expected no notification when below all thresholds")
	}
}

func TestScheduler_PerUser_DisabledRegime_Suppressed(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.85},
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             false,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 0 {
		t.Fatal("expected no notification when regime is disabled")
	}
}

func TestScheduler_PerUser_StrongestRegimeWins(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Breadth: mkt.Breadth{Trend: 0.40, Sideways: 0.45, Compression: 0.10},
	})

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:               "u1",
		Uptrend:              true,
		Sideways:             true,
		UptrendMinDominance:  0.35,
		SidewaysMinDominance: 0.35,
		UptrendTimeframe:     "1h",
		SidewaysTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 notification, got %d", spy.userCount())
	}
	body := spy.lastUserSend().n.Body
	if body != "Market is Sideways (45%, 1h)" {
		t.Fatalf("expected sideways to win, got: %s", body)
	}
}

func TestScheduler_PerUser_SetupWithCustomThreshold(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "BTCUSDT",
			BestSetup:  setup.CompressionBreakout,
			Score:      0.80,
			Confidence: 0.7,
		},
	}

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:         "u1",
		SetupOfDay:     true,
		SetupMinScore:  0.85,
		SetupTimeframe: "1h",
	})
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:         "u2",
		SetupOfDay:     true,
		SetupMinScore:  0.75,
		SetupTimeframe: "1h",
	})

	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 per-user setup notification, got %d", spy.userCount())
	}
	if spy.lastUserSend().userID != "u2" {
		t.Fatalf("expected u2 to be notified, got %s", spy.lastUserSend().userID)
	}
}

// ── Engine SendToUser tests ─────────────────────────────────────────────────

func TestEngine_SendToUser_PerUserDedup(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMarket, Title: "T", Body: "B",
		Key: "market_1h_2025-06-01",
	}

	_ = eng.SendToUser(context.Background(), "u1", n)
	_ = eng.SendToUser(context.Background(), "u2", n)
	if spy.userCount() != 2 {
		t.Fatalf("expected 2 (different users), got %d", spy.userCount())
	}

	_ = eng.SendToUser(context.Background(), "u1", n)
	if spy.userCount() != 2 {
		t.Fatalf("expected still 2 (deduped), got %d", spy.userCount())
	}
}

func TestEngine_SendToUser_QuietHours(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 3, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMarket, Title: "T", Body: "B", Key: "k",
	}
	_ = eng.SendToUser(context.Background(), "u1", n)
	if spy.userCount() != 0 {
		t.Fatal("expected suppressed during quiet hours")
	}
}

// ── Multi-timeframe tests ───────────────────────────────────────────────────

func TestScheduler_PerUser_DifferentTimeframesPerRegime(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	// 15m shows uptrend at 80%, 1h shows uptrend at 50%.
	market := &fakeMarketProvider{
		summaries: map[string]mkt.Summary{
			"15m": {Timeframe: "15m", Breadth: mkt.Breadth{Trend: 0.80, Sideways: 0.10, Compression: 0.05}, Bias: "up"},
			"1h":  {Timeframe: "1h", Breadth: mkt.Breadth{Trend: 0.50, Sideways: 0.30, Compression: 0.10}, Bias: "up"},
		},
	}

	cfgStore := newMemConfigStore()
	// User wants uptrend on 15m — should trigger.
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "15m",
	})
	// User wants uptrend on 1h — should NOT trigger (60% < 75%).
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u2",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 notification (u1 only), got %d", spy.userCount())
	}
	rec := spy.lastUserSend()
	if rec.userID != "u1" {
		t.Fatalf("expected user u1, got %s", rec.userID)
	}
	if rec.n.Body != "Market is Uptrend (80%, 15m)" {
		t.Fatalf("unexpected body: %s", rec.n.Body)
	}
}

func TestScheduler_PerUser_SetupDifferentTimeframes(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "ETHUSDT",
			BestSetup:  setup.CompressionBreakout,
			Score:      0.85,
			Confidence: 0.7,
		},
	}

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:         "u1",
		SetupOfDay:     true,
		SetupMinScore:  0.80,
		SetupTimeframe: "15m",
	})

	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckSetupOfDay(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 setup notification, got %d", spy.userCount())
	}
	body := spy.lastUserSend().n.Body
	if body != "ETHUSDT (85%, 15m)" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestConfigStore_TimeframeRoundtrip(t *testing.T) {
	store := openTestConfigStore(t)
	cfg := notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		UptrendTimeframe:    "15m",
		DowntrendTimeframe:  "4h",
		SidewaysTimeframe:   "1d",
		SetupTimeframe:      "5m",
		UptrendMinDominance: 0.80,
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save error: %v", err)
	}
	got, err := store.Get("u1")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got.UptrendTimeframe != "15m" {
		t.Fatalf("expected 15m, got %s", got.UptrendTimeframe)
	}
	if got.DowntrendTimeframe != "4h" {
		t.Fatalf("expected 4h, got %s", got.DowntrendTimeframe)
	}
	if got.SidewaysTimeframe != "1d" {
		t.Fatalf("expected 1d, got %s", got.SidewaysTimeframe)
	}
	if got.SetupTimeframe != "5m" {
		t.Fatalf("expected 5m, got %s", got.SetupTimeframe)
	}
}

func TestConfigStore_ResetsMinDominanceFieldsForPreMigrationConfigs(t *testing.T) {
	// Regression test covering two successive semantic changes to the
	// *MinDominance fields:
	//   - PR-072: DowntrendMinDominance moved from thresholding
	//     Scores.Expansion to Scores.Trend.
	//   - PR-073: all three fields moved from thresholding the softmax
	//     pipeline's output to MarketStateService's proportional Breadth,
	//     a structurally flatter scale (see DefaultNotificationConfig's doc).
	// A config stored before either change (config_version 0) must not
	// silently reuse thresholds tuned for a metric that no longer exists —
	// Get should reset all three to the current defaults.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := infranotify.NewSQLiteConfigStore(db)
	if err != nil {
		t.Fatal(err)
	}

	const legacyJSON = `{"downtrend":true,"downtrend_min_dominance":0.40,"uptrend_min_dominance":0.60,"sideways_min_dominance":0.60}`
	_, err = db.Exec(`INSERT INTO notification_config (user_id, config, updated_at)
		VALUES (?, ?, datetime('now'))`, "u-pre-migration", legacyJSON)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	got, err := store.Get("u-pre-migration")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	const wantDefault = 0.35
	if got.DowntrendMinDominance != wantDefault {
		t.Fatalf("expected DowntrendMinDominance reset to default %v, got %f", wantDefault, got.DowntrendMinDominance)
	}
	if got.UptrendMinDominance != wantDefault {
		t.Fatalf("expected UptrendMinDominance reset to default %v, got %f", wantDefault, got.UptrendMinDominance)
	}
	if got.SidewaysMinDominance != wantDefault {
		t.Fatalf("expected SidewaysMinDominance reset to default %v, got %f", wantDefault, got.SidewaysMinDominance)
	}
	// Genuinely unrelated fields must survive untouched.
	if !got.Downtrend {
		t.Fatal("expected Downtrend toggle to remain true")
	}
}

func TestConfigStore_BackfillsEmptyTimeframes(t *testing.T) {
	store := openTestConfigStore(t)
	// Simulate a legacy config without timeframe fields (stored as empty strings).
	cfg := notifications.NotificationConfig{
		UserID:  "u-legacy",
		Uptrend: true,
		// All timeframe fields are zero-value ("").
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save error: %v", err)
	}
	got, err := store.Get("u-legacy")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	// Should backfill to "1h".
	if got.UptrendTimeframe != "1h" {
		t.Fatalf("expected 1h backfill, got %s", got.UptrendTimeframe)
	}
	if got.SetupTimeframe != "1h" {
		t.Fatalf("expected 1h backfill, got %s", got.SetupTimeframe)
	}
}
