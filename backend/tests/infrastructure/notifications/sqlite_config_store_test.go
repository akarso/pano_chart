package notifications_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appnotify "pano_chart/backend/application/notifications"
	infranotify "pano_chart/backend/infrastructure/notifications"

	_ "modernc.org/sqlite"
)

func openConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	return db
}

// A config row saved before WatchlistTransitions existed (no such JSON key
// at all, config_version 2 — the pre-PR-101 schema) must migrate to
// WatchlistTransitions=true: the field simply didn't exist yet, so
// json.Unmarshal always zeroed it to false regardless of intent, silently
// opting every pre-existing user permanently out with no way to discover
// the feature exists (PR-101 CR).
func TestSQLiteConfigStore_PreMigrationRow_WatchlistTransitionsDefaultsTrue(t *testing.T) {
	db := openConfigTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infranotify.NewSQLiteConfigStore(db)
	require.NoError(t, err)

	legacyJSON := `{
		"config_version": 2,
		"social": true, "macro_high": true, "macro_moderate": true, "news": true,
		"uptrend": true, "downtrend": true, "sideways": true, "setup_of_day": true,
		"uptrend_min_dominance": 0.35, "downtrend_min_dominance": 0.35, "sideways_min_dominance": 0.35,
		"setup_min_score": 0.75,
		"uptrend_timeframe": "1h", "downtrend_timeframe": "1h", "sideways_timeframe": "1h", "setup_timeframe": "1h"
	}`
	_, err = db.Exec(
		`INSERT INTO notification_config (user_id, config, updated_at) VALUES (?, ?, datetime('now'))`,
		"legacy-user", legacyJSON,
	)
	require.NoError(t, err)

	cfg, err := store.Get("legacy-user")
	require.NoError(t, err)
	assert.True(t, cfg.WatchlistTransitions, "a pre-PR-101 config row must migrate to WatchlistTransitions=true")
	assert.Equal(t, "1h", cfg.WatchlistTimeframe, "must also backfill the new timeframe field")
}

// A config row with no config_version field at all (the oldest possible
// shape) must migrate the same way as an explicit config_version 2 — both
// are "before config_version 3" for this migration's purposes.
func TestSQLiteConfigStore_NoConfigVersionAtAll_WatchlistTransitionsDefaultsTrue(t *testing.T) {
	db := openConfigTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infranotify.NewSQLiteConfigStore(db)
	require.NoError(t, err)

	legacyJSON := `{"social": true, "uptrend": true}`
	_, err = db.Exec(
		`INSERT INTO notification_config (user_id, config, updated_at) VALUES (?, ?, datetime('now'))`,
		"ancient-user", legacyJSON,
	)
	require.NoError(t, err)

	cfg, err := store.Get("ancient-user")
	require.NoError(t, err)
	assert.True(t, cfg.WatchlistTransitions)
}

// Once a user has (re)saved their config through this version, their
// explicit choice — including an explicit false — must stick, never be
// silently re-forced back to true by the migration on a later read.
func TestSQLiteConfigStore_ExplicitFalseAfterMigration_Sticks(t *testing.T) {
	db := openConfigTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infranotify.NewSQLiteConfigStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Save(appnotify.NotificationConfig{
		UserID:               "user1",
		WatchlistTransitions: false,
		WatchlistTimeframe:   "4h",
	}))

	got, err := store.Get("user1")
	require.NoError(t, err)
	assert.False(t, got.WatchlistTransitions,
		"an explicit false saved at the current schema version must not be forced back to true")
	assert.Equal(t, "4h", got.WatchlistTimeframe)
}

// A brand-new user (no row at all) gets DefaultNotificationConfig, which
// also defaults WatchlistTransitions to true — covered here for parity
// with the migration tests above, confirming both paths agree.
func TestSQLiteConfigStore_NewUser_WatchlistTransitionsDefaultsTrue(t *testing.T) {
	db := openConfigTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infranotify.NewSQLiteConfigStore(db)
	require.NoError(t, err)

	cfg, err := store.Get("brand-new-user")
	require.NoError(t, err)
	assert.True(t, cfg.WatchlistTransitions)
	assert.Equal(t, "1h", cfg.WatchlistTimeframe)
}
