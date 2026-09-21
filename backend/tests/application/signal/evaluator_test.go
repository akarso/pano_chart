package signal_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"pano_chart/backend/application/market/metrics"
	appsignal "pano_chart/backend/application/signal"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	domainsignal "pano_chart/backend/domain/signal"
	infrasignal "pano_chart/backend/infrastructure/signal"
)

type memRepo struct {
	sigs     []domainsignal.Signal
	outcomes map[string]domainsignal.Outcome
}

func newMemRepo() *memRepo {
	return &memRepo{outcomes: map[string]domainsignal.Outcome{}}
}

func (m *memRepo) Append(_ context.Context, s domainsignal.Signal) error {
	m.sigs = append(m.sigs, s)
	return nil
}

func (m *memRepo) Unresolved(_ context.Context, before time.Time, limit int) ([]domainsignal.Signal, error) {
	var out []domainsignal.Signal
	for _, s := range m.sigs {
		if _, ok := m.outcomes[s.ID]; ok {
			continue
		}
		if !s.EmittedAt.Before(before) {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memRepo) UnresolvedReady(_ context.Context, now time.Time, limit int) ([]domainsignal.Signal, error) {
	var out []domainsignal.Signal
	for _, s := range m.sigs {
		if _, ok := m.outcomes[s.ID]; ok {
			continue
		}
		end, ok := domainsignal.HorizonEnd(s)
		if !ok || now.Before(end) {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memRepo) UnresolvedInvalidTF(_ context.Context, limit int) ([]domainsignal.Signal, error) {
	var out []domainsignal.Signal
	for _, s := range m.sigs {
		if _, ok := m.outcomes[s.ID]; ok {
			continue
		}
		if _, ok := domainsignal.HorizonEnd(s); ok {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memRepo) MarkResolved(_ context.Context, id string, o domainsignal.Outcome) error {
	if o.SignalID == "" {
		o.SignalID = id
	}
	m.outcomes[id] = o
	return nil
}

func (m *memRepo) Query(context.Context, domainsignal.Filter) ([]domainsignal.SignalWithOutcome, error) {
	return nil, nil
}

type fakeCandles struct {
	series map[string]domain.CandleSeries
	err    error
}

func (f *fakeCandles) GetSeries(
	_ context.Context, sym domain.Symbol, tf domain.Timeframe, from, to time.Time,
) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	key := sym.String() + "|" + tf.String()
	full, ok := f.series[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("missing series %s", key)
	}
	// Return unclipped (inclusive extras) to exercise evaluator clip.
	return full, nil
}

func (f *fakeCandles) GetLastNCandles(
	context.Context, domain.Symbol, domain.Timeframe, int,
) (domain.CandleSeries, error) {
	return domain.CandleSeries{}, fmt.Errorf("not used")
}

func makeSeries(sym string, tf domain.Timeframe, start time.Time, closes []float64) domain.CandleSeries {
	s := domain.NewSymbolUnsafe(sym)
	candles := make([]domain.Candle, len(closes))
	for i, c := range closes {
		candles[i] = domain.NewCandleUnsafe(
			s, tf, start.Add(time.Duration(i)*tf.Duration()),
			c, c+1, c-1, c, 100,
		)
	}
	cs, err := domain.NewCandleSeries(s, tf, candles)
	if err != nil {
		panic(err)
	}
	return cs
}

func TestEvaluator_resolvesTrendUpEndToEnd(t *testing.T) {
	repo := newMemRepo()
	tf := domain.Timeframe1h
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	horizon := 3
	// Bars at 0,1,2,3h — evaluator clips to [0,3h) → 3 bars; inclusive extra at 3h dropped.
	closes := []float64{100, 101, 102, 104}
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"BTCUSDT|1h": makeSeries("BTCUSDT", tf, emitted, closes),
	}}

	sig := domainsignal.Signal{
		ID: "s1", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Score: 0.8, Price: 100, ATR: 2,
		EmittedAt: emitted, HorizonBars: horizon,
	}
	_ = repo.Append(context.Background(), sig)

	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetNow(func() time.Time { return emitted.Add(2 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 0 {
		t.Fatalf("expected 0 before horizon, got %d", n)
	}

	ev.SetNow(func() time.Time { return emitted.Add(3 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 1 {
		t.Fatalf("expected 1 resolved, got %d", n)
	}
	oc := repo.outcomes["s1"]
	if !oc.Success || oc.Rule != domainsignal.RuleTrendUp {
		t.Fatalf("%+v", oc)
	}
	wantFwd := (102.0 - 100.0) / 100.0
	if oc.ForwardReturn != wantFwd {
		t.Fatalf("fwd=%v want %v (extra bar at horizonEnd must be clipped)", oc.ForwardReturn, wantFwd)
	}
}

func TestEvaluator_skipsWhenCandlesMissing(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "s2", Kind: domainsignal.KindBadge, Symbol: "ETHUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 2,
	})
	ev := appsignal.NewEvaluator(repo, &fakeCandles{series: map[string]domain.CandleSeries{}})
	ev.SetNow(func() time.Time { return emitted.Add(3 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 0 {
		t.Fatalf("got %d", n)
	}
	if _, ok := repo.outcomes["s2"]; ok {
		t.Fatal("should stay unresolved")
	}
}

func TestEvaluator_partialCandlesNotResolved(t *testing.T) {
	repo := newMemRepo()
	tf := domain.Timeframe1h
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Only 2 bars for horizon 4.
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"BTCUSDT|1h": makeSeries("BTCUSDT", tf, emitted, []float64{100, 101}),
	}}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "partial", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 4,
	})
	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetNow(func() time.Time { return emitted.Add(5 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 0 {
		t.Fatalf("got %d", n)
	}
	if _, ok := repo.outcomes["partial"]; ok {
		t.Fatal("partial path must stay unresolved")
	}
}

func TestEvaluator_atrZeroLeavesUnresolved(t *testing.T) {
	repo := newMemRepo()
	tf := domain.Timeframe1h
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"BTCUSDT|1h": makeSeries("BTCUSDT", tf, emitted, []float64{100, 101, 102}),
	}}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "atr0", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 100, ATR: 0, EmittedAt: emitted, HorizonBars: 3,
	})
	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetNow(func() time.Time { return emitted.Add(3 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 0 {
		t.Fatalf("ATR=0 must not auto-succeed, got %d", n)
	}
	if _, ok := repo.outcomes["atr0"]; ok {
		t.Fatal("ATR=0 must stay unresolved")
	}
}

func TestEvaluator_starvationNotReadyDoesNotBlockReady(t *testing.T) {
	repo := newMemRepo()
	tf := domain.Timeframe1h
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// 250 not-ready 1d signals (horizon 20 bars = 20 days).
	for i := 0; i < 250; i++ {
		_ = repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("long-%d", i), Kind: domainsignal.KindBadge,
			Symbol: "BTCUSDT", Timeframe: "1d", Label: "trend_up",
			Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		})
	}
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"ETHUSDT|1h": makeSeries("ETHUSDT", tf, emitted, []float64{100, 101, 102}),
	}}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "ready-short", Kind: domainsignal.KindBadge, Symbol: "ETHUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 3,
	})
	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetNow(func() time.Time { return emitted.Add(3 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 1 {
		t.Fatalf("expected ready short-TF to resolve despite oldest long-TF backlog, got %d", n)
	}
	if _, ok := repo.outcomes["ready-short"]; !ok {
		t.Fatal("ready-short missing")
	}
}

func TestEvaluator_invalidTFDrained(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Older valid-TF rows stuck without candles must not hide invalid TF.
	for i := 0; i < 60; i++ {
		_ = repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("stuck-%d", i), Kind: domainsignal.KindBadge,
			Symbol: "BTCUSDT", Timeframe: "1h", Label: "trend_up",
			Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 2,
		})
	}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "bad-tf", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "3h",
		Label: "trend_up", Price: 100, ATR: 1, EmittedAt: emitted, HorizonBars: 5,
	})
	ev := appsignal.NewEvaluator(repo, &fakeCandles{series: map[string]domain.CandleSeries{}})
	ev.SetNow(func() time.Time { return emitted.Add(3 * time.Hour) })
	_ = ev.Tick(context.Background())
	oc, ok := repo.outcomes["bad-tf"]
	if !ok || oc.Rule != domainsignal.RuleInvalid {
		t.Fatalf("want invalid drain behind stuck valids, got %+v ok=%v", oc, ok)
	}
}

func TestEvaluator_sharedBudgetCapsInvalidPlusReady(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < appsignal.MaxPerTick; i++ {
		_ = repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("bad-%d", i), Kind: domainsignal.KindBadge,
			Symbol: "X", Timeframe: "3h", Label: "gain",
			EmittedAt: emitted, HorizonBars: 1,
		})
	}
	// Ready path-independent signal — must still grade despite invalid backlog.
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "gain-ready", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "gain", Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 1,
	})
	ev := appsignal.NewEvaluator(repo, &fakeCandles{})
	ev.SetNow(func() time.Time { return emitted.Add(2 * time.Hour) })
	n := ev.Tick(context.Background())
	if n != appsignal.MaxPerTick {
		t.Fatalf("resolved=%d want %d", n, appsignal.MaxPerTick)
	}
	if _, ok := repo.outcomes["gain-ready"]; !ok {
		t.Fatal("ready reserve must grade horizon-elapsed signals despite invalid backlog")
	}
	// Invalid first pass is capped at MaxPerTick/2; remainder after ready fills invalid.
	invalidN := 0
	for id, oc := range repo.outcomes {
		if strings.HasPrefix(id, "bad-") && oc.Rule == domainsignal.RuleInvalid {
			invalidN++
		}
	}
	if invalidN != appsignal.MaxPerTick-1 {
		t.Fatalf("invalid resolved=%d want %d", invalidN, appsignal.MaxPerTick-1)
	}
}

func TestEvaluator_regimeUnsupported(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tape := &fakeTape{} // must not be required / called
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "reg-exp", Kind: domainsignal.KindRegime, Timeframe: "1h",
		Label: "regime:expansion", Score: 1,
		EmittedAt: emitted, HorizonBars: 2,
	})
	ev := appsignal.NewEvaluator(repo, &fakeCandles{})
	ev.SetTapeProvider(tape)
	ev.SetNow(func() time.Time { return emitted.Add(2 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 1 {
		t.Fatalf("got %d", n)
	}
	if tape.calls != 0 {
		t.Fatalf("unsupported must not load tape, calls=%d", tape.calls)
	}
	oc := repo.outcomes["reg-exp"]
	if oc.Rule != domainsignal.RuleUnsupported || oc.Success {
		t.Fatalf("%+v", oc)
	}
	if !domainsignal.ExcludedFromHitRate(oc.Rule) {
		t.Fatal("unsupported must be excluded from hit-rate")
	}
}

type fakeTape struct {
	tape  metrics.CompositeTape
	err   error
	calls int
}

func (f *fakeTape) CalculateTape(_ context.Context, _ string, _ int) (metrics.CompositeTape, error) {
	f.calls++
	return f.tape, f.err
}

type fakeRegimes struct {
	hist mkt.RegimeHistory
	err  error
}

func (f *fakeRegimes) GetHistory(string, int) (mkt.RegimeHistory, error) {
	return f.hist, f.err
}

func TestEvaluator_marketWideTransition(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	horizon := 2
	tape := &fakeTape{} // transition must not require tape
	regimes := &fakeRegimes{hist: mkt.RegimeHistory{
		Periods: []mkt.RegimePeriod{
			{Regime: mkt.RegimeSideways, StartTimestamp: emitted.Unix(), EndTimestamp: nil},
		},
	}}

	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "t1", Kind: domainsignal.KindTransition, Timeframe: "1h",
		Label: "transition:sideways", Score: 0.6,
		EmittedAt: emitted, HorizonBars: horizon,
	})

	ev := appsignal.NewEvaluator(repo, &fakeCandles{})
	ev.SetTapeProvider(tape)
	ev.SetRegimeHistory(regimes)
	ev.SetNow(func() time.Time { return emitted.Add(2 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 1 {
		t.Fatalf("resolved=%d tapeCalls=%d", n, tape.calls)
	}
	if tape.calls != 0 {
		t.Fatalf("transition must not load tape, calls=%d", tape.calls)
	}
	oc := repo.outcomes["t1"]
	if !oc.Success || oc.Rule != domainsignal.RuleTransition {
		t.Fatalf("%+v", oc)
	}
}

func TestEvaluator_transitionMissingHistoryStaysUnresolved(t *testing.T) {
	repo := newMemRepo()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "t-miss", Kind: domainsignal.KindTransition, Timeframe: "1h",
		Label: "transition:trend", Score: 0.6,
		EmittedAt: emitted, HorizonBars: 2,
	})
	ev := appsignal.NewEvaluator(repo, &fakeCandles{})
	ev.SetRegimeHistory(&fakeRegimes{}) // empty periods
	ev.SetNow(func() time.Time { return emitted.Add(2 * time.Hour) })
	if n := ev.Tick(context.Background()); n != 0 {
		t.Fatalf("got %d", n)
	}
	if _, ok := repo.outcomes["t-miss"]; ok {
		t.Fatal("missing regime must not resolve as false negative")
	}
}

func TestEvaluator_delayedTapeMissPermanent(t *testing.T) {
	repo := newMemRepo()
	tf := domain.Timeframe1h
	// Emission far in the past; tape only has recent bars.
	emitted := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	recentStart := now.Add(-10 * time.Hour)
	series := makeSeries("COMPOSITE", tf, recentStart, []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109})
	tape := &fakeTape{tape: metrics.CompositeTape{MedianSeries: series, PreferredSource: "composite_median"}}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "old-tape", Kind: domainsignal.KindRegime, Timeframe: "1h",
		Label: "regime:trend", Score: 1, Price: 100, ATR: 1,
		Context:   map[string]float64{"bias": 1},
		EmittedAt: emitted, HorizonBars: 2,
	})
	// Also a ready short signal that must still resolve (not blocked).
	shortEmitted := now.Add(-3 * time.Hour)
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"ETHUSDT|1h": makeSeries("ETHUSDT", tf, shortEmitted, []float64{100, 101, 102}),
	}}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "still-ok", Kind: domainsignal.KindBadge, Symbol: "ETHUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 100, ATR: 1, EmittedAt: shortEmitted, HorizonBars: 3,
	})

	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetTapeProvider(tape)
	ev.SetNow(func() time.Time { return now })
	n := ev.Tick(context.Background())
	if n < 1 {
		t.Fatalf("expected at least still-ok resolved, got %d", n)
	}
	if oc, ok := repo.outcomes["old-tape"]; ok {
		if oc.Rule != domainsignal.RulePathUnavailable {
			t.Fatalf("delayed tape: %+v", oc)
		}
	} else {
		t.Fatal("permanent tape miss must drain as path_unavailable")
	}
	if _, ok := repo.outcomes["still-ok"]; !ok {
		t.Fatal("ready symbol signal blocked by tape miss")
	}
}

func TestEvaluator_sqliteRoundTrip(t *testing.T) {
	dbPath := t.TempDir() + "/signals.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	tf := domain.Timeframe15m
	emitted := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 100 + float64(i)*0.1
	}
	candles := &fakeCandles{series: map[string]domain.CandleSeries{
		"BTCUSDT|15m": makeSeries("BTCUSDT", tf, emitted, closes),
	}}
	sig := domainsignal.Signal{
		ID: "sql1", Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "15m",
		Label: "trend_up", Price: 100, ATR: 1,
		EmittedAt: emitted, HorizonBars: 20,
	}
	if err := repo.Append(context.Background(), sig); err != nil {
		t.Fatal(err)
	}

	// Not ready yet — UnresolvedReady empty.
	ready, err := repo.UnresolvedReady(context.Background(), emitted.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 0 {
		t.Fatalf("premature ready: %d", len(ready))
	}

	ev := appsignal.NewEvaluator(repo, candles)
	ev.SetNow(func() time.Time { return emitted.Add(20 * 15 * time.Minute) })
	if n := ev.Tick(context.Background()); n != 1 {
		t.Fatalf("resolved=%d", n)
	}
	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Outcome == nil || !rows[0].Outcome.Success {
		t.Fatalf("%+v", rows)
	}
}

func TestSQLite_UnresolvedReadySkipsNotReady(t *testing.T) {
	dbPath := t.TempDir() + "/sig.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_ = repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("d%d", i), Kind: domainsignal.KindBadge,
			Symbol: "BTCUSDT", Timeframe: "1d", Label: "trend_up",
			Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		})
	}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "h1", Kind: domainsignal.KindBadge, Symbol: "ETHUSDT", Timeframe: "1h",
		Label: "trend_up", Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 2,
	})
	now := emitted.Add(3 * time.Hour)
	ready, err := repo.UnresolvedReady(context.Background(), now, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != "h1" {
		t.Fatalf("ready=%v", ready)
	}
}

func TestSQLite_UnresolvedInvalidTF(t *testing.T) {
	dbPath := t.TempDir() + "/inv.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		_ = repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("ok-%d", i), Kind: domainsignal.KindBadge,
			Symbol: "BTCUSDT", Timeframe: "1h", Label: "trend_up",
			Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		})
	}
	_ = repo.Append(context.Background(), domainsignal.Signal{
		ID: "bad", Kind: domainsignal.KindBadge, Symbol: "X", Timeframe: "3h",
		Label: "gain", EmittedAt: emitted, HorizonBars: 1,
	})
	inv, err := repo.UnresolvedInvalidTF(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) != 1 || inv[0].ID != "bad" {
		t.Fatalf("invalid=%v", inv)
	}
}

func TestSQLite_timeframeNormalization(t *testing.T) {
	dbPath := t.TempDir() + "/tf.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	emitted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Domain-valid but non-canonical spellings must not be treated as invalid.
	for _, tc := range []struct {
		id, tf string
	}{
		{"h1", "1H"},
		{"m15", " 15m "},
	} {
		if err := repo.Append(context.Background(), domainsignal.Signal{
			ID: tc.id, Kind: domainsignal.KindBadge, Symbol: "BTCUSDT",
			Timeframe: tc.tf, Label: "gain", Price: 1, ATR: 1,
			EmittedAt: emitted, HorizonBars: 2,
		}); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := repo.UnresolvedInvalidTF(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) != 0 {
		t.Fatalf("canonical-valid TFs must not be invalid: %v", inv)
	}
	now := emitted.Add(3 * time.Hour)
	ready, err := repo.UnresolvedReady(context.Background(), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 2 {
		t.Fatalf("ready=%v want 2", ready)
	}
	for _, s := range ready {
		if s.Timeframe != "1h" && s.Timeframe != "15m" {
			t.Fatalf("stored tf=%q want canonical", s.Timeframe)
		}
	}
}
