package auth_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infraauth "pano_chart/backend/infrastructure/auth"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLiteCredentialStore_SaveAndLookup(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infraauth.NewSQLiteCredentialStore(db)
	require.NoError(t, err)

	ok, err := store.SaveIfUserUnclaimed(context.Background(), "hash-abc", "user1")
	require.NoError(t, err)
	assert.True(t, ok)

	userID, found, err := store.Lookup(context.Background(), "hash-abc")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "user1", userID)
}

func TestSQLiteCredentialStore_Lookup_Unknown(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infraauth.NewSQLiteCredentialStore(db)
	require.NoError(t, err)

	_, ok, err := store.Lookup(context.Background(), "no-such-hash")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSQLiteCredentialStore_SaveIfUserUnclaimed_SecondClaimForSameUser_Rejected(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infraauth.NewSQLiteCredentialStore(db)
	require.NoError(t, err)

	ok, err := store.SaveIfUserUnclaimed(context.Background(), "hash-abc", "user1")
	require.NoError(t, err)
	assert.True(t, ok, "first claim for a fresh user id should succeed")

	// A different secret trying to claim the SAME user id — must be
	// rejected, not silently rebound (rebinding would let anyone who
	// learns user1's id hijack it later).
	ok, err = store.SaveIfUserUnclaimed(context.Background(), "hash-def", "user1")
	require.NoError(t, err)
	assert.False(t, ok, "second claim for an already-claimed user id must be rejected")

	// The original binding must be untouched.
	userID, found, err := store.Lookup(context.Background(), "hash-abc")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "user1", userID)

	_, found, err = store.Lookup(context.Background(), "hash-def")
	require.NoError(t, err)
	assert.False(t, found, "the rejected secret must not have been saved")
}

func TestSQLiteCredentialStore_SaveIfUserUnclaimed_ConcurrentClaims_OnlyOneWins(t *testing.T) {
	// Regression test for the TOCTOU gap: a separate "is it claimed?" read
	// followed by a write would let two concurrent requests for the same
	// userID both pass the check. SaveIfUserUnclaimed must close that race
	// via a single guarded statement, not application-level check-then-act.
	// Uses a shared-cache in-memory DB + multiple real connections so the
	// race is actually exercised, not serialized away by a single
	// connection.
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)

	store, err := infraauth.NewSQLiteCredentialStore(db)
	require.NoError(t, err)

	const attempts = 8
	oks := make([]bool, attempts)
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		i := i
		go func() {
			defer wg.Done()
			ok, err := store.SaveIfUserUnclaimed(
				context.Background(),
				fmt.Sprintf("hash-%d", i),
				"contested-user",
			)
			oks[i] = ok
			errs[i] = err
		}()
	}
	wg.Wait()

	wins := 0
	for i, ok := range oks {
		require.NoError(t, errs[i])
		if ok {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "exactly one concurrent claim for the same user id should succeed")
}

func TestNewSQLiteCredentialStore_SharesConnection(t *testing.T) {
	// Mirrors the production wiring (cmd/api/main.go), which reuses the
	// device token store's *sql.DB rather than opening a new file.
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS device_tokens (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)

	store, err := infraauth.NewSQLiteCredentialStore(db)
	require.NoError(t, err)

	ok, err := store.SaveIfUserUnclaimed(context.Background(), "hash-xyz", "user1")
	require.NoError(t, err)
	assert.True(t, ok)

	_, found, err := store.Lookup(context.Background(), "hash-xyz")
	require.NoError(t, err)
	assert.True(t, found)
}
