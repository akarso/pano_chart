package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	domainsignal "pano_chart/backend/domain/signal"
)

// SQLiteRepository persists signals and outcomes.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) the signal DB and migrates.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening signal db: %w", err)
	}
	// modernc.org/sqlite: concurrent writers across pooled connections cause
	// intermittent busy errors (same rationale as payment sqlite).
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling foreign_keys: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("signal migrations: %w", err)
	}
	return repo, nil
}

// NewSQLiteRepositoryFromDB wraps an existing *sql.DB (tests).
func NewSQLiteRepositoryFromDB(db *sql.DB) (*SQLiteRepository, error) {
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign_keys: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("signal migrations: %w", err)
	}
	return repo, nil
}

// Close closes the underlying DB. Nil-safe.
func (r *SQLiteRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLiteRepository) migrate() error {
	signals := `CREATE TABLE IF NOT EXISTS signals (
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
	)`
	if _, err := r.db.Exec(signals); err != nil {
		return fmt.Errorf("creating signals: %w", err)
	}
	outcomes := `CREATE TABLE IF NOT EXISTS outcomes (
		signal_id TEXT PRIMARY KEY,
		resolved_at TEXT NOT NULL,
		forward_return REAL NOT NULL,
		max_favorable REAL NOT NULL,
		max_adverse REAL NOT NULL,
		success INTEGER NOT NULL,
		rule TEXT NOT NULL,
		FOREIGN KEY(signal_id) REFERENCES signals(id)
	)`
	if _, err := r.db.Exec(outcomes); err != nil {
		return fmt.Errorf("creating outcomes: %w", err)
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_signals_emitted_at ON signals(emitted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_signals_kind_label ON signals(kind, label)`,
		`CREATE INDEX IF NOT EXISTS idx_signals_symbol_tf ON signals(symbol, timeframe)`,
	} {
		if _, err := r.db.Exec(ddl); err != nil {
			return fmt.Errorf("creating signals index: %w", err)
		}
	}
	return nil
}

// Append inserts a signal row.
func (r *SQLiteRepository) Append(ctx context.Context, s domainsignal.Signal) error {
	ctxJSON, err := json.Marshal(s.Context)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	if ctxJSON == nil {
		ctxJSON = []byte("{}")
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO signals
		 (id, kind, symbol, timeframe, label, score, price, atr, context, emitted_at, horizon_bars)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, string(s.Kind), s.Symbol, s.Timeframe, s.Label,
		s.Score, s.Price, s.ATR, string(ctxJSON),
		s.EmittedAt.UTC().Format(time.RFC3339Nano), s.HorizonBars,
	)
	if err != nil {
		return fmt.Errorf("insert signal: %w", err)
	}
	return nil
}

// Unresolved returns signals emitted before `before` with no outcome row.
func (r *SQLiteRepository) Unresolved(ctx context.Context, before time.Time, limit int) ([]domainsignal.Signal, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT s.id, s.kind, s.symbol, s.timeframe, s.label, s.score, s.price, s.atr,
		        s.context, s.emitted_at, s.horizon_bars
		 FROM signals s
		 LEFT JOIN outcomes o ON o.signal_id = s.id
		 WHERE o.signal_id IS NULL AND s.emitted_at < ?
		 ORDER BY s.emitted_at ASC
		 LIMIT ?`,
		before.UTC().Format(time.RFC3339Nano), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query unresolved: %w", err)
	}
	defer rows.Close()
	return scanSignals(rows)
}

// MarkResolved upserts an outcome for a signal.
// Prefer outcome.SignalID when set; otherwise use id. Both must agree if both set.
func (r *SQLiteRepository) MarkResolved(ctx context.Context, id string, outcome domainsignal.Outcome) error {
	signalID := id
	if outcome.SignalID != "" {
		if id != "" && id != outcome.SignalID {
			return fmt.Errorf("mark resolved: id %q != outcome.SignalID %q", id, outcome.SignalID)
		}
		signalID = outcome.SignalID
	}
	if signalID == "" {
		return fmt.Errorf("mark resolved: empty signal id")
	}
	success := 0
	if outcome.Success {
		success = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO outcomes
		 (signal_id, resolved_at, forward_return, max_favorable, max_adverse, success, rule)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(signal_id) DO UPDATE SET
		   resolved_at=excluded.resolved_at,
		   forward_return=excluded.forward_return,
		   max_favorable=excluded.max_favorable,
		   max_adverse=excluded.max_adverse,
		   success=excluded.success,
		   rule=excluded.rule`,
		signalID,
		outcome.ResolvedAt.UTC().Format(time.RFC3339Nano),
		outcome.ForwardReturn, outcome.MaxFavorable, outcome.MaxAdverse,
		success, outcome.Rule,
	)
	if err != nil {
		return fmt.Errorf("mark resolved: %w", err)
	}
	return nil
}

// Query returns signals matching filter, with outcomes when present.
func (r *SQLiteRepository) Query(ctx context.Context, filter domainsignal.Filter) ([]domainsignal.SignalWithOutcome, error) {
	var (
		conds []string
		args  []interface{}
	)
	if filter.Kind != "" {
		conds = append(conds, "s.kind = ?")
		args = append(args, string(filter.Kind))
	}
	if filter.Label != "" {
		conds = append(conds, "s.label = ?")
		args = append(args, filter.Label)
	}
	if filter.Timeframe != "" {
		conds = append(conds, "s.timeframe = ?")
		args = append(args, filter.Timeframe)
	}
	if filter.Symbol != "" {
		conds = append(conds, "s.symbol = ?")
		args = append(args, filter.Symbol)
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "s.emitted_at >= ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "s.emitted_at < ?")
		args = append(args, filter.Until.UTC().Format(time.RFC3339Nano))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 500
	}
	args = append(args, limit)

	q := fmt.Sprintf(`SELECT s.id, s.kind, s.symbol, s.timeframe, s.label, s.score, s.price, s.atr,
		 s.context, s.emitted_at, s.horizon_bars,
		 o.signal_id, o.resolved_at, o.forward_return, o.max_favorable, o.max_adverse, o.success, o.rule
		 FROM signals s
		 LEFT JOIN outcomes o ON o.signal_id = s.id
		 %s
		 ORDER BY s.emitted_at DESC
		 LIMIT ?`, where)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query signals: %w", err)
	}
	defer rows.Close()

	var out []domainsignal.SignalWithOutcome
	for rows.Next() {
		var (
			s                      domainsignal.Signal
			kind, ctxJSON, emitted string
			outID, resolved, rule  sql.NullString
			fwd, fav, adv          sql.NullFloat64
			success                sql.NullInt64
		)
		if err := rows.Scan(
			&s.ID, &kind, &s.Symbol, &s.Timeframe, &s.Label, &s.Score, &s.Price, &s.ATR,
			&ctxJSON, &emitted, &s.HorizonBars,
			&outID, &resolved, &fwd, &fav, &adv, &success, &rule,
		); err != nil {
			return nil, fmt.Errorf("scan signal+outcome: %w", err)
		}
		s.Kind = domainsignal.Kind(kind)
		s.EmittedAt, _ = time.Parse(time.RFC3339Nano, emitted)
		_ = json.Unmarshal([]byte(ctxJSON), &s.Context)
		row := domainsignal.SignalWithOutcome{Signal: s}
		if outID.Valid {
			oc := domainsignal.Outcome{
				SignalID:      outID.String,
				ForwardReturn: fwd.Float64,
				MaxFavorable:  fav.Float64,
				MaxAdverse:    adv.Float64,
				Success:       success.Int64 != 0,
				Rule:          rule.String,
			}
			if resolved.Valid {
				oc.ResolvedAt, _ = time.Parse(time.RFC3339Nano, resolved.String)
			}
			row.Outcome = &oc
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanSignals(rows *sql.Rows) ([]domainsignal.Signal, error) {
	var out []domainsignal.Signal
	for rows.Next() {
		var (
			s                      domainsignal.Signal
			kind, ctxJSON, emitted string
		)
		if err := rows.Scan(
			&s.ID, &kind, &s.Symbol, &s.Timeframe, &s.Label, &s.Score, &s.Price, &s.ATR,
			&ctxJSON, &emitted, &s.HorizonBars,
		); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		s.Kind = domainsignal.Kind(kind)
		s.EmittedAt, _ = time.Parse(time.RFC3339Nano, emitted)
		_ = json.Unmarshal([]byte(ctxJSON), &s.Context)
		out = append(out, s)
	}
	return out, rows.Err()
}
