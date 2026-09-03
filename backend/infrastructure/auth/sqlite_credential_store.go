package auth

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"pano_chart/backend/application/ports"
)

// Compile-time check.
var _ ports.CredentialStore = (*SQLiteCredentialStore)(nil)

// SQLiteCredentialStore implements ports.CredentialStore backed by SQLite.
type SQLiteCredentialStore struct {
	db *sql.DB
}

// NewSQLiteCredentialStore wraps an existing *sql.DB (shared with other
// device-related stores — see cmd/api/main.go) and runs the
// device_credentials migration.
func NewSQLiteCredentialStore(db *sql.DB) (*SQLiteCredentialStore, error) {
	s := &SQLiteCredentialStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate device_credentials: %w", err)
	}
	return s, nil
}

func (s *SQLiteCredentialStore) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS device_credentials (
		secret_hash TEXT PRIMARY KEY,
		user_id     TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create device_credentials: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_device_credentials_user
		ON device_credentials(user_id)`); err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}

// SaveIfUserUnclaimed atomically saves a new secret hash bound to userID,
// but only if userID has no existing credential. The INSERT...SELECT...
// WHERE NOT EXISTS form makes the check-and-set a single statement — SQLite
// evaluates and executes it atomically, so two concurrent calls for the
// same userID cannot both succeed (unlike a separate "check, then insert"
// pair, which races).
func (s *SQLiteCredentialStore) SaveIfUserUnclaimed(ctx context.Context, secretHash, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO device_credentials (secret_hash, user_id)
		 SELECT ?, ?
		 WHERE NOT EXISTS (SELECT 1 FROM device_credentials WHERE user_id = ?)`,
		secretHash, userID, userID,
	)
	if err != nil {
		return false, fmt.Errorf("saving device credential: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("saving device credential: %w", err)
	}
	return affected > 0, nil
}

// Lookup resolves a secret hash to its bound user ID.
func (s *SQLiteCredentialStore) Lookup(ctx context.Context, secretHash string) (string, bool, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM device_credentials WHERE secret_hash = ?`,
		secretHash,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("looking up device credential: %w", err)
	}
	return userID, true, nil
}
