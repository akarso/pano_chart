package signal_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	if !rows[0].Signal.EmittedAt.Equal(at) {
		t.Fatalf("emitted_at round-trip: got %v want %v", rows[0].Signal.EmittedAt, at)
	}
}

func TestSQLiteRepository_ChronologicalOrderAcrossSubsecond(t *testing.T) {
	// Regression: variable-width RFC3339Nano text ordered "…00Z" after "…00.1Z".
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 3, 1, 12, 0, 0, 100_000_000, time.UTC) // +100ms
	for i, at := range []time.Time{t1, t0} {                     // insert later first
		if err := repo.Append(context.Background(), domainsignal.Signal{
			ID: fmt.Sprintf("s%d", i), Kind: domainsignal.KindBadge,
			Symbol: "BTCUSDT", Timeframe: "1h", Label: "trend_up",
			Score: 1, Price: 1, ATR: 1, EmittedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len=%d", len(rows))
	}
	// DESC: newest first
	if !rows[0].Signal.EmittedAt.Equal(t1) || !rows[1].Signal.EmittedAt.Equal(t0) {
		t.Fatalf("order=%v %v", rows[0].Signal.EmittedAt, rows[1].Signal.EmittedAt)
	}
	before := t1
	unresolved, err := repo.Unresolved(context.Background(), before, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 1 || !unresolved[0].EmittedAt.Equal(t0) {
		t.Fatalf("unresolved before t1: %#v", unresolved)
	}
}

func TestSQLiteRepository_MigratesLegacyTextTimestampsWithOutcomes(t *testing.T) {
	// Populated legacy TEXT schema + FK-enforced outcomes must upgrade without
	// failing when dropping renamed parent/child tables.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`CREATE TABLE signals (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			symbol TEXT NOT NULL DEFAULT '',
			timeframe TEXT NOT NULL,
			label TEXT NOT NULL,
			score REAL NOT NULL,
			price REAL NOT NULL DEFAULT 0,
			atr REAL NOT NULL DEFAULT 0,
			context TEXT NOT NULL DEFAULT '{}',
			emitted_at TEXT NOT NULL,
			horizon_bars INTEGER NOT NULL DEFAULT 20
		)`,
		`CREATE TABLE outcomes (
			signal_id TEXT PRIMARY KEY,
			resolved_at TEXT NOT NULL,
			forward_return REAL NOT NULL,
			max_favorable REAL NOT NULL,
			max_adverse REAL NOT NULL,
			success INTEGER NOT NULL,
			rule TEXT NOT NULL,
			FOREIGN KEY(signal_id) REFERENCES signals(id)
		)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 3, 1, 12, 0, 0, 123456789, time.UTC)
	if _, err := db.Exec(`INSERT INTO signals
		(id, kind, symbol, timeframe, label, score, price, atr, context, emitted_at, horizon_bars)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-1", "badge", "BTCUSDT", "1h", "trend_up", 0.9, 100.0, 1.0, "{}",
		at.Format(time.RFC3339Nano), 20,
	); err != nil {
		t.Fatal(err)
	}
	resolved := at.Add(time.Hour)
	if _, err := db.Exec(`INSERT INTO outcomes
		(signal_id, resolved_at, forward_return, max_favorable, max_adverse, success, rule)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"legacy-1", resolved.Format(time.RFC3339Nano), 0.01, 1.0, 0.2, 1, "badge trend_up",
	); err != nil {
		t.Fatal(err)
	}

	repo, err := infrasignal.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatalf("legacy migration failed: %v", err)
	}

	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 migrated row, got %d", len(rows))
	}
	if rows[0].Signal.ID != "legacy-1" || rows[0].Outcome == nil || !rows[0].Outcome.Success {
		t.Fatalf("migrated=%#v", rows[0])
	}
	if !rows[0].Signal.EmittedAt.Equal(at) {
		t.Fatalf("emitted_at=%v want %v", rows[0].Signal.EmittedAt, at)
	}
	if !rows[0].Outcome.ResolvedAt.Equal(resolved) {
		t.Fatalf("resolved_at=%v want %v", rows[0].Outcome.ResolvedAt, resolved)
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

func TestEmitter_SkipsZeroPrice(t *testing.T) {
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

func TestEmitter_AllowsZeroATR(t *testing.T) {
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
	if !em.Emit(context.Background(), domainsignal.Signal{
		Kind: domainsignal.KindBadge, Symbol: "BTCUSDT", Timeframe: "1h",
		Label: "sideways", Score: 0.9, Price: 100, ATR: 0,
	}) {
		t.Fatal("flat series ATR=0 must still persist")
	}
	rows, err := repo.Query(context.Background(), domainsignal.Filter{Limit: 5})
	if err != nil || len(rows) != 1 || rows[0].Signal.ATR != 0 {
		t.Fatalf("got %#v err=%v", rows, err)
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

func TestRegimeWriter_SameTFSerialized(t *testing.T) {
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

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.Update("1h", "sideways", "neutral", 1_700_000_000)
		}()
	}
	wg.Wait()

	rows, err := repo.Query(context.Background(), domainsignal.Filter{Kind: domainsignal.KindRegime, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("concurrent same-TF change must emit once, got %d", len(rows))
	}
	if rows[0].Signal.Label != "regime:sideways" {
		t.Fatalf("label=%s", rows[0].Signal.Label)
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
func (f *failOnceRepo) UnresolvedReady(context.Context, time.Time, int) ([]domainsignal.Signal, error) {
	return nil, nil
}
func (f *failOnceRepo) UnresolvedInvalidTF(context.Context, int) ([]domainsignal.Signal, error) {
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
func (f *failAlwaysRepo) UnresolvedReady(context.Context, time.Time, int) ([]domainsignal.Signal, error) {
	return nil, nil
}
func (f *failAlwaysRepo) UnresolvedInvalidTF(context.Context, int) ([]domainsignal.Signal, error) {
	return nil, nil
}
func (f *failAlwaysRepo) MarkResolved(context.Context, string, domainsignal.Outcome) error {
	return nil
}
func (f *failAlwaysRepo) Query(context.Context, domainsignal.Filter) ([]domainsignal.SignalWithOutcome, error) {
	return nil, nil
}
