// Package watchlist persists each user's watchlisted symbols and the
// per-symbol regime-transition state used to dedupe watchlist alerts
// (ROADMAP PR-101).
package watchlist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appnotify "pano_chart/backend/application/notifications"
	"pano_chart/backend/application/ports"
	mkt "pano_chart/backend/domain/market"
)

// Compile-time checks.
var (
	_ ports.WatchlistStore          = (*SQLiteStore)(nil)
	_ appnotify.WatchlistProvider   = (*SQLiteStore)(nil)
	_ appnotify.WatchlistStateStore = (*SQLiteStore)(nil)
)

// SQLiteStore implements ports.WatchlistStore and the scheduler-facing
// WatchlistProvider / WatchlistStateStore backed by SQLite.
type SQLiteStore struct {
	db  *sql.DB
	now func() time.Time
}

// NewSQLiteStore creates or opens the watchlist tables on db (a shared
// connection, matching infrastructure/notifications.SQLiteConfigStore).
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db, now: time.Now}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate watchlist: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS watchlist (
		user_id  TEXT NOT NULL,
		symbol   TEXT NOT NULL,
		added_at TEXT NOT NULL,
		PRIMARY KEY (user_id, symbol)
	)`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS watchlist_state (
		user_id     TEXT NOT NULL,
		symbol      TEXT NOT NULL,
		timeframe   TEXT NOT NULL,
		state       TEXT NOT NULL,
		notified_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, symbol, timeframe)
	)`)
	return err
}

// ---------- ports.WatchlistStore / appnotify.WatchlistProvider ----------

// Get returns userID's watchlisted symbols, oldest-added first.
func (s *SQLiteStore) Get(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol FROM watchlist WHERE user_id = ? ORDER BY added_at ASC, symbol ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query watchlist: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var symbols []string
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, fmt.Errorf("scan watchlist symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

// Users returns every user ID with at least one watchlisted symbol.
func (s *SQLiteStore) Users(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM watchlist`)
	if err != nil {
		return nil, fmt.Errorf("query watchlist users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan watchlist user: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

// Replace overwrites userID's entire watchlist with symbols. A symbol
// already present keeps its original added_at (via INSERT ... ON CONFLICT
// DO NOTHING after pruning only what's no longer in symbols) — a full PUT
// resync must not make every remaining symbol look freshly added.
//
// De-duplicates symbols before the WatchlistMaxSymbols cap check (PR-101
// CR) — the caller (WatchlistHandler) already normalizes and de-dupes, but
// the cap is this store's own invariant ("50 distinct symbols"), so it
// must hold for any caller, not only one that happens to pre-dedupe.
// Without this, N duplicate entries of the same symbol would count as N
// toward the cap instead of 1.
//
// Also clears watchlist_state for every symbol this call drops (PR-101
// CR) — see clearStateFor's doc for why leaving it behind is a bug.
func (s *SQLiteStore) Replace(ctx context.Context, userID string, symbols []string) error {
	symbols = dedupe(symbols)
	if len(symbols) > ports.WatchlistMaxSymbols {
		return ports.ErrWatchlistTooLarge
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin watchlist replace: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := querySymbolsTx(ctx, tx, userID)
	if err != nil {
		return fmt.Errorf("read existing watchlist: %w", err)
	}
	keep := make(map[string]struct{}, len(symbols))
	for _, sym := range symbols {
		keep[sym] = struct{}{}
	}
	var removed []string
	for _, sym := range existing {
		if _, ok := keep[sym]; !ok {
			removed = append(removed, sym)
		}
	}

	if len(symbols) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("clear watchlist: %w", err)
		}
	} else {
		deleteQuery := fmt.Sprintf(
			`DELETE FROM watchlist WHERE user_id = ? AND symbol NOT IN (%s)`,
			placeholders(len(symbols)),
		)
		if _, err := tx.ExecContext(ctx, deleteQuery, args(userID, symbols)...); err != nil {
			return fmt.Errorf("prune watchlist: %w", err)
		}

		addedAt := s.now().UTC().Format(time.RFC3339Nano)
		for _, sym := range symbols {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO watchlist (user_id, symbol, added_at) VALUES (?, ?, ?)
				 ON CONFLICT(user_id, symbol) DO NOTHING`,
				userID, sym, addedAt,
			); err != nil {
				return fmt.Errorf("insert watchlist symbol %q: %w", sym, err)
			}
		}
	}

	if err := clearStateFor(ctx, tx, userID, removed); err != nil {
		return err
	}

	return tx.Commit()
}

// Remove deletes the given symbols from userID's watchlist, if present,
// and clears their watchlist_state (PR-101 CR) — see clearStateFor's doc.
func (s *SQLiteStore) Remove(ctx context.Context, userID string, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin watchlist remove: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := fmt.Sprintf(
		`DELETE FROM watchlist WHERE user_id = ? AND symbol IN (%s)`,
		placeholders(len(symbols)),
	)
	if _, err := tx.ExecContext(ctx, query, args(userID, symbols)...); err != nil {
		return fmt.Errorf("remove watchlist symbols: %w", err)
	}

	if err := clearStateFor(ctx, tx, userID, symbols); err != nil {
		return err
	}

	return tx.Commit()
}

// querySymbolsTx reads userID's current watchlist within tx (used by
// Replace to compute which symbols a resync is about to drop).
func querySymbolsTx(ctx context.Context, tx *sql.Tx, userID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT symbol FROM watchlist WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var symbols []string
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

// clearStateFor deletes watchlist_state rows for (userID, each of symbols)
// across every timeframe (PR-101 CR). Unwatching a symbol and later
// re-adding it must start fresh: without this, a stale State would let the
// very first post-re-add scan report a "transition" that isn't real (it
// never observed the symbol going quiet in between), and a stale
// NotifiedAt would wrongly suppress a genuinely new alert as if it were
// still inside the original dedup window from before the symbol was ever
// removed.
func clearStateFor(ctx context.Context, tx *sql.Tx, userID string, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	query := fmt.Sprintf(
		`DELETE FROM watchlist_state WHERE user_id = ? AND symbol IN (%s)`,
		placeholders(len(symbols)),
	)
	if _, err := tx.ExecContext(ctx, query, args(userID, symbols)...); err != nil {
		return fmt.Errorf("clear watchlist state: %w", err)
	}
	return nil
}

// dedupe removes duplicate entries, preserving first-seen order.
func dedupe(symbols []string) []string {
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func placeholders(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ", ")
}

func args(userID string, symbols []string) []any {
	out := make([]any, 0, len(symbols)+1)
	out = append(out, userID)
	for _, sym := range symbols {
		out = append(out, sym)
	}
	return out
}

// ---------- appnotify.WatchlistStateStore ----------

// GetState returns the last-observed transition state for
// (userID, symbol, timeframe), or found=false if none is stored yet.
func (s *SQLiteStore) GetState(ctx context.Context, userID, symbol, timeframe string) (appnotify.WatchlistTransitionState, bool, error) {
	var stateStr, notifiedAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT state, notified_at FROM watchlist_state WHERE user_id = ? AND symbol = ? AND timeframe = ?`,
		userID, symbol, timeframe,
	).Scan(&stateStr, &notifiedAtStr)
	if err == sql.ErrNoRows {
		return appnotify.WatchlistTransitionState{}, false, nil
	}
	if err != nil {
		return appnotify.WatchlistTransitionState{}, false, fmt.Errorf("query watchlist state: %w", err)
	}

	var notifiedAt time.Time
	if notifiedAtStr != "" {
		notifiedAt, err = time.Parse(time.RFC3339Nano, notifiedAtStr)
		if err != nil {
			return appnotify.WatchlistTransitionState{}, false, fmt.Errorf("parse watchlist state notified_at: %w", err)
		}
	}

	return appnotify.WatchlistTransitionState{
		State:      mkt.State(stateStr),
		NotifiedAt: notifiedAt,
	}, true, nil
}

// LastNotifiedAt returns the most recent NotifiedAt recorded for
// (userID, symbol) across every timeframe, or the zero Time if never
// notified under any of them.
//
// Compares parsed time.Time values in Go rather than MAX()-ing the raw
// notified_at strings in SQL: time.RFC3339Nano trims trailing zero
// fractional digits, so stored timestamps are not fixed-width and are not
// safely comparable as strings (e.g. "...09.5Z" sorts after "...10Z"
// lexicographically, the wrong way round). The row count here is small —
// at most one per canonical MTF timeframe — so fetching all of them and
// comparing properly-parsed values in Go is simple and unambiguously
// correct, without depending on a particular text encoding's sort order.
func (s *SQLiteStore) LastNotifiedAt(ctx context.Context, userID, symbol string) (time.Time, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT notified_at FROM watchlist_state WHERE user_id = ? AND symbol = ?`,
		userID, symbol,
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("query watchlist last-notified: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var latest time.Time
	for rows.Next() {
		var notifiedAtStr string
		if err := rows.Scan(&notifiedAtStr); err != nil {
			return time.Time{}, fmt.Errorf("scan watchlist last-notified: %w", err)
		}
		if notifiedAtStr == "" {
			continue
		}
		notifiedAt, err := time.Parse(time.RFC3339Nano, notifiedAtStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse watchlist last-notified: %w", err)
		}
		if notifiedAt.After(latest) {
			latest = notifiedAt
		}
	}
	return latest, rows.Err()
}

// SetState persists the transition state for (userID, symbol, timeframe).
func (s *SQLiteStore) SetState(ctx context.Context, userID, symbol, timeframe string, state appnotify.WatchlistTransitionState) error {
	var notifiedAtStr string
	if !state.NotifiedAt.IsZero() {
		notifiedAtStr = state.NotifiedAt.UTC().Format(time.RFC3339Nano)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO watchlist_state (user_id, symbol, timeframe, state, notified_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, symbol, timeframe) DO UPDATE SET
			state = excluded.state,
			notified_at = excluded.notified_at`,
		userID, symbol, timeframe, string(state.State), notifiedAtStr,
	)
	if err != nil {
		return fmt.Errorf("save watchlist state: %w", err)
	}
	return nil
}
