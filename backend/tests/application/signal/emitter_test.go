package signal_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	appsignal "pano_chart/backend/application/signal"
	mkt "pano_chart/backend/domain/market"
	domainsignal "pano_chart/backend/domain/signal"
	infrasignal "pano_chart/backend/infrastructure/signal"
)

func TestSQLiteRepository_RoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s := domainsignal.Signal{
		ID:          "sig-1",
		Kind:        domainsignal.KindBadge,
		Symbol:      "BTCUSDT",
		Timeframe:   "1h",
		Label:       "trend_up",
		Score:       0.9,
		Price:       50000,
		ATR:         100,
		Context:     map[string]float64{"percentile": 0.95},
		EmittedAt:   at,
		HorizonBars: 20,
	}
	if err := repo.Append(context.Background(), s); err != nil {
		t.Fatalf("Append: %v", err)
	}

	unresolved, err := repo.Unresolved(context.Background(), at.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("Unresolved: %v", err)
	}
	if len(unresolved) != 1 || unresolved[0].ID != "sig-1" {
		t.Fatalf("unresolved=%#v", unresolved)
	}

	oc := domainsignal.Outcome{
		SignalID:      "sig-1",
		ResolvedAt:    at.Add(2 * time.Hour),
		ForwardReturn: 0.01,
		MaxFavorable:  1.5,
		MaxAdverse:    0.2,
		Success:       true,
		Rule:          "badge trend_up",
	}
	if err := repo.MarkResolved(context.Background(), "sig-1", oc); err != nil {
		t.Fatalf("MarkResolved: %v", err)
	}

	unresolved, err = repo.Unresolved(context.Background(), at.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("Unresolved after resolve: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved, got %d", len(unresolved))
	}

	rows, err := repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindBadge, Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 || rows[0].Outcome == nil || !rows[0].Outcome.Success {
		t.Fatalf("query=%#v", rows)
	}
}

func TestEmitter_DedupWithinCandle(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	now := time.Date(2026, 3, 1, 12, 30, 0, 0, time.UTC)
	em.SetNow(func() time.Time { return now })

	sig := domainsignal.Signal{
		Kind: domainsignal.KindSetup, Symbol: "ETHUSDT", Timeframe: "1h",
		Label: "compression", Score: 0.7, Price: 3000, ATR: 50,
	}
	if !em.Emit(context.Background(), sig) {
		t.Fatal("first emit should succeed")
	}
	if !em.Emit(context.Background(), sig) {
		t.Fatal("dedupe hit should still return true")
	}

	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 signal after dedup, got %d", len(rows))
	}

	em.SetNow(func() time.Time { return now.Add(20 * time.Minute) })
	em.Emit(context.Background(), sig)
	rows, _ = repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("expected still 1 within same candle, got %d", len(rows))
	}

	em.SetNow(func() time.Time { return now.Add(time.Hour) })
	em.Emit(context.Background(), sig)
	rows, err = repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 signals after next candle, got %d", len(rows))
	}
}

func TestEmitter_AppendFailureDoesNotPoisonDedupe(t *testing.T) {
	fail := &failOnceRepo{err: errors.New("sqlite busy")}
	em := appsignal.NewEmitter(fail)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	em.SetNow(func() time.Time { return now })

	sig := domainsignal.Signal{
		Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Score: 0.8, Price: 1, ATR: 0.1,
	}
	if em.Emit(context.Background(), sig) {
		t.Fatal("first emit should fail")
	}
	if !em.Emit(context.Background(), sig) {
		t.Fatal("retry after failure should succeed")
	}
	if fail.n != 2 {
		t.Fatalf("expected 2 Append attempts after failure, got %d", fail.n)
	}
}

func TestEmitter_ConcurrentSameKeySingleRow(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	em.SetNow(func() time.Time { return now })

	sig := domainsignal.Signal{
		Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Score: 0.9, Price: 100, ATR: 1,
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			em.Emit(context.Background(), sig)
		}()
	}
	wg.Wait()
	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row under concurrent Emit, got %d", len(rows))
	}
}

func TestEmitter_CanceledContextStillPersists(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !em.Emit(ctx, domainsignal.Signal{
		Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Score: 0.9, Price: 100, ATR: 1,
	}) {
		t.Fatal("canceled request ctx must still persist")
	}
}

func TestEmitter_SkipsZeroPriceATR(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	if em.Emit(context.Background(), domainsignal.Signal{
		Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "trend_up", Score: 0.9, Price: 0, ATR: 1,
	}) {
		t.Fatal("expected skip on price<=0")
	}
}

func TestEmitter_NilRepoNoop(t *testing.T) {
	em := appsignal.NewEmitter(nil)
	if em.Emit(context.Background(), domainsignal.Signal{Kind: domainsignal.KindBadge, Label: "trend_up"}) {
		t.Fatal("nil repo must return false")
	}
}

func TestRegimeWriter_EmitsOnlyOnChange(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	w := appsignal.NewRegimeWriter(em)

	_ = w.Update("1h", "trend", "up", 1_700_000_000)
	_ = w.Update("1h", "trend", "up", 1_700_000_100)
	_ = w.Update("1h", "sideways", "neutral", 1_700_000_200)

	rows, err := repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 regime signals, got %d", len(rows))
	}
	var foundBias bool
	for _, r := range rows {
		if r.Signal.Label == "regime:trend" && r.Signal.Context["bias"] == 1 {
			foundBias = true
		}
	}
	if !foundBias {
		t.Fatalf("expected regime:trend with bias=1, rows=%#v", rows)
	}
}

func TestRegimeWriter_AppendFailureDoesNotAdvanceLast(t *testing.T) {
	fail := &failAlwaysRepo{err: errors.New("disk full")}
	em := appsignal.NewEmitter(fail)
	w := appsignal.NewRegimeWriter(em)

	_ = w.Update("1h", "trend", "up", 1)
	_ = w.Update("1h", "trend", "up", 2) // must retry — last not advanced
	if atomic.LoadInt64(&fail.n) < 2 {
		t.Fatalf("expected retries after Append failure, got %d", fail.n)
	}

	// Swap to succeeding repo via new emitter/writer path: rebuild with working repo.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	okEm := appsignal.NewEmitter(repo)
	okW := appsignal.NewRegimeWriter(okEm)
	// Simulate poisoned writer by Seed-ing nothing and failing first then succeeding
	// through the failAlways path already covered; here assert success path works.
	_ = okW.Update("1h", "trend", "up", 1)
	rows, _ := repo.Query(context.Background(), domainsignal.Filter{Limit: 5})
	if len(rows) != 1 {
		t.Fatalf("got %d", len(rows))
	}
}

func TestRegimeWriter_SeedPreventsColdStartReemit(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	w := appsignal.NewRegimeWriter(em)
	w.Seed("1h", "trend")

	_ = w.Update("1h", "trend", "up", 1_700_000_000)
	rows, _ := repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 10})
	if len(rows) != 0 {
		t.Fatalf("seeded regime must not re-emit on restart, got %d", len(rows))
	}

	_ = w.Update("1h", "sideways", "neutral", 1_700_000_100)
	rows, _ = repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 10})
	if len(rows) != 1 || rows[0].Signal.Label != "regime:sideways" {
		t.Fatalf("expected change emit, got %#v", rows)
	}
}

func TestRegimeWriter_LazyLookupPreventsColdStart(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	em := appsignal.NewEmitter(repo)
	w := appsignal.NewRegimeWriter(em)
	w.SetHistoryLookup(func(tf string) (mkt.Regime, bool) {
		if tf == "15m" {
			return "trend", true
		}
		return "", false
	})

	_ = w.Update("15m", "trend", "up", 1_700_000_000)
	rows, _ := repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 10})
	if len(rows) != 0 {
		t.Fatalf("lazy lookup must suppress cold-start re-emit, got %d", len(rows))
	}

	_ = w.Update("15m", "sideways", "neutral", 1_700_000_100)
	rows, _ = repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 10})
	if len(rows) != 1 || rows[0].Signal.Label != "regime:sideways" {
		t.Fatalf("expected real change after lazy seed, got %#v", rows)
	}
}

func TestRegimeWriter_NilEmitterNoop(t *testing.T) {
	w := appsignal.NewRegimeWriter(nil)
	if err := w.Update("1h", "trend", "up", 1); err != nil {
		t.Fatal(err)
	}
}

func TestRegimeWriter_DisabledEmitterNoop(t *testing.T) {
	// Soft-fail path equivalent: non-nil writer over a nil-repo emitter must
	// not keep retrying forever — Enabled() is false so Update is a no-op.
	w := appsignal.NewRegimeWriter(appsignal.NewEmitter(nil))
	if err := w.Update("1h", "trend", "up", 1); err != nil {
		t.Fatal(err)
	}
	if err := w.Update("1h", "sideways", "neutral", 2); err != nil {
		t.Fatal(err)
	}
}

func TestEmitter_Enabled(t *testing.T) {
	if appsignal.NewEmitter(nil).Enabled() {
		t.Fatal("nil repo must be disabled")
	}
	var nilEm *appsignal.Emitter
	if nilEm.Enabled() {
		t.Fatal("nil emitter must be disabled")
	}
}

func TestLabels_BadgeAndSetup(t *testing.T) {
	if got := appsignal.BadgeLabel("trend", []float64{1, 2, 3}); got != "trend_up" {
		t.Fatalf("trend_up: %s", got)
	}
	if got := appsignal.BadgeLabel("trend", []float64{3, 2, 1}); got != "trend_down" {
		t.Fatalf("trend_down: %s", got)
	}
	if got := appsignal.SetupLabel("compression_breakout", "compression", 0.6, 0.2); got != "breakout_up" {
		t.Fatalf("breakout_up: %s", got)
	}
	if got := appsignal.SetupLabel("compression_breakout", "compression", 0.2, 0.2); got != "compression" {
		t.Fatalf("compression: %s", got)
	}
	if got := appsignal.SetupLabel("range_reversion", "sideways", 0, 0); got != "range" {
		t.Fatalf("range: %s", got)
	}
	if got := appsignal.SetupLabel("trend_continuation", "downtrend", 0, 0); got != "trend_down" {
		t.Fatalf("trend_down setup: %s", got)
	}
	if got := appsignal.SetupLabel("trend_continuation", "sideways", 0.1, 0.4); got != "trend_down" {
		t.Fatalf("sideways continuation uses breakout tilt: %s", got)
	}
}

type failOnceRepo struct {
	mu  sync.Mutex
	n   int
	err error
}

func (f *failOnceRepo) Append(ctx context.Context, s domainsignal.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if f.n == 1 {
		return f.err
	}
	return nil
}

func (f *failOnceRepo) Unresolved(context.Context, time.Time, int) ([]domainsignal.Signal, error) {
	return nil, nil
}
func (f *failOnceRepo) MarkResolved(context.Context, string, domainsignal.Outcome) error {
	return nil
}
func (f *failOnceRepo) Query(context.Context, domainsignal.Filter) ([]domainsignal.SignalWithOutcome, error) {
	return nil, nil
}

type failAlwaysRepo struct {
	n   int64
	err error
}

func (f *failAlwaysRepo) Append(context.Context, domainsignal.Signal) error {
	atomic.AddInt64(&f.n, 1)
	return f.err
}
func (f *failAlwaysRepo) Unresolved(context.Context, time.Time, int) ([]domainsignal.Signal, error) {
	return nil, nil
}
func (f *failAlwaysRepo) MarkResolved(context.Context, string, domainsignal.Outcome) error {
	return nil
}
func (f *failAlwaysRepo) Query(context.Context, domainsignal.Filter) ([]domainsignal.SignalWithOutcome, error) {
	return nil, nil
}
