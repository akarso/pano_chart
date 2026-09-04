package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"pano_chart/backend/adapters/http/middleware"
)

type fakeCredentialStore struct {
	byHash map[string]string
	err    error
}

func (f *fakeCredentialStore) SaveIfUserUnclaimed(_ context.Context, secretHash, userID string) (bool, error) {
	if f.byHash == nil {
		f.byHash = make(map[string]string)
	}
	for _, uid := range f.byHash {
		if uid == userID {
			return false, nil
		}
	}
	f.byHash[secretHash] = userID
	return true, nil
}

func (f *fakeCredentialStore) Lookup(_ context.Context, secretHash string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	userID, ok := f.byHash[secretHash]
	return userID, ok, nil
}

func hashOf(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func TestRequireAuth_ValidSecret_InjectsUserID(t *testing.T) {
	store := &fakeCredentialStore{byHash: map[string]string{hashOf("s3cr3t"): "user1"}}

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = middleware.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, true)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "user1", gotUserID)
}

func TestRequireAuth_MissingHeader_401(t *testing.T) {
	store := &fakeCredentialStore{}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, true)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
	assert.False(t, called, "next handler must not run")
}

func TestRequireAuth_LogOnly_MissingHeader_AllowsThroughWithoutContext(t *testing.T) {
	store := &fakeCredentialStore{}
	called := false
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, gotOK = middleware.UserIDFromContextOK(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, false)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.True(t, called, "log-only mode must still call the next handler")
	assert.False(t, gotOK, "no verified identity should be in context")
}

func TestRequireAuth_LogOnly_ValidSecret_StillInjectsUserID(t *testing.T) {
	// Log-only only changes what happens on FAILURE to authenticate — a
	// valid secret is still verified and trusted exactly like enforce=true.
	store := &fakeCredentialStore{byHash: map[string]string{hashOf("s3cr3t"): "user1"}}

	var gotUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = middleware.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, false)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "user1", gotUserID)
}

func TestRequireAuth_LogOnly_UnknownSecret_AllowsThroughWithoutContext(t *testing.T) {
	store := &fakeCredentialStore{}
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotOK = middleware.UserIDFromContextOK(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer nonsense")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, false)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.False(t, gotOK)
}

func TestRequireAuth_LogOnly_StoreError_Still500(t *testing.T) {
	// A store error is a real infra failure, not "unauthenticated" — must
	// still surface as 500 even in log-only mode, not be swallowed.
	store := &fakeCredentialStore{err: assert.AnError}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, false)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestRequireAuth_MalformedAuthHeader_401(t *testing.T) {
	// Right secret, wrong/missing scheme — must not be silently treated as
	// the secret itself.
	store := &fakeCredentialStore{byHash: map[string]string{hashOf("s3cr3t"): "user1"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	for _, header := range []string{"s3cr3t", "Basic s3cr3t", "Bearer", "bearer s3cr3t"} {
		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		req.Header.Set("Authorization", header)
		w := httptest.NewRecorder()

		middleware.RequireAuth(store, true)(next).ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode, "header=%q", header)
	}
}

func TestRequireAuth_UnknownSecret_401(t *testing.T) {
	store := &fakeCredentialStore{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer nonsense")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, true)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

func TestRequireAuth_StoreError_500(t *testing.T) {
	store := &fakeCredentialStore{err: assert.AnError}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	middleware.RequireAuth(store, true)(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestUserIDFromContext_PanicsWithoutWithUserID(t *testing.T) {
	assert.Panics(t, func() {
		middleware.UserIDFromContext(context.Background())
	})
}

func TestWithUserID_RoundTrips(t *testing.T) {
	ctx := middleware.WithUserID(context.Background(), "abc")
	assert.Equal(t, "abc", middleware.UserIDFromContext(ctx))
}
