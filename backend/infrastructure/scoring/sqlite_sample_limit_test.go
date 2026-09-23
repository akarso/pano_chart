package scoring

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSQLiteSampleSink_ScoresCapsAtMostRecent(t *testing.T) {
	sink, err := NewSQLiteSampleSink(t.TempDir()+"/samples.sqlite", 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	sink.scoreLimit = 3

	ctx := context.Background()
	base := time.Now().UTC()
	for i, score := range []float64{1, 2, 3, 4, 5} {
		if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", score, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := sink.Scores(ctx, "sideways", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 3 || got[1] != 4 || got[2] != 5 {
		t.Fatalf("scores=%v", got)
	}
}

func TestSQLiteSampleSink_CalculatorsRetriesAfterFailedLoad(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink, err := NewSQLiteSampleSink(path, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, firstErr := sink.Calculators(ctx)
	if firstErr == nil {
		t.Fatal("expected canceled load to fail")
	}
	if sink.namesLoaded {
		t.Fatal("failed load was cached as success")
	}
	_, secondErr := sink.Calculators(context.Background())
	if secondErr != firstErr {
		t.Fatalf("retry inside the fail cache = %v, want %v", secondErr, firstErr)
	}

	sink.namesFailedAt = time.Now().Add(-2 * calculatorFailCache)
	closed, err := sqlOpenClosed(path)
	if err != nil {
		t.Fatal(err)
	}
	real := sink.db
	sink.db = closed
	if _, err := sink.Calculators(context.Background()); err == nil {
		t.Fatal("expected closed database to fail")
	}
	if sink.namesLoaded {
		t.Fatal("database error was cached as success")
	}
	sink.db = real
	sink.namesFailedAt = time.Now().Add(-2 * calculatorFailCache)

	names, err := sink.Calculators(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("names=%v", names)
	}
	if !sink.namesLoaded {
		t.Fatal("successful load was not cached")
	}
}

// Non-constraint failures abort the batch. query_only produces SQLITE_READONLY,
// the same branch as a disk or full-database error.
func TestSQLiteSampleSink_ReadOnlyAbortsBatch(t *testing.T) {
	sink, err := NewSQLiteSampleSink(t.TempDir()+"/samples.sqlite", 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	if _, err := sink.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", 0.5, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err == nil {
		t.Fatal("expected flush to report the rejected batch")
	}
	if _, err := sink.db.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := sink.db.QueryRow(`SELECT COUNT(*) FROM score_samples`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rows=%d", n)
	}
}

func TestCalculatorsUsesRetainedRowsOnly(t *testing.T) {
	sink, err := NewSQLiteSampleSink(t.TempDir()+"/samples.sqlite", 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	ctx := context.Background()
	names, err := sink.Calculators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("names=%v", names)
	}

	now := time.Now().UTC()
	if err := sink.Record(ctx, "kept", "BTCUSDT", "1h", 0.2, now); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	names, err = sink.Calculators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "kept" {
		t.Fatalf("after commit names=%v", names)
	}

	if _, err := sink.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(ctx, "ghost", "BTCUSDT", "1h", 0.2, now); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(ctx); err == nil {
		t.Fatal("expected flush to report the rejected batch")
	}
	if _, err := sink.db.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	names, err = sink.Calculators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "kept" {
		t.Fatalf("after failed insert names=%v", names)
	}

	sink.now = func() time.Time { return now.Add(91 * 24 * time.Hour) }
	if err := sink.Purge(); err != nil {
		t.Fatal(err)
	}
	names, err = sink.Calculators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("after purge names=%v", names)
	}
}

func TestFlushReportsEarlierBatchFailure(t *testing.T) {
	sink, err := NewSQLiteSampleSink(t.TempDir()+"/samples.sqlite", 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	if _, err := sink.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < sampleBatchSize+5; i++ {
		if err := sink.Record(ctx, "sideways", "BTCUSDT", "1h", 0.5, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(ctx); err == nil {
		t.Fatal("expected flush to report the batch that failed before the marker")
	}
}

func TestFlushReleasesMutexWhileQueueIsFull(t *testing.T) {
	path := t.TempDir() + "/samples.sqlite"
	sink, err := NewSQLiteSampleSink(path, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	locker, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = locker.Exec("ROLLBACK")
		_ = locker.Close()
		_ = sink.Close()
	})
	if _, err := locker.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < sampleQueueSize+sampleBatchSize; i++ {
		if err := sink.Record(context.Background(), "sideways", "BTCUSDT", "1h", 0.5, now); err != nil {
			t.Fatal(err)
		}
	}
	flushCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = sink.Flush(flushCtx) }()
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	err = sink.Close()
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("close blocked for %s", elapsed)
	}
	if err == nil {
		t.Fatal("expected close to give up while the writer was stuck")
	}
}

func TestPublishNamesSkipsStaleGeneration(t *testing.T) {
	sink := &SQLiteSampleSink{
		names:    map[string]struct{}{"kept": {}},
		namesGen: 2,
	}
	if sink.publishNames(1, map[string]struct{}{"stale": {}}) {
		t.Fatal("published a load from an older generation")
	}
	if _, ok := sink.names["stale"]; ok {
		t.Fatal("stale name replaced the current map")
	}
	if _, ok := sink.names["kept"]; !ok {
		t.Fatal("current names were discarded")
	}
	if !sink.publishNames(2, map[string]struct{}{"fresh": {}}) {
		t.Fatal("current generation was not published")
	}
	if _, ok := sink.names["fresh"]; !ok || !sink.namesLoaded {
		t.Fatalf("names=%v loaded=%v", sink.names, sink.namesLoaded)
	}
}

func TestLogLimitedDebounce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	sink := &SQLiteSampleSink{}
	sink.logLimited("one")
	sink.logLimited("two")
	if strings.Count(buf.String(), "\n") != 1 || !strings.Contains(buf.String(), "one") {
		t.Fatalf("%q", buf.String())
	}
}

func sqlOpenClosed(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return db, nil
}
