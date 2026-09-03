package volatility

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// CandleCache persists fetched 1-minute candles in SQLite so re-runs
// skip already-downloaded data.
type CandleCache struct {
	db *sql.DB
}

// NewCandleCache opens (or creates) a SQLite database at dbPath and
// runs the candles migration.
func NewCandleCache(dbPath string) (*CandleCache, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL: %w", err)
	}
	c := &CandleCache{db: db}
	if err := c.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return c, nil
}

// NewCandleCacheFromDB wraps an existing *sql.DB (useful for tests).
func NewCandleCacheFromDB(db *sql.DB) (*CandleCache, error) {
	c := &CandleCache{db: db}
	if err := c.migrate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Close closes the underlying database.
func (c *CandleCache) Close() error {
	return c.db.Close()
}

func (c *CandleCache) migrate() error {
	_, err := c.db.Exec(`CREATE TABLE IF NOT EXISTS candles_1m (
		symbol    TEXT    NOT NULL,
		open_time INTEGER NOT NULL,
		open      REAL    NOT NULL,
		high      REAL    NOT NULL,
		low       REAL    NOT NULL,
		close     REAL    NOT NULL,
		PRIMARY KEY (symbol, open_time)
	)`)
	if err != nil {
		return fmt.Errorf("create candles_1m: %w", err)
	}
	return nil
}

// Store inserts candles, skipping duplicates.
func (c *CandleCache) Store(symbol string, candles []Candle) error {
	if len(candles) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO candles_1m
		(symbol, open_time, open, high, low, close)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, cd := range candles {
		if _, err := stmt.Exec(symbol, cd.OpenTime, cd.Open, cd.High, cd.Low, cd.Close); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec: %w", err)
		}
	}
	return tx.Commit()
}

// Load returns all cached candles for a symbol within [startTime, endTime],
// ordered by open_time ascending.
func (c *CandleCache) Load(symbol string, startTime, endTime int64) ([]Candle, error) {
	rows, err := c.db.Query(`SELECT open_time, open, high, low, close
		FROM candles_1m
		WHERE symbol = ? AND open_time >= ? AND open_time <= ?
		ORDER BY open_time`, symbol, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candles []Candle
	for rows.Next() {
		var cd Candle
		if err := rows.Scan(&cd.OpenTime, &cd.Open, &cd.High, &cd.Low, &cd.Close); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		candles = append(candles, cd)
	}
	return candles, rows.Err()
}

// MaxOpenTime returns the latest cached open_time for a symbol, or 0 if empty.
func (c *CandleCache) MaxOpenTime(symbol string) (int64, error) {
	var maxTime sql.NullInt64
	err := c.db.QueryRow(`SELECT MAX(open_time) FROM candles_1m WHERE symbol = ?`, symbol).Scan(&maxTime)
	if err != nil {
		return 0, fmt.Errorf("query max: %w", err)
	}
	if !maxTime.Valid {
		return 0, nil
	}
	return maxTime.Int64, nil
}

// Count returns the number of cached candles for a symbol.
func (c *CandleCache) Count(symbol string) (int, error) {
	var n int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM candles_1m WHERE symbol = ?`, symbol).Scan(&n)
	return n, err
}
