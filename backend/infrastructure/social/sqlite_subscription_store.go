package social

import (
	"database/sql"
	"fmt"

	appsocial "pano_chart/backend/application/social"
)

// Compile-time check.
var _ appsocial.SubscriptionStore = (*SQLiteSubscriptionStore)(nil)

// SQLiteSubscriptionStore implements SubscriptionStore backed by SQLite.
type SQLiteSubscriptionStore struct {
	db *sql.DB
}

// NewSQLiteSubscriptionStore opens (or reuses) a SQLite database and runs
// the subscriptions migration.
func NewSQLiteSubscriptionStore(dbPath string) (*SQLiteSubscriptionStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL: %w", err)
	}
	s := &SQLiteSubscriptionStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// NewSQLiteSubscriptionStoreFromDB wraps an existing *sql.DB (useful for tests).
func NewSQLiteSubscriptionStoreFromDB(db *sql.DB) (*SQLiteSubscriptionStore, error) {
	s := &SQLiteSubscriptionStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *SQLiteSubscriptionStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteSubscriptionStore) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS social_subscriptions (
		user_id    TEXT NOT NULL,
		account_id TEXT NOT NULL,
		PRIMARY KEY (user_id, account_id)
	)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_subs_account
		ON social_subscriptions(account_id)`)
	return err
}

func (s *SQLiteSubscriptionStore) Subscribe(userID, accountID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO social_subscriptions (user_id, account_id)
		VALUES (?, ?)`, userID, accountID)
	return err
}

func (s *SQLiteSubscriptionStore) Unsubscribe(userID, accountID string) error {
	_, err := s.db.Exec(`DELETE FROM social_subscriptions
		WHERE user_id = ? AND account_id = ?`, userID, accountID)
	return err
}

func (s *SQLiteSubscriptionStore) AccountsForUser(userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT account_id FROM social_subscriptions
		WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLiteSubscriptionStore) HasSubscribers(accountID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM social_subscriptions
		WHERE account_id = ?`, accountID).Scan(&count)
	return count > 0, err
}

func (s *SQLiteSubscriptionStore) UsersForAccount(accountID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT user_id FROM social_subscriptions
		WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
