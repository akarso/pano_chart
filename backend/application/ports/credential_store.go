package ports

import "context"

// CredentialStore persists device-secret hashes and resolves them back to
// the user ID they were issued for. Secrets are never stored or looked up
// in plaintext — callers pass/receive only the SHA-256 hash (hex-encoded).
type CredentialStore interface {
	// SaveIfUserUnclaimed atomically saves a new secret hash bound to
	// userID, but only if userID has no existing credential — the
	// check-and-set happens as a single guarded SQL statement (same
	// pattern as SQLiteDeviceStore.Register's ownership guard), not a
	// separate read-then-write, so two concurrent claims for the same
	// userID cannot both succeed. Returns ok=false (no error) if userID
	// was already claimed. This is what makes the existingUserId
	// migration path first-claim-wins: once a user ID has been claimed
	// for real, it can never be claimed again by someone who merely
	// learned the ID — including via a race.
	SaveIfUserUnclaimed(ctx context.Context, secretHash, userID string) (ok bool, err error)

	// Lookup resolves a secret hash to its bound user ID. Returns ok=false
	// if no credential matches.
	Lookup(ctx context.Context, secretHash string) (userID string, ok bool, err error)
}
