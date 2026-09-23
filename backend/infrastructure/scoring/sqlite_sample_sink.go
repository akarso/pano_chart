package scoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"
)

const (
	defaultSampleRetention = 90 * 24 * time.Hour
	sampleQueueSize        = 1024
	sampleBatchSize        = 64
	sampleFlushEvery       = 50 * time.Millisecond
	retentionJitter        = 15 * time.Minute
	sampleCloseTimeout     = 2 * time.Second
	maxDistributionSamples = 100_000
	calculatorLoadTimeout  = 3 * time.Second
	calculatorFailCache    = time.Second
	sqliteConstraint       = 19 // primary SQLITE_CONSTRAINT, including extended codes
)

// SQLiteSampleSink stores sampled calculator scores.
// Record enqueues and returns; a background writer batches inserts, and drops
// a sample when the queue is full so scoring never waits on disk.
// Timestamps are UTC unix nanoseconds. RunRetention deletes rows older than
// the retention window immediately, then about once a day.
type SQLiteSampleSink struct {
	db         *sql.DB
	now        func() time.Time
	retention  time.Duration
	scoreLimit int

	mu       sync.Mutex // closed flag and channel send; disk I/O stays on the writer
	ch       chan sampleWrite
	done     chan struct{}
	closed   bool
	lastDrop atomic.Int64
	lastSkip atomic.Int64

	namesMu       sync.Mutex
	names         map[string]struct{}
	namesLoaded   bool
	namesGen      uint64
	namesErr      error
	namesFailedAt time.Time
	loadMu        sync.Mutex
}

type sampleWrite struct {
	calculator string
	symbol     string
	tf         string
	score      float64
	at         int64
	done       chan error
}

// RetentionFromEnv parses PC_SCORE_SAMPLE_RETENTION (`90d` or a Go duration).
// Empty, non-positive, and invalid values use 90 days.
func RetentionFromEnv(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultSampleRetention
	}
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return defaultSampleRetention
		}
		return time.Duration(n) * 24 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultSampleRetention
	}
	return d
}

// NewSQLiteSampleSink opens (or creates) the sample DB and starts the writer.
// retention <= 0 uses 90 days. Expired rows are removed by RunRetention, not here.
func NewSQLiteSampleSink(dbPath string, retention time.Duration) (*SQLiteSampleSink, error) {
	if retention <= 0 {
		retention = defaultSampleRetention
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening score sample db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=15000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy_timeout: %w", err)
	}
	sink := &SQLiteSampleSink{
		db:         db,
		now:        time.Now,
		retention:  retention,
		scoreLimit: maxDistributionSamples,
		names:      map[string]struct{}{},
		ch:         make(chan sampleWrite, sampleQueueSize),
		done:       make(chan struct{}),
	}
	if err := sink.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("score sample migrations: %w", err)
	}
	go sink.loop()
	return sink, nil
}

// AttachSampleSink opens the DB and installs it on calc.
// On failure calc is unchanged, so sampled scores keep being logged.
func AttachSampleSink(calc *LoggingScoreCalculator, dbPath string, retention time.Duration) (*SQLiteSampleSink, error) {
	sink, err := NewSQLiteSampleSink(dbPath, retention)
	if err != nil {
		return nil, err
	}
	if calc != nil {
		calc.SetSink(sink)
	}
	return sink, nil
}

// Close drains the writer and closes the DB. Nil-safe.
// If the writer is still blocked after 2s, Close logs and returns so process
// shutdown is not stuck behind disk I/O. The database handle is left open in
// that case; exiting the process releases it.
func (s *SQLiteSampleSink) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.ch)
	s.mu.Unlock()
	timer := time.NewTimer(sampleCloseTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return s.db.Close()
	case <-timer.C:
		log.Printf("[score-samples] writer did not drain within %s; leaving the database open", sampleCloseTimeout)
		return fmt.Errorf("score sample writer did not drain")
	}
}

// RunRetention deletes expired samples immediately, then about once a day
// (waits longer than 24h by up to 15 minutes) until ctx is canceled.
func (s *SQLiteSampleSink) RunRetention(ctx context.Context) {
	s.RunRetentionEvery(ctx, 24*time.Hour)
}

// RunRetentionEvery purges once, then every interval, until ctx is canceled.
// Intervals of at least an hour get up to 15 minutes of extra delay.
func (s *SQLiteSampleSink) RunRetentionEvery(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if err := s.Purge(); err != nil {
		log.Printf("[score-samples] retention: %v", err)
	}
	for {
		timer := time.NewTimer(retentionWait(interval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := s.Purge(); err != nil {
				log.Printf("[score-samples] retention: %v", err)
			}
		}
	}
}

// retentionWait uses the process-wide math/rand source, which is safe for
// concurrent callers. This draw is separate from Score's sample-rate check.
func retentionWait(interval time.Duration) time.Duration {
	if interval < time.Hour {
		return interval
	}
	return interval + time.Duration(rand.Int63n(int64(retentionJitter)))
}

// Record enqueues one sample. A full queue drops the sample and returns nil.
func (s *SQLiteSampleSink) Record(ctx context.Context, calculator, symbol, tf string, score float64, at time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("score sample sink is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	req := sampleWrite{
		calculator: calculator,
		symbol:     symbol,
		tf:         tf,
		score:      score,
		at:         at.UTC().UnixNano(),
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("score sample sink is closed")
	}
	select {
	case s.ch <- req:
		s.mu.Unlock()
		return nil
	default:
		s.mu.Unlock()
		s.logDrop()
		return nil
	}
}

// Flush waits until samples enqueued before this call have been written.
// It returns the writer error when a batch was rolled back or otherwise
// not committed.
func (s *SQLiteSampleSink) Flush(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("score sample sink is closed")
	}
	done := make(chan error, 1)
	marker := sampleWrite{done: done}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return fmt.Errorf("score sample sink is closed")
		}
		select {
		case s.ch <- marker:
			s.mu.Unlock()
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		default:
			s.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

// Scores returns retained scores for calculator and tf.
// Only the most recent maxDistributionSamples rows in the window are loaded.
// The connection is released before the scores are sorted.
func (s *SQLiteSampleSink) Scores(ctx context.Context, calculator, tf string) ([]float64, error) {
	limit := s.scoreLimit
	if limit <= 0 {
		limit = maxDistributionSamples
	}
	cutoff := s.cutoff()
	rows, err := s.db.QueryContext(ctx,
		`SELECT score FROM score_samples
		 WHERE calculator = ? AND tf = ? AND at >= ?
		 ORDER BY at DESC LIMIT ?`,
		calculator, tf, cutoff, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("reading score samples: %w", err)
	}
	out, err := scanScores(rows)
	if err != nil {
		return nil, err
	}
	sort.Float64s(out)
	return out, nil
}

// Calculators lists calculator names seen in this process or still stored.
// The list ignores timeframe: a name is known when any retained sample exists
// for it. Names come from retained rows: a commit or purge drops the cache
// so the next call reloads. A failed load is reused for one second, then the
// next call tries again. The load uses ctx and stops after calculatorLoadTimeout.
func (s *SQLiteSampleSink) Calculators(ctx context.Context) ([]string, error) {
	for {
		if err := s.ensureNames(ctx); err != nil {
			return nil, err
		}
		s.namesMu.Lock()
		if !s.namesLoaded {
			s.namesMu.Unlock()
			continue
		}
		out := make([]string, 0, len(s.names))
		for name := range s.names {
			out = append(out, name)
		}
		s.namesMu.Unlock()
		sort.Strings(out)
		return out, nil
	}
}

func (s *SQLiteSampleSink) ensureNames(ctx context.Context) error {
	if err, ok := s.cachedNames(time.Now()); ok {
		return err
	}
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	for {
		if err, ok := s.cachedNames(time.Now()); ok {
			return err
		}
		s.namesMu.Lock()
		gen := s.namesGen
		s.namesMu.Unlock()
		loadCtx, cancel := context.WithTimeout(ctx, calculatorLoadTimeout)
		next, err := s.loadNames(loadCtx)
		cancel()
		if err != nil {
			s.noteNamesFailure(err)
			return err
		}
		if s.publishNames(gen, next) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *SQLiteSampleSink) noteNamesFailure(err error) {
	s.namesMu.Lock()
	s.namesErr = err
	s.namesFailedAt = time.Now()
	s.namesMu.Unlock()
}

func (s *SQLiteSampleSink) publishNames(gen uint64, next map[string]struct{}) bool {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	if s.namesGen != gen {
		return false
	}
	s.names = next
	s.namesLoaded = true
	s.namesErr = nil
	s.namesFailedAt = time.Time{}
	return true
}

func (s *SQLiteSampleSink) cachedNames(now time.Time) (error, bool) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	if s.namesLoaded {
		return nil, true
	}
	if !s.namesFailedAt.IsZero() && now.Sub(s.namesFailedAt) < calculatorFailCache {
		return s.namesErr, true
	}
	return nil, false
}

func (s *SQLiteSampleSink) invalidateNames() {
	s.namesMu.Lock()
	s.namesGen++
	s.namesLoaded = false
	s.namesFailedAt = time.Time{}
	s.namesMu.Unlock()
}

func (s *SQLiteSampleSink) loadNames(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT calculator FROM score_samples WHERE at >= ?`,
		s.cutoff(),
	)
	if err != nil {
		return nil, fmt.Errorf("listing score calculators: %w", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning score calculator: %w", err)
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing score calculators: %w", err)
	}
	next := make(map[string]struct{}, len(found))
	for _, name := range found {
		next[name] = struct{}{}
	}
	return next, nil
}

// Purge deletes samples older than the retention window.
func (s *SQLiteSampleSink) Purge() error {
	if _, err := s.db.Exec(`DELETE FROM score_samples WHERE at < ?`, s.cutoff()); err != nil {
		return fmt.Errorf("purging score samples: %w", err)
	}
	s.invalidateNames()
	return nil
}

func (s *SQLiteSampleSink) cutoff() int64 {
	return s.now().UTC().Add(-s.retention).UnixNano()
}

func (s *SQLiteSampleSink) loop() {
	defer close(s.done)
	ticker := time.NewTicker(sampleFlushEvery)
	defer ticker.Stop()
	var batch []sampleWrite
	var pending error
	for {
		select {
		case req, ok := <-s.ch:
			if !ok {
				_ = s.writeBatch(batch)
				return
			}
			if req.done != nil {
				err := s.finishBatch(&batch, &pending)
				req.done <- err
				continue
			}
			batch = append(batch, req)
			if len(batch) >= sampleBatchSize {
				if err := s.writeBatch(batch); err != nil {
					pending = err
				}
				batch = nil
			}
		case <-ticker.C:
			if err := s.writeBatch(batch); err != nil {
				pending = err
			}
			batch = nil
		}
	}
}

func (s *SQLiteSampleSink) finishBatch(batch *[]sampleWrite, pending *error) error {
	err := s.writeBatch(*batch)
	*batch = nil
	if err == nil {
		err = *pending
	}
	*pending = nil
	return err
}

func (s *SQLiteSampleSink) writeBatch(batch []sampleWrite) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("[score-samples] begin: %v", err)
		return fmt.Errorf("recording score samples: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO score_samples (calculator, symbol, tf, score, at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		log.Printf("[score-samples] prepare: %v", err)
		return fmt.Errorf("recording score samples: %w", err)
	}
	defer stmt.Close()
	wrote := 0
	for _, row := range batch {
		_, err := stmt.Exec(row.calculator, row.symbol, row.tf, row.score, row.at)
		if err == nil {
			wrote++
			continue
		}
		if statementSkip(err) && transactionStillOpen(tx) {
			s.logLimited("insert skipped: " + err.Error())
			continue
		}
		_ = tx.Rollback()
		s.logLimited("insert batch aborted: " + err.Error())
		return fmt.Errorf("recording score samples: %w", err)
	}
	if wrote == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("recording score samples: no rows committed")
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		log.Printf("[score-samples] commit: %v", err)
		return fmt.Errorf("recording score samples: %w", err)
	}
	s.invalidateNames()
	return nil
}

// statementSkip is a constraint failure. RAISE(ABORT) and RAISE(ROLLBACK)
// share that code, so the caller also checks that the transaction is still open.
func statementSkip(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	return se.Code()&0xff == sqliteConstraint
}

// transactionStillOpen reports whether tx still has a SQLite transaction.
//
// modernc.org/sqlite returns SQLITE_CONSTRAINT for both RAISE(ABORT) and
// RAISE(ROLLBACK). ABORT leaves the transaction active. ROLLBACK ends it, and
// the next statement on that connection then autocommits.
//
// SQLite rejects BEGIN while a transaction is already active and accepts BEGIN
// when none is. Tx.Exec sends that SQL on the connection database/sql already
// reserved for this Tx; it does not start another Go transaction. When BEGIN
// is accepted, the following ROLLBACK ends only that probe transaction. The
// caller's later Rollback then runs against a transaction SQLite has already
// finished, which is a no-op error the caller ignores.
func transactionStillOpen(tx *sql.Tx) bool {
	_, err := tx.Exec(`BEGIN`)
	if err != nil {
		return true
	}
	_, _ = tx.Exec(`ROLLBACK`)
	return false
}

func (s *SQLiteSampleSink) logLimited(msg string) {
	now := time.Now().UnixNano()
	prev := s.lastSkip.Load()
	if now-prev < int64(time.Second) {
		return
	}
	if s.lastSkip.CompareAndSwap(prev, now) {
		log.Printf("[score-samples] %s", msg)
	}
}

func (s *SQLiteSampleSink) logDrop() {
	now := time.Now().UnixNano()
	prev := s.lastDrop.Load()
	if now-prev < int64(time.Second) {
		return
	}
	if s.lastDrop.CompareAndSwap(prev, now) {
		log.Printf("[score-samples] sample queue full; dropping samples")
	}
}

func (s *SQLiteSampleSink) migrate() error {
	ddl := `CREATE TABLE IF NOT EXISTS score_samples (
		calculator TEXT NOT NULL,
		symbol TEXT NOT NULL,
		tf TEXT NOT NULL,
		score REAL NOT NULL,
		at INTEGER NOT NULL
	)`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("creating score_samples: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_score_samples_calc_tf_at ON score_samples(calculator, tf, at)`); err != nil {
		return fmt.Errorf("creating score_samples index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_score_samples_at ON score_samples(at)`); err != nil {
		return fmt.Errorf("creating score_samples at index: %w", err)
	}
	return nil
}

func scanScores(rows *sql.Rows) ([]float64, error) {
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var score float64
		if err := rows.Scan(&score); err != nil {
			return nil, fmt.Errorf("scanning score sample: %w", err)
		}
		out = append(out, score)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading score samples: %w", err)
	}
	return out, nil
}
