package scoring_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"pano_chart/backend/domain"
	infrascoring "pano_chart/backend/infrastructure/scoring"
)

func TestSQLiteSampleSink_RoundTripAndRetention(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	ctx := context.Background()

	now := time.Now().UTC()
	recent := []float64{0.2, 0.8, 0.5}
	for _, score := range recent {
		if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", score, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Record(ctx, "sideways", "ETHUSDT", "4h", 0.1, now); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-91 * 24 * time.Hour)
	if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", 0.99, old); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := sink.Scores(ctx, "sideways", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 0.2 || got[1] != 0.5 || got[2] != 0.8 {
		t.Fatalf("scores=%v", got)
	}
	names, err := sink.Calculators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "sideways" {
		t.Fatalf("calculators=%v", names)
	}
	if n := countSamples(t, path); n != 5 {
		t.Fatalf("rows before purge=%d", n)
	}

	if err := sink.Purge(); err != nil {
		t.Fatal(err)
	}
	if n := countSamples(t, path); n != 4 {
		t.Fatalf("rows after purge=%d", n)
	}

	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openSink(t, path)
	got, err = reopened.Scores(ctx, "sideways", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("reopened scores=%v", got)
	}
	if err := reopened.Record(ctx, "sideways", "BTCUSDT", "1h", 0.01, now.Add(-100*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Purge(); err != nil {
		t.Fatal(err)
	}
	if n := countSamples(t, path); n != 4 {
		t.Fatalf("rows after purge=%d", n)
	}
	if !indexExists(t, path, "idx_score_samples_calc_tf_at") {
		t.Fatal("missing (calculator, tf, at) index")
	}
	if !indexExists(t, path, "idx_score_samples_at") {
		t.Fatal("missing at index")
	}
}

func TestSQLiteSampleSink_ConcurrentRecord(t *testing.T) {
	sink := openSink(t, t.TempDir()+"/samples.sqlite")
	ctx := context.Background()
	var wg sync.WaitGroup
	for w := 0; w < 12; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", 0.5, time.Now()); err != nil {
					t.Errorf("record: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := sink.Scores(ctx, "sideways", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 240 {
		t.Fatalf("scores=%d", len(got))
	}
}

func TestSQLiteSampleSink_RecordDoesNotWaitOnLockedDB(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	locker, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = locker.Exec("ROLLBACK")
		_ = locker.Close()
	})
	if _, err := locker.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < 2000; i++ {
		if err := sink.Record(context.Background(), "sideways", "BTCUSDT", "1h", 0.5, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Record blocked for %s", elapsed)
	}
}

func TestSQLiteSampleSink_RetentionTickDeletes(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sink.RunRetentionEvery(ctx, 20*time.Millisecond)

	time.Sleep(40 * time.Millisecond)
	old := time.Now().Add(-100 * 24 * time.Hour)
	if err := sink.Record(context.Background(), "sideways", "BTCUSDT", "1h", 0.5, old); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if countSamples(t, path) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduled purge did not delete the expired sample")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSQLiteSampleSink_CloseDoesNotHangWhileWriterBlocked(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	locker, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = locker.Exec("ROLLBACK")
		_ = locker.Close()
	})
	if _, err := locker.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 80; i++ {
		if err := sink.Record(context.Background(), "sideways", "BTCUSDT", "1h", 0.5, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	err = sink.Close()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected close to give up while the writer was stuck")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("close hung for %s", elapsed)
	}
}

func TestSQLiteSampleSink_SkipsBadRow(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TRIGGER fail_bad BEFORE INSERT ON score_samples
		WHEN NEW.symbol = 'BAD'
		BEGIN
			SELECT RAISE(ABORT, 'bad row');
		END`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, symbol := range []string{"BTCUSDT", "BAD", "ETHUSDT"} {
		if err := sink.Record(ctx, "sideways", symbol, "1h", 0.5, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countSamples(t, path); n != 2 {
		t.Fatalf("rows=%d", n)
	}
}

func TestSQLiteSampleSink_TransactionRollbackDropsBatch(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink := openSink(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TRIGGER fail_txn BEFORE INSERT ON score_samples
		WHEN NEW.symbol = 'BAD'
		BEGIN
			SELECT RAISE(ROLLBACK, 'txn');
		END`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, symbol := range []string{"BTCUSDT", "BAD", "ETHUSDT"} {
		if err := sink.Record(ctx, "sideways", symbol, "1h", 0.5, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err == nil {
		t.Fatal("expected flush to report the rolled-back batch")
	}
	if n := countSamples(t, path); n != 0 {
		t.Fatalf("rows=%d", n)
	}
}

func TestSQLiteSampleSink_RunRetentionStops(t *testing.T) {
	sink := openSink(t, t.TempDir()+"/samples.sqlite")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sink.RunRetention(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retention did not stop")
	}
}

func TestRetentionFromEnv(t *testing.T) {
	if got := infrascoring.RetentionFromEnv(""); got != 90*24*time.Hour {
		t.Fatalf("empty=%s", got)
	}
	if got := infrascoring.RetentionFromEnv("30d"); got != 30*24*time.Hour {
		t.Fatalf("days=%s", got)
	}
	if got := infrascoring.RetentionFromEnv("2160h"); got != 2160*time.Hour {
		t.Fatalf("duration=%s", got)
	}
	if got := infrascoring.RetentionFromEnv("nope"); got != 90*24*time.Hour {
		t.Fatalf("invalid=%s", got)
	}
}

func TestAttachSampleSink_OpenFailureKeepsLogging(t *testing.T) {
	logs := captureLog(t)
	calc := infrascoring.NewLoggingScoreCalculator(&fakeCalc{name: "X", score: 0.4}, 1)
	path := filepath.Join(t.TempDir(), "missing", "samples.sqlite")
	_, err := infrascoring.AttachSampleSink(calc, path, 0)
	if err == nil {
		t.Fatal("expected open failure")
	}
	if _, err := calc.Score(domain.CandleSeries{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "score=") {
		t.Fatalf("log=%q", logs.String())
	}
}

func TestAttachSampleSink_Records(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	calc := infrascoring.NewLoggingScoreCalculator(&fakeCalc{name: "Sideways Consistency", score: 0.4}, 1)
	sink, err := infrascoring.AttachSampleSink(calc, path, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	series, err := domain.NewCandleSeries(domain.NewSymbolUnsafe("btcusdt"), domain.NewTimeframeUnsafe("1h"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := calc.Score(series); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := sink.Scores(ctx, "Sideways Consistency", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 0.4 {
		t.Fatalf("scores=%v", got)
	}
}

func openSink(t *testing.T, path string) *infrascoring.SQLiteSampleSink {
	t.Helper()
	sink, err := infrascoring.NewSQLiteSampleSink(path, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return sink
}

func countSamples(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM score_samples`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func indexExists(t *testing.T, path, name string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&got)
	return err == nil && got == name
}
