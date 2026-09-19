package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"pano_chart/backend/domain"
)

// SQLiteRepository implements PurchaseRepository and SubscriptionRepository
// backed by a SQLite database.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) a SQLite database at the given
// path and runs schema migrations.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}
	// SQLite does not like concurrent writers across pooled connections
	// (and :memory: is per-connection). One open connection keeps the
	// schema visible and serializes ApplyVerifiedPurchase transfers.
	db.SetMaxOpenConns(1)
	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return repo, nil
}

// NewSQLiteRepositoryFromDB wraps an existing *sql.DB (useful for tests).
func NewSQLiteRepositoryFromDB(db *sql.DB) (*SQLiteRepository, error) {
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return repo, nil
}

// Close closes the underlying database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS purchases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			external_transaction_id TEXT NOT NULL,
			product_id TEXT NOT NULL,
			purchase_time DATETIME NOT NULL,
			expiration_time DATETIME NOT NULL,
			verified INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			UNIQUE(provider, external_transaction_id)
		)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			user_id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			product_id TEXT NOT NULL,
			start_time DATETIME NOT NULL,
			expiration_time DATETIME NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// ---- PurchaseRepository ----

// Save inserts a purchase record and returns its auto-generated ID.
func (r *SQLiteRepository) Save(ctx context.Context, p domain.Purchase) (int64, error) {
	// Ensure user exists (idempotent).
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO users (id) VALUES (?)`,
		p.UserID(),
	); err != nil {
		return 0, fmt.Errorf("ensuring user: %w", err)
	}

	verified := 0
	if p.Verified() {
		verified = 1
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO purchases
			(user_id, provider, external_transaction_id, product_id,
			 purchase_time, expiration_time, verified, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID(),
		p.Provider(),
		p.ExternalTransactionID(),
		p.ProductID(),
		p.PurchaseTime().Format(time.RFC3339),
		p.ExpirationTime().Format(time.RFC3339),
		verified,
		p.CreatedAt().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting purchase: %w", err)
	}
	return res.LastInsertId()
}

// FindByTransactionID looks up a purchase by provider + external transaction ID.
func (r *SQLiteRepository) FindByTransactionID(
	ctx context.Context,
	provider, externalTransactionID string,
) (domain.Purchase, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, provider, external_transaction_id, product_id,
		        purchase_time, expiration_time, verified, created_at
		 FROM purchases
		 WHERE provider = ? AND external_transaction_id = ?`,
		provider, externalTransactionID,
	)
	return r.scanPurchase(row)
}

// errStalePurchaseOwner means another writer moved the purchase between our
// SELECT of the current owner and the owner-checked UPDATE. The caller retries.
var errStalePurchaseOwner = fmt.Errorf("stale purchase owner")

// ApplyVerifiedPurchase updates purchase ownership + expiry, upserts the
// caller's subscription, and expires the holder observed inside the same
// write transaction when they differ — so concurrent rebinds cannot leave an
// intermediate destination entitled after a later transfer wins.
func (r *SQLiteRepository) ApplyVerifiedPurchase(
	ctx context.Context,
	result domain.PaymentVerificationResult,
) error {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := r.applyVerifiedPurchaseOnce(ctx, result)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errStalePurchaseOwner) {
			return err
		}
	}
	return fmt.Errorf("transferring purchase after concurrent rebinds: %w", errStalePurchaseOwner)
}

func (r *SQLiteRepository) applyVerifiedPurchaseOnce(
	ctx context.Context,
	result domain.PaymentVerificationResult,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO users (id) VALUES (?)`,
		result.UserID(),
	); err != nil {
		return fmt.Errorf("ensuring user: %w", err)
	}

	var currentOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM purchases
		 WHERE provider = ? AND external_transaction_id = ?`,
		result.Provider(),
		result.ExternalTransactionID(),
	).Scan(&currentOwner)
	if err == sql.ErrNoRows {
		return fmt.Errorf("purchase not found for %s/%s", result.Provider(), result.ExternalTransactionID())
	}
	if err != nil {
		return fmt.Errorf("reading purchase owner: %w", err)
	}

	// Owner-checked update: if another transfer committed after our SELECT,
	// RowsAffected is 0 and we retry with a fresh owner read.
	res, err := tx.ExecContext(ctx,
		`UPDATE purchases
		 SET user_id = ?, expiration_time = ?
		 WHERE provider = ? AND external_transaction_id = ? AND user_id = ?`,
		result.UserID(),
		result.ExpirationTime().Format(time.RFC3339),
		result.Provider(),
		result.ExternalTransactionID(),
		currentOwner,
	)
	if err != nil {
		return fmt.Errorf("updating purchase ownership: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errStalePurchaseOwner
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO subscriptions
			(user_id, provider, product_id, start_time, expiration_time, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
			provider = excluded.provider,
			product_id = excluded.product_id,
			start_time = excluded.start_time,
			expiration_time = excluded.expiration_time,
			updated_at = excluded.updated_at`,
		result.UserID(),
		result.Provider(),
		result.ProductID(),
		result.PurchaseTime().Format(time.RFC3339),
		result.ExpirationTime().Format(time.RFC3339),
		now.Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upserting subscription: %w", err)
	}

	if currentOwner != "" && currentOwner != result.UserID() {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO users (id) VALUES (?)`,
			currentOwner,
		); err != nil {
			return fmt.Errorf("ensuring previous user: %w", err)
		}
		// Expire the owner observed in this transaction (start=expiration=now).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO subscriptions
				(user_id, provider, product_id, start_time, expiration_time, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
				provider = excluded.provider,
				product_id = excluded.product_id,
				start_time = excluded.start_time,
				expiration_time = excluded.expiration_time,
				updated_at = excluded.updated_at`,
			currentOwner,
			result.Provider(),
			result.ProductID(),
			now.Format(time.RFC3339),
			now.Format(time.RFC3339),
			now.Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("expiring previous subscriber: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transfer tx: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) scanPurchase(row *sql.Row) (domain.Purchase, bool, error) {
	var (
		id            int64
		userID        string
		provider      string
		txID          string
		productID     string
		purchaseTimeS string
		expirationS   string
		verifiedInt   int
		createdAtS    string
	)
	err := row.Scan(&id, &userID, &provider, &txID, &productID,
		&purchaseTimeS, &expirationS, &verifiedInt, &createdAtS)
	if err == sql.ErrNoRows {
		return domain.Purchase{}, false, nil
	}
	if err != nil {
		return domain.Purchase{}, false, fmt.Errorf("scanning purchase: %w", err)
	}

	purchaseTime, _ := time.Parse(time.RFC3339, purchaseTimeS)
	expiration, _ := time.Parse(time.RFC3339, expirationS)
	createdAt, _ := time.Parse(time.RFC3339, createdAtS)

	p := domain.NewPurchaseUnsafe(
		id, userID, provider, txID, productID,
		purchaseTime, expiration, createdAt,
		verifiedInt == 1,
	)
	return p, true, nil
}

// ---- SubscriptionRepository ----

// Upsert creates or replaces the subscription for the user.
func (r *SQLiteRepository) Upsert(ctx context.Context, sub domain.Subscription) error {
	// Ensure user exists (idempotent).
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO users (id) VALUES (?)`,
		sub.UserID(),
	); err != nil {
		return fmt.Errorf("ensuring user: %w", err)
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions
			(user_id, provider, product_id, start_time, expiration_time, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
			provider = excluded.provider,
			product_id = excluded.product_id,
			start_time = excluded.start_time,
			expiration_time = excluded.expiration_time,
			updated_at = excluded.updated_at`,
		sub.UserID(),
		sub.Provider(),
		sub.ProductID(),
		sub.StartTime().Format(time.RFC3339),
		sub.ExpirationTime().Format(time.RFC3339),
		sub.UpdatedAt().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upserting subscription: %w", err)
	}
	return nil
}

// FindByUserID returns the subscription for the given user.
func (r *SQLiteRepository) FindByUserID(
	ctx context.Context,
	userID string,
) (domain.Subscription, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT user_id, provider, product_id, start_time, expiration_time, updated_at
		 FROM subscriptions
		 WHERE user_id = ?`,
		userID,
	)

	var (
		uid         string
		provider    string
		productID   string
		startTimeS  string
		expirationS string
		updatedAtS  string
	)
	err := row.Scan(&uid, &provider, &productID, &startTimeS, &expirationS, &updatedAtS)
	if err == sql.ErrNoRows {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, fmt.Errorf("scanning subscription: %w", err)
	}

	startTime, _ := time.Parse(time.RFC3339, startTimeS)
	expiration, _ := time.Parse(time.RFC3339, expirationS)
	updatedAt, _ := time.Parse(time.RFC3339, updatedAtS)

	sub := domain.NewSubscriptionUnsafe(
		uid, provider, productID,
		startTime, expiration, updatedAt,
	)
	return sub, true, nil
}
