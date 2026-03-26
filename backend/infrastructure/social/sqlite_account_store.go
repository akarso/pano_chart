package social

import (
	"database/sql"
	"fmt"
	"time"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
)

// Compile-time check.
var _ appsocial.AccountStore = (*SQLiteAccountStore)(nil)

// SQLiteAccountStore implements AccountStore backed by SQLite.
type SQLiteAccountStore struct {
	db *sql.DB
}

// NewSQLiteAccountStore opens (or reuses) a SQLite database and runs
// the accounts migration.
func NewSQLiteAccountStore(dbPath string) (*SQLiteAccountStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL: %w", err)
	}
	s := &SQLiteAccountStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// NewSQLiteAccountStoreFromDB wraps an existing *sql.DB (useful for tests).
func NewSQLiteAccountStoreFromDB(db *sql.DB) (*SQLiteAccountStore, error) {
	s := &SQLiteAccountStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *SQLiteAccountStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteAccountStore) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS social_accounts (
		id               TEXT PRIMARY KEY,
		platform         TEXT NOT NULL,
		handle           TEXT NOT NULL,
		last_seen_post   TEXT NOT NULL DEFAULT '',
		last_seen_ts     INTEGER NOT NULL DEFAULT 0,
		last_polled_at   INTEGER NOT NULL DEFAULT 0,
		last_used_at     INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	// Migration: add last_seen_ts if the table existed before this column.
	_, _ = s.db.Exec(`ALTER TABLE social_accounts ADD COLUMN last_seen_ts INTEGER NOT NULL DEFAULT 0`)
	return nil
}

func (s *SQLiteAccountStore) Upsert(account domain.Account) error {
	_, err := s.db.Exec(`INSERT INTO social_accounts (id, platform, handle, last_seen_post, last_seen_ts, last_polled_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_seen_post = excluded.last_seen_post,
			last_seen_ts   = excluded.last_seen_ts,
			last_polled_at = excluded.last_polled_at,
			last_used_at   = excluded.last_used_at`,
		account.ID, account.Platform, account.Handle,
		account.LastSeenPostID, account.LastSeenTimestamp, account.LastPolledAt, account.LastUsedAt,
	)
	return err
}

func (s *SQLiteAccountStore) Get(accountID string) (*domain.Account, error) {
	row := s.db.QueryRow(`SELECT id, platform, handle, last_seen_post, last_seen_ts, last_polled_at, last_used_at
		FROM social_accounts WHERE id = ?`, accountID)

	var acc domain.Account
	err := row.Scan(&acc.ID, &acc.Platform, &acc.Handle,
		&acc.LastSeenPostID, &acc.LastSeenTimestamp, &acc.LastPolledAt, &acc.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

func (s *SQLiteAccountStore) GetAllActive() ([]domain.Account, error) {
	rows, err := s.db.Query(`SELECT id, platform, handle, last_seen_post, last_seen_ts, last_polled_at, last_used_at
		FROM social_accounts`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []domain.Account
	for rows.Next() {
		var acc domain.Account
		if err := rows.Scan(&acc.ID, &acc.Platform, &acc.Handle,
			&acc.LastSeenPostID, &acc.LastSeenTimestamp, &acc.LastPolledAt, &acc.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, acc)
	}
	return result, rows.Err()
}

func (s *SQLiteAccountStore) MarkUsed(accountID string) error {
	_, err := s.db.Exec(`UPDATE social_accounts SET last_used_at = ? WHERE id = ?`,
		time.Now().Unix(), accountID)
	return err
}

func (s *SQLiteAccountStore) CleanupUnused(thresholdUnix int64) (int, error) {
	res, err := s.db.Exec(`DELETE FROM social_accounts WHERE last_used_at < ?`, thresholdUnix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
