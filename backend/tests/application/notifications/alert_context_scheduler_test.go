package notifications_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"pano_chart/backend/application/mtf"
	"pano_chart/backend/application/notifications"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/setup"
)

type fakeEvalStore struct {
	snaps map[string]domain.EvaluationSnapshot // key: tf|symbol
	at    time.Time
	err   error
	gets  int
}

func (f *fakeEvalStore) Put(context.Context, string, []domain.EvaluationSnapshot, time.Time) error {
	return nil
}

func (f *fakeEvalStore) Get(context.Context, string) ([]domain.EvaluationSnapshot, time.Time, error) {
	return nil, time.Time{}, ports.ErrEvaluationNotFound
}

func (f *fakeEvalStore) GetSymbol(_ context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error) {
	f.gets++
	if f.err != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, f.err
	}
	snap, ok := f.snaps[tf+"|"+symbol]
	if !ok {
		return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
	}
	at := f.at
	if at.IsZero() {
		at = time.Now()
	}
	return snap, at, nil
}

func freshSnap(symbol, tf string, score float64, rs *float64, spark []float64) domain.EvaluationSnapshot {
	s := score
	return domain.EvaluationSnapshot{
		Symbol:           symbol,
		Timeframe:        tf,
		AlgoVersion:      domain.AlgoVersion,
		TotalScore:       &s,
		RelativeStrength: rs,
		Sparkline:        spark,
	}
}

func TestScheduler_Watchlist_AttachesContextSparkline(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))

	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	stack := oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral")
	stack.Alignment = 0.75
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{"BTCUSDT": stack}}
	states := newMemWatchlistStateStore()

	spark := make([]float64, 110)
	for i := range spark {
		spark[i] = float64(100 + i)
	}
	rs := 0.032
	evals := &fakeEvalStore{
		at: now,
		snaps: map[string]domain.EvaluationSnapshot{
			"1h|BTCUSDT": freshSnap("BTCUSDT", "1h", 0.81, &rs, spark),
		},
	}
	market := singleMarket("1h", mkt.Summary{
		Timeframe:  "1h",
		State:      mkt.StateTrend,
		Bias:       "up",
		Confidence: 0.62,
	})

	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time { return now })
	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.SetWatchlistProvider(watchlists)
	sched.SetRegimeStackProvider(regimes)
	sched.SetWatchlistStateStore(states)
	sched.SetEvaluationStore(evals)

	sched.CheckWatchlistTransitions(context.Background())

	exp := oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	exp.Alignment = 0.75
	regimes.stacks["BTCUSDT"] = exp
	sched.CheckWatchlistTransitions(context.Background())

	raw := spy.lastUserSend().n.Data["context"]
	if raw == "" {
		t.Fatal("expected context on watchlist payload")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	sparkline, ok := parsed["sparkline"].([]any)
	if !ok || len(sparkline) != 30 {
		t.Fatalf("sparkline len=%v want 30", parsed["sparkline"])
	}
	if parsed["symbolScore"].(float64) != 0.81 {
		t.Fatalf("symbolScore=%v", parsed["symbolScore"])
	}
	if parsed["rs"].(float64) != 0.032 {
		t.Fatalf("rs=%v", parsed["rs"])
	}
	if parsed["alignment"].(float64) != 0.75 {
		t.Fatalf("alignment=%v", parsed["alignment"])
	}
	if parsed["tapeRegime"] != "trend" || parsed["tapeBias"] != "up" {
		t.Fatalf("tape=%v", parsed)
	}
}

func TestScheduler_Watchlist_MissingSnapshotStillSends(t *testing.T) {
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
	sched.SetEvaluationStore(&fakeEvalStore{at: now, snaps: map[string]domain.EvaluationSnapshot{}})
	sched.CheckWatchlistTransitions(context.Background())

	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected send despite missing snapshot, got %d", spy.userCount())
	}
	raw := spy.lastUserSend().n.Data["context"]
	if raw == "" {
		t.Fatal("context should still be attached when alignment is known")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatal("sparkline should be omitted when snapshot missing")
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("symbolScore should be omitted when snapshot missing")
	}
}

func TestScheduler_Watchlist_AbsentTotalScoreOmitsSymbolScore(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))
	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	stack := oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral")
	stack.Alignment = 0.4
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{"BTCUSDT": stack}}
	states := newMemWatchlistStateStore()

	spark := make([]float64, 30)
	for i := range spark {
		spark[i] = float64(10 + i)
	}
	// Fresh + matching algo, but TotalScore unset (pre-deploy Redis row).
	snap := freshSnap("BTCUSDT", "1h", 0.81, nil, spark)
	snap.TotalScore = nil
	evals := &fakeEvalStore{
		at:    now,
		snaps: map[string]domain.EvaluationSnapshot{"1h|BTCUSDT": snap},
	}

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.SetEvaluationStore(evals)
	sched.CheckWatchlistTransitions(context.Background())

	exp := oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	exp.Alignment = 0.4
	regimes.stacks["BTCUSDT"] = exp
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected send, got %d", spy.userCount())
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(spy.lastUserSend().n.Data["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("absent TotalScore must not publish symbolScore:0")
	}
	sparkline, ok := parsed["sparkline"].([]any)
	if !ok || len(sparkline) != 30 {
		t.Fatalf("sparkline still expected when present, got %v", parsed["sparkline"])
	}
}

func TestScheduler_Watchlist_StaleSnapshotOmitsSymbolFields(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))
	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	stack := oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral")
	stack.Alignment = 0.5
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{"BTCUSDT": stack}}
	states := newMemWatchlistStateStore()

	spark := make([]float64, 30)
	for i := range spark {
		spark[i] = float64(i)
	}
	evals := &fakeEvalStore{
		at: now.Add(-2 * time.Hour), // stale for 1h (stale-after = 30m)
		snaps: map[string]domain.EvaluationSnapshot{
			"1h|BTCUSDT": freshSnap("BTCUSDT", "1h", 0.9, nil, spark),
		},
	}

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.SetEvaluationStore(evals)
	sched.CheckWatchlistTransitions(context.Background())

	exp := oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	exp.Alignment = 0.5
	regimes.stacks["BTCUSDT"] = exp
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected send despite stale snap, got %d", spy.userCount())
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(spy.lastUserSend().n.Data["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatal("stale snap must omit sparkline")
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("stale snap must omit symbolScore")
	}
	if parsed["alignment"].(float64) != 0.5 {
		t.Fatalf("alignment still expected from stack, got %v", parsed["alignment"])
	}
}

func TestScheduler_Watchlist_WrongAlgoOmitsSymbolFields(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(watchlistCfg("u1", "1h"))
	watchlists := &fakeWatchlistProvider{symbols: map[string][]string{"u1": {"BTCUSDT"}}}
	regimes := &fakeRegimeStackProvider{stacks: map[string]mtf.Stack{
		"BTCUSDT": oneFrameStack("BTCUSDT", "1h", mkt.StateCompression, "neutral"),
	}}
	states := newMemWatchlistStateStore()

	snap := freshSnap("BTCUSDT", "1h", 0.9, nil, []float64{1, 2, 3})
	snap.AlgoVersion = "v0.0.0-old"
	evals := &fakeEvalStore{
		at:    now,
		snaps: map[string]domain.EvaluationSnapshot{"1h|BTCUSDT": snap},
	}

	sched := newWatchlistScheduler(spy, now, watchlists, regimes, states, cfgStore)
	sched.SetEvaluationStore(evals)
	sched.CheckWatchlistTransitions(context.Background())
	regimes.stacks["BTCUSDT"] = oneFrameStack("BTCUSDT", "1h", mkt.StateExpansion, "up")
	sched.CheckWatchlistTransitions(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected send despite wrong algo, got %d", spy.userCount())
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(spy.lastUserSend().n.Data["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatal("wrong-algo snap must omit sparkline")
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("wrong-algo snap must omit symbolScore")
	}
}

func TestScheduler_Setup_AttachesContextSparkline(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time { return now })

	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol:     "BTCUSDT",
			BestSetup:  setup.CompressionBreakout,
			Score:      0.85,
			Confidence: 0.7,
		},
	}
	market := singleMarket("1h", mkt.Summary{
		Timeframe:  "1h",
		State:      mkt.StateCompression,
		Bias:       "neutral",
		Confidence: 0.55,
	})
	spark := make([]float64, 110)
	for i := range spark {
		spark[i] = float64(200 + i)
	}
	evals := &fakeEvalStore{
		at: now,
		snaps: map[string]domain.EvaluationSnapshot{
			"1h|BTCUSDT": freshSnap("BTCUSDT", "1h", 0.85, nil, spark),
		},
	}
	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:         "u1",
		SetupOfDay:     true,
		SetupMinScore:  0.75,
		SetupTimeframe: "1h",
	})

	sched := notifications.NewScheduler(eng, market, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.SetEvaluationStore(evals)
	sched.CheckSetupOfDay(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 setup send, got %d", spy.userCount())
	}
	raw := spy.lastUserSend().n.Data["context"]
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	sparkline, ok := parsed["sparkline"].([]any)
	if !ok || len(sparkline) != 30 {
		t.Fatalf("sparkline=%v", parsed["sparkline"])
	}
	if parsed["symbolScore"].(float64) != 0.85 {
		t.Fatalf("symbolScore=%v", parsed["symbolScore"])
	}
	if parsed["tapeRegime"] != "compression" {
		t.Fatalf("tapeRegime=%v", parsed["tapeRegime"])
	}
	if spy.lastUserSend().n.Data["timeframe"] != "1h" {
		t.Fatalf("timeframe=%v", spy.lastUserSend().n.Data["timeframe"])
	}
}

func TestScheduler_Setup_CachesContextPerSymbol(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time { return now })

	setups := &fakeSetupProvider{
		scores: setup.SetupScores{
			Symbol: "BTCUSDT", Score: 0.85, Confidence: 0.7,
			BestSetup: setup.CompressionBreakout,
		},
	}
	evals := &fakeEvalStore{
		at: now,
		snaps: map[string]domain.EvaluationSnapshot{
			"1h|BTCUSDT": freshSnap("BTCUSDT", "1h", 0.85, nil, []float64{1, 2, 3}),
		},
	}
	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID: "u1", SetupOfDay: true, SetupMinScore: 0.75, SetupTimeframe: "1h",
	})
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID: "u2", SetupOfDay: true, SetupMinScore: 0.75, SetupTimeframe: "1h",
	})

	sched := notifications.NewScheduler(eng, nil, setups, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.SetEvaluationStore(evals)
	sched.CheckSetupOfDay(context.Background())

	if spy.userCount() != 2 {
		t.Fatalf("expected 2 sends, got %d", spy.userCount())
	}
	if evals.gets != 1 {
		t.Fatalf("expected 1 GetSymbol for two subscribers, got %d", evals.gets)
	}
}

func TestScheduler_Market_AttachesTapeOnly(t *testing.T) {
	spy := &spySender{}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	eng.SetClock(func() time.Time { return now })

	market := singleMarket("1h", mkt.Summary{
		Timeframe:  "1h",
		Breadth:    mkt.Breadth{Trend: 0.82, Sideways: 0.10},
		Bias:       "up",
		State:      mkt.StateTrend,
		Confidence: 0.7,
	})
	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		UptrendMinDominance: 0.75,
		UptrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetClock(func() time.Time { return now })
	sched.SetConfigStore(cfgStore)
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected 1 market send, got %d", spy.userCount())
	}
	raw := spy.lastUserSend().n.Data["context"]
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tapeRegime"] != "trend" || parsed["tapeBias"] != "up" {
		t.Fatalf("tape=%v", parsed)
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatal("market must not carry sparkline")
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("market must not carry symbolScore")
	}
}
