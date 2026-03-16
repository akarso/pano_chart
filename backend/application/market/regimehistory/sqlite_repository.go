package regimehistory

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	mkt "pano_chart/backend/domain/market"
)

// SQLiteRepository implements Repository backed by a SQLite database.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) an SQLite database and runs migrations.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening regime history db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running regime history migrations: %w", err)
	}
	return repo, nil
}

// NewSQLiteRepositoryFromDB wraps an existing *sql.DB (useful for tests).
func NewSQLiteRepositoryFromDB(db *sql.DB) (*SQLiteRepository, error) {
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("running regime history migrations: %w", err)
	}
	return repo, nil
}

// Close closes the underlying database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) migrate() error {
	stmt := `CREATE TABLE IF NOT EXISTS regime_periods (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timeframe TEXT NOT NULL,
		regime TEXT NOT NULL,
		start_ts INTEGER NOT NULL,
		end_ts INTEGER,
		duration_candles INTEGER NOT NULL DEFAULT 1
	)`
	if _, err := r.db.Exec(stmt); err != nil {
		return fmt.Errorf("creating regime_periods table: %w", err)
	}

	idx := `CREATE INDEX IF NOT EXISTS idx_regime_periods_tf ON regime_periods(timeframe, id)`
	if _, err := r.db.Exec(idx); err != nil {
		return fmt.Errorf("creating index: %w", err)
	}
	return nil
}

// GetLatest returns the most recent period for a timeframe.
func (r *SQLiteRepository) GetLatest(timeframe string) (*mkt.RegimePeriod, error) {
	row := r.db.QueryRow(
		`SELECT regime, start_ts, end_ts, duration_candles
		 FROM regime_periods
		 WHERE timeframe = ?
		 ORDER BY id DESC LIMIT 1`,
		timeframe,
	)

	var regime string
	var startTS int64
	var endTS sql.NullInt64
	var dur int

	err := row.Scan(&regime, &startTS, &endTS, &dur)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning latest period: %w", err)
	}

	p := &mkt.RegimePeriod{
		Regime:          mkt.Regime(regime),
		StartTimestamp:  startTS,
		DurationCandles: dur,
	}
	if endTS.Valid {
		v := endTS.Int64
		p.EndTimestamp = &v
	}
	return p, nil
}

// Append inserts a new open regime period.
func (r *SQLiteRepository) Append(timeframe string, period mkt.RegimePeriod) error {
	var endTS *int64
	if period.EndTimestamp != nil {
		v := *period.EndTimestamp
		endTS = &v
	}
	_, err := r.db.Exec(
		`INSERT INTO regime_periods (timeframe, regime, start_ts, end_ts, duration_candles)
		 VALUES (?, ?, ?, ?, ?)`,
		timeframe, string(period.Regime), period.StartTimestamp, endTS, period.DurationCandles,
	)
	if err != nil {
		return fmt.Errorf("appending period: %w", err)
	}
	return nil
}

// CloseCurrent sets the end timestamp on the currently open period.
func (r *SQLiteRepository) CloseCurrent(timeframe string, endTimestamp int64) error {
	_, err := r.db.Exec(
		`UPDATE regime_periods SET end_ts = ?
		 WHERE id = (
		   SELECT id FROM regime_periods
		   WHERE timeframe = ? AND end_ts IS NULL
		   ORDER BY id DESC LIMIT 1
		 )`,
		endTimestamp, timeframe,
	)
	if err != nil {
		return fmt.Errorf("closing current period: %w", err)
	}
	return nil
}

// UpdateDuration increments the duration on the most recent period.
func (r *SQLiteRepository) UpdateDuration(timeframe string, newDuration int) error {
	_, err := r.db.Exec(
		`UPDATE regime_periods SET duration_candles = ?
		 WHERE id = (
		   SELECT id FROM regime_periods
		   WHERE timeframe = ? AND end_ts IS NULL
		   ORDER BY id DESC LIMIT 1
		 )`,
		newDuration, timeframe,
	)
	if err != nil {
		return fmt.Errorf("updating duration: %w", err)
	}
	return nil
}

// GetHistory returns the most recent `limit` periods ordered oldest-first.
func (r *SQLiteRepository) GetHistory(timeframe string, limit int) ([]mkt.RegimePeriod, error) {
	rows, err := r.db.Query(
		`SELECT regime, start_ts, end_ts, duration_candles
		 FROM (
		   SELECT regime, start_ts, end_ts, duration_candles
		   FROM regime_periods
		   WHERE timeframe = ?
		   ORDER BY id DESC
		   LIMIT ?
		 ) sub ORDER BY start_ts ASC`,
		timeframe, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var periods []mkt.RegimePeriod
	for rows.Next() {
		var regime string
		var startTS int64
		var endTS sql.NullInt64
		var dur int

		if err := rows.Scan(&regime, &startTS, &endTS, &dur); err != nil {
			return nil, fmt.Errorf("scanning period: %w", err)
		}

		p := mkt.RegimePeriod{
			Regime:          mkt.Regime(regime),
			StartTimestamp:  startTS,
			DurationCandles: dur,
		}
		if endTS.Valid {
			v := endTS.Int64
			p.EndTimestamp = &v
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}
