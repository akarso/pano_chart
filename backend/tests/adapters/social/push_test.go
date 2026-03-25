package social_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"

	_ "modernc.org/sqlite"
)

// ── Stub PushNotifier ───────────────────────────────────────────────────────

type stubNotifier struct {
	mu    sync.Mutex
	sends []sendRecord
}

type sendRecord struct {
	Token string
	Title string
	Body  string
	Data  map[string]string
}

func (s *stubNotifier) Send(_ context.Context, token, title, body string, data map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends = append(s.sends, sendRecord{Token: token, Title: title, Body: body, Data: data})
	return nil
}

func (s *stubNotifier) getSends() []sendRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]sendRecord, len(s.sends))
	copy(cp, s.sends)
	return cp
}

// ── In-memory DeviceTokenStore ──────────────────────────────────────────────

type memDeviceStore struct {
	mu      sync.Mutex
	devices map[string]deviceEntry
}

type deviceEntry struct {
	userID   string
	fcmToken string
	platform string
}

func newMemDeviceStore() *memDeviceStore {
	return &memDeviceStore{devices: make(map[string]deviceEntry)}
}

func (s *memDeviceStore) Register(userID, deviceID, fcmToken, platform string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[deviceID] = deviceEntry{userID: userID, fcmToken: fcmToken, platform: platform}
	return nil
}

func (s *memDeviceStore) Unregister(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.devices, deviceID)
	return nil
}

func (s *memDeviceStore) TokensForUsers(userIDs []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idSet := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		idSet[id] = true
	}
	seen := make(map[string]bool)
	var tokens []string
	for _, e := range s.devices {
		if idSet[e.userID] && !seen[e.fcmToken] {
			tokens = append(tokens, e.fcmToken)
			seen[e.fcmToken] = true
		}
	}
	return tokens, nil
}

func (s *memDeviceStore) AllTokens() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool)
	var tokens []string
	for _, e := range s.devices {
		if !seen[e.fcmToken] {
			tokens = append(tokens, e.fcmToken)
			seen[e.fcmToken] = true
		}
	}
	return tokens, nil
}

// ── SQLite DeviceStore tests ────────────────────────────────────────────────

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLiteDeviceStore_RegisterAndQuery(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infrasocial.NewSQLiteDeviceStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Register("u1", "d1", "tok-aaa", "android"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.Register("u1", "d2", "tok-bbb", "ios"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.Register("u2", "d3", "tok-ccc", "android"); err != nil {
		t.Fatalf("register: %v", err)
	}

	tokens, err := store.TokensForUsers([]string{"u1"})
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens for u1, got %d", len(tokens))
	}

	tokens, err = store.TokensForUsers([]string{"u1", "u2"})
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestSQLiteDeviceStore_Unregister(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infrasocial.NewSQLiteDeviceStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_ = store.Register("u1", "d1", "tok-aaa", "android")
	_ = store.Unregister("d1")

	tokens, err := store.TokensForUsers([]string{"u1"})
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens after unregister, got %d", len(tokens))
	}
}

func TestSQLiteDeviceStore_UpsertUpdatesToken(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infrasocial.NewSQLiteDeviceStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_ = store.Register("u1", "d1", "old-tok", "android")
	_ = store.Register("u1", "d1", "new-tok", "android")

	tokens, err := store.TokensForUsers([]string{"u1"})
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token after upsert, got %d", len(tokens))
	}
	if tokens[0] != "new-tok" {
		t.Fatalf("expected 'new-tok', got '%s'", tokens[0])
	}
}

func TestSQLiteDeviceStore_EmptyUserIDs(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	store, err := infrasocial.NewSQLiteDeviceStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	tokens, err := store.TokensForUsers(nil)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens != nil {
		t.Fatalf("expected nil for empty user list, got %v", tokens)
	}
}

// ── PushConsumer tests ──────────────────────────────────────────────────────

func TestPushConsumer_SendsToSubscribers(t *testing.T) {
	subStore := infrasocial.NewMemorySubscriptionStore()
	_ = subStore.Subscribe("u1", "twitter:alice")
	_ = subStore.Subscribe("u2", "twitter:alice")

	devStore := newMemDeviceStore()
	_ = devStore.Register("u1", "d1", "tok-u1", "android")
	_ = devStore.Register("u2", "d2", "tok-u2", "ios")

	notifier := &stubNotifier{}

	ch := make(chan []domain.Post, 1)
	consumer := appsocial.NewPushConsumer(ch, subStore, devStore, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(ctx)

	ch <- []domain.Post{
		{ID: "p1", AccountID: "twitter:alice", Author: "@alice", Title: "big news", Timestamp: 1000},
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	sends := notifier.getSends()
	if len(sends) != 2 {
		t.Fatalf("expected 2 sends, got %d", len(sends))
	}
	if sends[0].Title != "@alice" {
		t.Fatalf("expected title '@alice', got '%s'", sends[0].Title)
	}
	if sends[0].Body != "big news" {
		t.Fatalf("expected body 'big news', got '%s'", sends[0].Body)
	}
}

func TestPushConsumer_MultiplePosts_AggregatesBody(t *testing.T) {
	subStore := infrasocial.NewMemorySubscriptionStore()
	_ = subStore.Subscribe("u1", "twitter:bob")

	devStore := newMemDeviceStore()
	_ = devStore.Register("u1", "d1", "tok-u1", "android")

	notifier := &stubNotifier{}

	ch := make(chan []domain.Post, 1)
	consumer := appsocial.NewPushConsumer(ch, subStore, devStore, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(ctx)

	ch <- []domain.Post{
		{ID: "p1", AccountID: "twitter:bob", Title: "first", Timestamp: 1001},
		{ID: "p2", AccountID: "twitter:bob", Title: "second", Timestamp: 1000},
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	sends := notifier.getSends()
	if len(sends) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sends))
	}
	if sends[0].Body != "first (+1 more)" {
		t.Fatalf("expected aggregated body, got '%s'", sends[0].Body)
	}
}

func TestPushConsumer_NoSubscribers_NoSends(t *testing.T) {
	subStore := infrasocial.NewMemorySubscriptionStore()
	devStore := newMemDeviceStore()
	notifier := &stubNotifier{}

	ch := make(chan []domain.Post, 1)
	consumer := appsocial.NewPushConsumer(ch, subStore, devStore, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(ctx)

	ch <- []domain.Post{
		{ID: "p1", AccountID: "twitter:nobody", Title: "hello", Timestamp: 1000},
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	if len(notifier.getSends()) != 0 {
		t.Fatal("expected 0 sends for no subscribers")
	}
}

func TestPushConsumer_NoDeviceTokens_NoSends(t *testing.T) {
	subStore := infrasocial.NewMemorySubscriptionStore()
	_ = subStore.Subscribe("u1", "twitter:alice")

	devStore := newMemDeviceStore()
	notifier := &stubNotifier{}

	ch := make(chan []domain.Post, 1)
	consumer := appsocial.NewPushConsumer(ch, subStore, devStore, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go consumer.Run(ctx)

	ch <- []domain.Post{
		{ID: "p1", AccountID: "twitter:alice", Title: "hello", Timestamp: 1000},
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	if len(notifier.getSends()) != 0 {
		t.Fatal("expected 0 sends for user without device")
	}
}

// ── Device Register/Unregister handler tests ────────────────────────────────

func TestDeviceRegisterHandler_Success(t *testing.T) {
	store := newMemDeviceStore()
	handler := adhttp.NewDeviceRegisterHandler(store)

	body := `{"user_id":"u1","device_id":"d1","fcm_token":"tok-123","platform":"android"}`
	req := httptest.NewRequest(http.MethodPost, "/api/device/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "registered" {
		t.Fatalf("expected 'registered', got '%s'", resp["status"])
	}
}

func TestDeviceRegisterHandler_MissingFields(t *testing.T) {
	store := newMemDeviceStore()
	handler := adhttp.NewDeviceRegisterHandler(store)

	body := `{"user_id":"u1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/device/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeviceRegisterHandler_InvalidPlatform(t *testing.T) {
	store := newMemDeviceStore()
	handler := adhttp.NewDeviceRegisterHandler(store)

	body := `{"user_id":"u1","device_id":"d1","fcm_token":"tok","platform":"windows"}`
	req := httptest.NewRequest(http.MethodPost, "/api/device/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeviceRegisterHandler_WrongMethod(t *testing.T) {
	store := newMemDeviceStore()
	handler := adhttp.NewDeviceRegisterHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/device/register", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDeviceUnregisterHandler_Success(t *testing.T) {
	store := newMemDeviceStore()
	_ = store.Register("u1", "d1", "tok-123", "android")

	handler := adhttp.NewDeviceUnregisterHandler(store)

	body := `{"device_id":"d1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/device/unregister", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeviceUnregisterHandler_MissingDeviceID(t *testing.T) {
	store := newMemDeviceStore()
	handler := adhttp.NewDeviceUnregisterHandler(store)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/device/unregister", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ── MemorySubscriptionStore.UsersForAccount ─────────────────────────────────

func TestUsersForAccount(t *testing.T) {
	store := infrasocial.NewMemorySubscriptionStore()
	_ = store.Subscribe("u1", "twitter:alice")
	_ = store.Subscribe("u2", "twitter:alice")
	_ = store.Subscribe("u3", "twitter:bob")

	users, err := store.UsersForAccount("twitter:alice")
	if err != nil {
		t.Fatalf("users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users for alice, got %d", len(users))
	}

	users, err = store.UsersForAccount("twitter:nobody")
	if err != nil {
		t.Fatalf("users: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users for unknown account, got %d", len(users))
	}
}
