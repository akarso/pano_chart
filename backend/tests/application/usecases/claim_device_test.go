package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/application/usecases"
)

type fakeCredentialStore struct {
	saved      map[string]string // secretHash -> userID
	claimedIDs map[string]bool   // userID -> already claimed
	err        error
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{
		saved:      make(map[string]string),
		claimedIDs: make(map[string]bool),
	}
}

func (f *fakeCredentialStore) SaveIfUserUnclaimed(_ context.Context, secretHash, userID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.claimedIDs[userID] {
		return false, nil
	}
	f.claimedIDs[userID] = true
	f.saved[secretHash] = userID
	return true, nil
}

func (f *fakeCredentialStore) Lookup(_ context.Context, secretHash string) (string, bool, error) {
	userID, ok := f.saved[secretHash]
	return userID, ok, nil
}

func TestClaimDevice_MintsNewUserID_WhenNoneProvided(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	result, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{})

	require.NoError(t, err)
	assert.NotEmpty(t, result.UserID)
	assert.NotEmpty(t, result.Secret)
}

func TestClaimDevice_BindsToExistingUserID(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	result, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{
		ExistingUserID: "legacy-user-123",
	})

	require.NoError(t, err)
	assert.Equal(t, "legacy-user-123", result.UserID)
}

func TestClaimDevice_SecretHashesToWhatWasStored(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	result, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{})
	require.NoError(t, err)

	// The middleware will hash the raw secret string exactly like this —
	// the use case must have stored under the same hash or auth breaks.
	hash := sha256.Sum256([]byte(result.Secret))
	userID, ok, err := store.Lookup(context.Background(), hex.EncodeToString(hash[:]))

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, result.UserID, userID)
}

func TestClaimDevice_TwoCallsProduceDifferentSecrets(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	r1, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{})
	require.NoError(t, err)
	r2, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{})
	require.NoError(t, err)

	assert.NotEqual(t, r1.Secret, r2.Secret)
	assert.NotEqual(t, r1.UserID, r2.UserID)
}

func TestClaimDevice_StoreError_Propagates(t *testing.T) {
	store := newFakeCredentialStore()
	store.err = assert.AnError
	uc := usecases.NewClaimDevice(store)

	_, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{})
	assert.Error(t, err)
}

func TestClaimDevice_ExistingUserID_SecondClaim_Rejected(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	// Legitimate first claim (the real owner's app, post-upgrade).
	_, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{
		ExistingUserID: "legacy-user-123",
	})
	require.NoError(t, err)

	// Someone who merely learned the ID tries to claim it too.
	_, err = uc.Execute(context.Background(), usecases.ClaimDeviceInput{
		ExistingUserID: "legacy-user-123",
	})
	assert.ErrorIs(t, err, usecases.ErrUserIDAlreadyClaimed)
}

func TestClaimDevice_ExistingUserID_InvalidFormat_Rejected(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	_, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{
		ExistingUserID: "not a valid id; drop table users;--",
	})
	assert.ErrorIs(t, err, usecases.ErrInvalidUserID)
}

func TestClaimDevice_ExistingUserID_TooLong_Rejected(t *testing.T) {
	store := newFakeCredentialStore()
	uc := usecases.NewClaimDevice(store)

	huge := make([]byte, 129)
	for i := range huge {
		huge[i] = 'a'
	}
	_, err := uc.Execute(context.Background(), usecases.ClaimDeviceInput{
		ExistingUserID: string(huge),
	})
	assert.ErrorIs(t, err, usecases.ErrInvalidUserID)
}
