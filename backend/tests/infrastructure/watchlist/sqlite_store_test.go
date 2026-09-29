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
