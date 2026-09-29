package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/ports"
)

// fakeWatchlistStore is an in-memory ports.WatchlistStore for handler tests.
type fakeWatchlistStore struct {
	bySymbols          map[string][]string
	getErr             error
	replaceErr         error
	removeErr          error
	lastReplaceUserID  string
	lastReplaceSymbols []string
	lastRemoveUserID   string
	lastRemoveSymbols  []string
}

func (f *fakeWatchlistStore) Get(_ context.Context, userID string) ([]string, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.bySymbols == nil {
		return nil, nil
	}
	return f.bySymbols[userID], nil
}

func (f *fakeWatchlistStore) Replace(_ context.Context, userID string, symbols []string) error {
	f.lastReplaceUserID = userID
	f.lastReplaceSymbols = symbols
	if f.replaceErr != nil {
		return f.replaceErr
	}
	if f.bySymbols == nil {
		f.bySymbols = make(map[string][]string)
	}
	f.bySymbols[userID] = symbols
	return nil
}

func (f *fakeWatchlistStore) Remove(_ context.Context, userID string, symbols []string) error {
	f.lastRemoveUserID = userID
	f.lastRemoveSymbols = symbols
	if f.removeErr != nil {
		return f.removeErr
	}
	if f.bySymbols == nil {
		return nil
	}
	remove := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		remove[s] = true
	}
	kept := f.bySymbols[userID][:0]
	for _, s := range f.bySymbols[userID] {
		if !remove[s] {
			kept = append(kept, s)
		}
	}
	f.bySymbols[userID] = kept
	return nil
}

func TestWatchlistHandler_Get_ReturnsAuthenticatedUsersSymbols(t *testing.T) {
	store := &fakeWatchlistStore{bySymbols: map[string][]string{"user1": {"BTCUSDT", "ETHUSDT"}}}
	handler := adhttp.NewWatchlistHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var resp struct {
		Symbols []string `json:"symbols"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, []string{"BTCUSDT", "ETHUSDT"}, resp.Symbols)
}

func TestWatchlistHandler_Get_EmptyWatchlist_ReturnsEmptyArrayNotNull(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.JSONEq(t, `{"symbols":[]}`, w.Body.String())
}

func TestWatchlistHandler_Get_StoreError_500(t *testing.T) {
	store := &fakeWatchlistStore{getErr: fmt.Errorf("db down")}
	handler := adhttp.NewWatchlistHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestWatchlistHandler_Put_ReplacesAndReturnsWatchlist(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"BTCUSDT", "SOLUSDT"}})
	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "user1", store.lastReplaceUserID)
	assert.Equal(t, []string{"BTCUSDT", "SOLUSDT"}, store.lastReplaceSymbols)
	var resp struct {
		Symbols []string `json:"symbols"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, []string{"BTCUSDT", "SOLUSDT"}, resp.Symbols)
}

func TestWatchlistHandler_Put_TooManySymbols_400(t *testing.T) {
	store := &fakeWatchlistStore{replaceErr: ports.ErrWatchlistTooLarge}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"BTCUSDT"}})
	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestWatchlistHandler_Put_InvalidBody_400(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader([]byte("not json")))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

// PR-101 CR: "btcusdt" and "BTCUSDT" must not become two distinct entries.
func TestWatchlistHandler_Put_NormalizesCase(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"btcusdt", "BTCUSDT", "EthUsdt"}})
	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, []string{"BTCUSDT", "ETHUSDT"}, store.lastReplaceSymbols,
		"case-variant duplicates must collapse to one canonical, uppercased entry")
}

// PR-101 CR: an empty-string symbol must be rejected, not silently
// persisted and fed into the scheduler forever.
func TestWatchlistHandler_Put_RejectsEmptySymbol_400(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"BTCUSDT", ""}})
	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.Nil(t, store.lastReplaceSymbols, "Replace must never be called with an invalid payload")
}

// PR-101 CR: a symbol string longer than WatchlistMaxSymbolLength must be
// rejected before it ever reaches the store.
func TestWatchlistHandler_Put_RejectsOverlongSymbol_400(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	overlong := strings.Repeat("A", ports.WatchlistMaxSymbolLength+1)
	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{overlong}})
	req := httptest.NewRequest(http.MethodPut, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.Nil(t, store.lastReplaceSymbols)
}

// PR-101 CR: DELETE must normalize the same way PUT does, so deleting
// "btcusdt" removes the canonically-stored "BTCUSDT" entry.
func TestWatchlistHandler_Delete_NormalizesCase(t *testing.T) {
	store := &fakeWatchlistStore{bySymbols: map[string][]string{"user1": {"BTCUSDT"}}}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"btcusdt"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, []string{"BTCUSDT"}, store.lastRemoveSymbols)
}

func TestWatchlistHandler_Delete_RemovesAndReturnsWatchlist(t *testing.T) {
	store := &fakeWatchlistStore{bySymbols: map[string][]string{"user1": {"BTCUSDT", "SOLUSDT"}}}
	handler := adhttp.NewWatchlistHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"symbols": []string{"SOLUSDT"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/watchlist", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "user1", store.lastRemoveUserID)
	assert.Equal(t, []string{"SOLUSDT"}, store.lastRemoveSymbols)
	var resp struct {
		Symbols []string `json:"symbols"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, []string{"BTCUSDT"}, resp.Symbols)
}

func TestWatchlistHandler_MethodNotAllowed(t *testing.T) {
	store := &fakeWatchlistStore{}
	handler := adhttp.NewWatchlistHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/watchlist", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

// These drive requests through NewWatchlistRoute — the exact constructor
// cmd/api/main.go calls — instead of the bare handler, matching
// payment_handler_test.go's rationale: a regression like "someone stops
// wrapping this route in hard-enforced auth" must be caught here, not only
// in the middleware's own unit tests.

func TestWatchlistRoute_Unauthenticated_401(t *testing.T) {
	store := &fakeWatchlistStore{}
	credStore := &fakeRouteCredentialStore{}
	route := adhttp.NewWatchlistRoute(store, credStore)

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	w := httptest.NewRecorder()

	route.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

// TestWatchlistRoute_RateLimited_Returns429 is the regression test for
// PR-101 CR: /api/watchlist originally shipped with no rate limiting at
// all, unlike every other mutating endpoint in this codebase's history
// (see PR-075's payment rate-limiter). Drives requests through the real
// NewWatchlistRoute wiring, matching payment_handler_test.go's rationale.
func TestWatchlistRoute_RateLimited_Returns429(t *testing.T) {
	store := &fakeWatchlistStore{}
	credStore := &fakeRouteCredentialStore{byHash: map[string]string{routeHashOf("s3cr3t"): "user1"}}
	route := adhttp.NewWatchlistRoute(store, credStore)

	doRequest := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
		req.Header.Set("Authorization", "Bearer s3cr3t")
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)
		return w.Result().StatusCode
	}

	for i := 0; i < adhttp.WatchlistRateLimitBurst; i++ {
		if code := doRequest(); code != http.StatusOK {
			t.Fatalf("request %d: expected 200 within the burst allowance, got %d", i+1, code)
		}
	}
	if code := doRequest(); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhausting the burst allowance, got %d", code)
	}
}

func TestWatchlistRoute_ValidSecret_Succeeds(t *testing.T) {
	store := &fakeWatchlistStore{bySymbols: map[string][]string{"user1": {"BTCUSDT"}}}
	credStore := &fakeRouteCredentialStore{byHash: map[string]string{routeHashOf("s3cr3t"): "user1"}}
	route := adhttp.NewWatchlistRoute(store, credStore)

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	route.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var resp struct {
		Symbols []string `json:"symbols"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, []string{"BTCUSDT"}, resp.Symbols)
}
