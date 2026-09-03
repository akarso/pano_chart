package usecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"

	"pano_chart/backend/application/ports"
)

// ErrUserIDAlreadyClaimed is returned when ExistingUserID has already been
// bound to a credential. The migration path is first-claim-wins: once the
// real owner's app has claimed its local ID, learning that ID later (leaked
// log, screenshot, whatever) can no longer mint a credential for it.
var ErrUserIDAlreadyClaimed = errors.New("user id already claimed")

// ErrInvalidUserID is returned when ExistingUserID doesn't look like an ID
// this backend would have generated or accepted.
var ErrInvalidUserID = errors.New("invalid existing user id")

// existingUserIDPattern accepts the UUID-v4-ish shape PreferencesService
// generates, but is deliberately a bit more permissive (any reasonable
// opaque token shape) so a future client ID format isn't blocked outright.
var existingUserIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ClaimDevice mints a new server-issued device credential, or binds one to
// an existing user ID for installs that already have a locally-generated ID
// (migration path for pre-PR-070 clients).
//
// This is intentionally NOT a login/account system — no password, no email,
// nothing to remember. It exists solely so the backend can stop trusting a
// client-supplied userId on every request.
type ClaimDevice interface {
	Execute(ctx context.Context, input ClaimDeviceInput) (ClaimDeviceResult, error)
}

// ClaimDeviceInput carries the optional pre-existing local user ID.
type ClaimDeviceInput struct {
	// ExistingUserID, if set, binds the new secret to this ID instead of
	// minting a fresh one — preserves subscription/notification history for
	// installs that predate server-issued identity.
	ExistingUserID string
}

// ClaimDeviceResult is the newly issued credential.
type ClaimDeviceResult struct {
	UserID string
	Secret string // raw secret — shown once, caller must persist it
}

type claimDevice struct {
	store ports.CredentialStore
}

// NewClaimDevice constructs the use case.
func NewClaimDevice(store ports.CredentialStore) ClaimDevice {
	return &claimDevice{store: store}
}

func (uc *claimDevice) Execute(ctx context.Context, input ClaimDeviceInput) (ClaimDeviceResult, error) {
	userID := input.ExistingUserID
	if userID != "" {
		if !existingUserIDPattern.MatchString(userID) {
			return ClaimDeviceResult{}, ErrInvalidUserID
		}
	} else {
		userID = uuid.NewString()
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return ClaimDeviceResult{}, fmt.Errorf("generating secret: %w", err)
	}
	// The base64 string form is what the client stores and sends back as
	// the Authorization header — hash that exact form, not the raw bytes,
	// so RequireAuth's lookup hash matches byte-for-byte.
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(secret))

	// Single atomic check-and-set — see SaveIfUserUnclaimed's doc for why
	// this can't be a separate "is it claimed?" read followed by a write.
	ok, err := uc.store.SaveIfUserUnclaimed(ctx, hex.EncodeToString(hash[:]), userID)
	if err != nil {
		return ClaimDeviceResult{}, fmt.Errorf("saving credential: %w", err)
	}
	if !ok {
		return ClaimDeviceResult{}, ErrUserIDAlreadyClaimed
	}

	return ClaimDeviceResult{UserID: userID, Secret: secret}, nil
}
