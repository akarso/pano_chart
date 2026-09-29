package watchlist_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appnotify "pano_chart/backend/application/notifications"
	"pano_chart/backend/application/ports"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/infrastructure/watchlist"

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

func TestSQLiteStore_Get_EmptyForNewUser(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Empty(t, symbols)
}

func TestSQLiteStore_Replace_ThenGet_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	err = store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"})
	require.NoError(t, err)

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, []string{"BTCUSDT", "ETHUSDT"}, symbols)
}

func TestSQLiteStore_Replace_OrdersByAddedAt(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT", "BTCUSDT"}))

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, []string{"ETHUSDT", "BTCUSDT"}, symbols,
		"ETHUSDT was added first and must stay first even though a later PUT included it again")
}

// A full PUT resync must not re-timestamp a symbol that was already on the
// list — only a genuinely new symbol gets "now" as its added_at. Verified
// indirectly via ordering: if re-adding reset ETHUSDT's added_at, it would
// sort after a symbol added afterwards instead of before it.
func TestSQLiteStore_Replace_PreservesOriginalAddedAtAcrossResync(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT"})) // resync, no new symbol
	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT", "SOLUSDT"}))

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, []string{"ETHUSDT", "SOLUSDT"}, symbols)
}

func TestSQLiteStore_Replace_DropsSymbolsNoLongerInTheList(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user1", []string{"ETHUSDT"}))

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, []string{"ETHUSDT"}, symbols)
}

func TestSQLiteStore_Replace_EmptyList_ClearsWatchlist(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user1", nil))

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Empty(t, symbols)
}

func TestSQLiteStore_Replace_TooManySymbols_ErrorsAndWritesNothing(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))

	tooMany := make([]string, ports.WatchlistMaxSymbols+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("SYM%d", i) // distinct — the cap counts unique symbols
	}
	err = store.Replace(context.Background(), "user1", tooMany)
	assert.ErrorIs(t, err, ports.ErrWatchlistTooLarge)

	// Nothing was written — the pre-existing watchlist survives untouched.
	symbols, getErr := store.Get(context.Background(), "user1")
	require.NoError(t, getErr)
	assert.Equal(t, []string{"BTCUSDT"}, symbols)
}

// A PUT with more raw entries than the cap, but few enough distinct
// symbols once de-duplicated, must succeed — the cap is "50 distinct
// symbols", not "50 request entries" (PR-101 CR).
func TestSQLiteStore_Replace_DuplicatesDontCountTowardCap(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	manyDuplicates := make([]string, ports.WatchlistMaxSymbols+10)
	for i := range manyDuplicates {
		manyDuplicates[i] = "BTCUSDT" // 60 raw entries, 1 distinct symbol
	}
	err = store.Replace(context.Background(), "user1", manyDuplicates)
	require.NoError(t, err)

	symbols, getErr := store.Get(context.Background(), "user1")
	require.NoError(t, getErr)
	assert.Equal(t, []string{"BTCUSDT"}, symbols)
}

func TestSQLiteStore_Remove_DeletesOnlyGivenSymbols(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}))
	require.NoError(t, store.Remove(context.Background(), "user1", []string{"ETHUSDT"}))

	symbols, err := store.Get(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, []string{"BTCUSDT", "SOLUSDT"}, symbols)
}

func TestSQLiteStore_Remove_UnknownSymbol_NoError(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	err = store.Remove(context.Background(), "user1", []string{"DOESNOTEXIST"})
	assert.NoError(t, err)

	symbols, getErr := store.Get(context.Background(), "user1")
	require.NoError(t, getErr)
	assert.Equal(t, []string{"BTCUSDT"}, symbols)
}

func TestSQLiteStore_PerUserIsolation(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user2", []string{"ETHUSDT"}))

	s1, _ := store.Get(context.Background(), "user1")
	s2, _ := store.Get(context.Background(), "user2")
	assert.Equal(t, []string{"BTCUSDT"}, s1)
	assert.Equal(t, []string{"ETHUSDT"}, s2)
}

// ── WatchlistStateStore (transition dedup state) ────────────────────────

func TestSQLiteStore_GetState_NotFoundForUnknownKey(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	_, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSQLiteStore_SetState_ThenGetState_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	notifiedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	want := appnotify.WatchlistTransitionState{State: mkt.StateExpansion, NotifiedAt: notifiedAt}
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h", want))

	got, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, mkt.StateExpansion, got.State)
	assert.True(t, notifiedAt.Equal(got.NotifiedAt))
}

func TestSQLiteStore_SetState_ZeroNotifiedAt_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	want := appnotify.WatchlistTransitionState{State: mkt.StateTrend}
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h", want))

	got, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, mkt.StateTrend, got.State)
	assert.True(t, got.NotifiedAt.IsZero())
}

func TestSQLiteStore_SetState_OverwritesPreviousValue(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateCompression}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateExpansion}))

	got, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, mkt.StateExpansion, got.State)
}

func TestSQLiteStore_State_ScopedPerTimeframe(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "4h",
		appnotify.WatchlistTransitionState{State: mkt.StateSideways}))

	got1h, _, _ := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	got4h, _, _ := store.GetState(context.Background(), "user1", "BTCUSDT", "4h")
	assert.Equal(t, mkt.StateTrend, got1h.State)
	assert.Equal(t, mkt.StateSideways, got4h.State)
}

// ── Users ────────────────────────────────────────────────────────────────

func TestSQLiteStore_Users_ReturnsDistinctUsersWithSymbols(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user2", []string{"SOLUSDT"}))

	users, err := store.Users(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user1", "user2"}, users)
}

func TestSQLiteStore_Users_EmptyWhenNoWatchlists(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	users, err := store.Users(context.Background())
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestSQLiteStore_Users_ExcludesUserAfterFullClear(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.Replace(context.Background(), "user1", nil))

	users, err := store.Users(context.Background())
	require.NoError(t, err)
	assert.Empty(t, users)
}

// ── LastNotifiedAt ───────────────────────────────────────────────────────

func TestSQLiteStore_LastNotifiedAt_ZeroWhenNeverNotified(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))

	got, err := store.LastNotifiedAt(context.Background(), "user1", "BTCUSDT")
	require.NoError(t, err)
	assert.True(t, got.IsZero())
}

// PR-101 CR: LastNotifiedAt must find a notification recorded under a
// DIFFERENT timeframe row for the same (user, symbol) — the dedup window
// is per-(user,symbol), not per-(user,symbol,timeframe).
func TestSQLiteStore_LastNotifiedAt_FindsAcrossTimeframes(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	older := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend, NotifiedAt: older}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "4h",
		appnotify.WatchlistTransitionState{State: mkt.StateSideways, NotifiedAt: newer}))

	got, err := store.LastNotifiedAt(context.Background(), "user1", "BTCUSDT")
	require.NoError(t, err)
	assert.True(t, newer.Equal(got), "expected the most recent NotifiedAt across all timeframe rows, got %v", got)
}

func TestSQLiteStore_LastNotifiedAt_ScopedPerUserAndSymbol(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	notifiedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend, NotifiedAt: notifiedAt}))

	otherSymbol, err := store.LastNotifiedAt(context.Background(), "user1", "ETHUSDT")
	require.NoError(t, err)
	assert.True(t, otherSymbol.IsZero())

	otherUser, err := store.LastNotifiedAt(context.Background(), "user2", "BTCUSDT")
	require.NoError(t, err)
	assert.True(t, otherUser.IsZero())
}

// ── watchlist_state cleanup on Remove / Replace ─────────────────────────

// PR-101 CR: unwatching a symbol must clear its transition state, so a
// later re-add starts fresh instead of inheriting a stale regime (a false
// "transition" on the very first post-re-add scan) or a stale NotifiedAt
// (wrongly suppressing a genuinely new alert).
func TestSQLiteStore_Remove_ClearsWatchlistState(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend, NotifiedAt: time.Now()}))

	require.NoError(t, store.Remove(context.Background(), "user1", []string{"BTCUSDT"}))

	_, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	assert.False(t, found, "watchlist_state must be cleared when a symbol is removed")
}

// Removing one symbol must not clear another symbol's transition state.
func TestSQLiteStore_Remove_OnlyClearsStateForRemovedSymbols(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"}))
	require.NoError(t, store.SetState(context.Background(), "user1", "ETHUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))

	require.NoError(t, store.Remove(context.Background(), "user1", []string{"BTCUSDT"}))

	_, found, err := store.GetState(context.Background(), "user1", "ETHUSDT", "1h")
	require.NoError(t, err)
	assert.True(t, found, "an untouched symbol's state must survive removing a different symbol")
}

// PR-101 CR: a full PUT resync that drops a symbol must clear its state,
// same as an explicit DELETE.
func TestSQLiteStore_Replace_ClearsStateForDroppedSymbols(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"}))
	require.NoError(t, store.SetState(context.Background(), "user1", "ETHUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))

	// Resync drops ETHUSDT.
	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))

	_, found, err := store.GetState(context.Background(), "user1", "ETHUSDT", "1h")
	require.NoError(t, err)
	assert.False(t, found, "watchlist_state must be cleared for a symbol dropped by a PUT resync")
}

// A symbol that survives a resync must keep its transition state.
func TestSQLiteStore_Replace_PreservesStateForRetainedSymbols(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT", "ETHUSDT"}))

	got, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, mkt.StateTrend, got.State)
}

// Clearing the whole watchlist (empty PUT) must clear all its state too.
func TestSQLiteStore_Replace_EmptyList_ClearsAllState(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store, err := watchlist.NewSQLiteStore(db)
	require.NoError(t, err)

	require.NoError(t, store.Replace(context.Background(), "user1", []string{"BTCUSDT"}))
	require.NoError(t, store.SetState(context.Background(), "user1", "BTCUSDT", "1h",
		appnotify.WatchlistTransitionState{State: mkt.StateTrend}))

	require.NoError(t, store.Replace(context.Background(), "user1", nil))

	_, found, err := store.GetState(context.Background(), "user1", "BTCUSDT", "1h")
	require.NoError(t, err)
	assert.False(t, found)
}
