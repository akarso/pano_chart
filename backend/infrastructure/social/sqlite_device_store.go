package social

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	appsocial "pano_chart/backend/application/social"
)

// Compile-time check.
var _ appsocial.DeviceTokenStore = (*SQLiteDeviceStore)(nil)

// SQLiteDeviceStore implements DeviceTokenStore backed by SQLite.
type SQLiteDeviceStore struct {
	db *sql.DB
}

// NewSQLiteDeviceStore opens (or reuses) a SQLite database and runs
// the device_tokens migration.
func NewSQLiteDeviceStore(dbPath string) (*SQLiteDeviceStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL: %w", err)
	}
	s := &SQLiteDeviceStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// NewSQLiteDeviceStoreFromDB wraps an existing *sql.DB (useful for tests).
func NewSQLiteDeviceStoreFromDB(db *sql.DB) (*SQLiteDeviceStore, error) {
	s := &SQLiteDeviceStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *SQLiteDeviceStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB so co-located stores can share it.
func (s *SQLiteDeviceStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteDeviceStore) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS device_tokens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    TEXT NOT NULL,
		device_id  TEXT NOT NULL UNIQUE,
		fcm_token  TEXT NOT NULL,
		platform   TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create device_tokens: %w", err)
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_device_tokens_user
		ON device_tokens(user_id)`)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}

// Register stores or updates a device's FCM token for a user. The upsert's
// WHERE guard makes the ownership check atomic with the write: if deviceID
// already belongs to a different user, the ON CONFLICT branch is skipped
// entirely (SQLite UPSERT semantics) rather than reassigning it.
func (s *SQLiteDeviceStore) Register(userID, deviceID, fcmToken, platform string) error {
	res, err := s.db.Exec(`INSERT INTO device_tokens (user_id, device_id, fcm_token, platform, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(device_id) DO UPDATE SET
			fcm_token  = excluded.fcm_token,
			platform   = excluded.platform,
			updated_at = datetime('now')
		WHERE device_tokens.user_id = excluded.user_id`,
		userID, deviceID, fcmToken, platform,
	)
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	if affected == 0 {
		// The conflict branch's WHERE guard rejected the write — the row
		// exists and belongs to someone else. Confirm before reporting
		// (defensive; this read isn't what makes the check race-free, the
		// WHERE guard above already is).
		var existingUser string
		lookupErr := s.db.QueryRow(`SELECT user_id FROM device_tokens WHERE device_id = ?`, deviceID).Scan(&existingUser)
		if lookupErr == nil && existingUser != userID {
			return appsocial.ErrDeviceOwnedByAnotherUser
		}
	}
	return nil
}

// Unregister removes a device token, scoped to the owning user.
func (s *SQLiteDeviceStore) Unregister(userID, deviceID string) error {
	_, err := s.db.Exec(`DELETE FROM device_tokens WHERE device_id = ? AND user_id = ?`, deviceID, userID)
	if err != nil {
		return fmt.Errorf("unregister device: %w", err)
	}
	return nil
}

// TokensForUsers returns all distinct FCM tokens for the given user IDs.
func (s *SQLiteDeviceStore) TokensForUsers(userIDs []string) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT DISTINCT fcm_token FROM device_tokens WHERE user_id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// AllTokens returns every registered FCM token.
func (s *SQLiteDeviceStore) AllTokens() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT fcm_token FROM device_tokens`)
	if err != nil {
		return nil, fmt.Errorf("query all tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}
