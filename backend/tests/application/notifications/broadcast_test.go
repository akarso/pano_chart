package notifications_test

import (
	"context"
	"database/sql"
	"testing"

	appnotify "pano_chart/backend/application/notifications"
	infranotify "pano_chart/backend/infrastructure/notifications"
	infrasocial "pano_chart/backend/infrastructure/social"

	_ "modernc.org/sqlite"
)

// fakePushNotifier records sent pushes.
type fakePushNotifier struct {
	sent []pushCall
}

type pushCall struct {
	token, title, body string
	data               map[string]string
}

func (f *fakePushNotifier) Send(_ context.Context, token, title, body string, data map[string]string) error {
	f.sent = append(f.sent, pushCall{token, title, body, data})
	return nil
}

func openTestDeviceStore(t *testing.T) *infrasocial.SQLiteDeviceStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store, err := infrasocial.NewSQLiteDeviceStoreFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAllTokens_Empty(t *testing.T) {
	store := openTestDeviceStore(t)
	tokens, err := store.AllTokens()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestAllTokens_ReturnsAll(t *testing.T) {
	store := openTestDeviceStore(t)
	_ = store.Register("u1", "d1", "tok_aaa", "android")
	_ = store.Register("u2", "d2", "tok_bbb", "ios")
	_ = store.Register("u3", "d3", "tok_ccc", "android")

	tokens, err := store.AllTokens()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestAllTokens_Distinct(t *testing.T) {
	store := openTestDeviceStore(t)
	_ = store.Register("u1", "d1", "tok_same", "android")
	_ = store.Register("u1", "d2", "tok_same", "android")

	tokens, err := store.AllTokens()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 distinct token, got %d", len(tokens))
	}
}

func TestBroadcastSender_SendsToAllTokens(t *testing.T) {
	store := openTestDeviceStore(t)
	_ = store.Register("u1", "d1", "tok_aaa", "android")
	_ = store.Register("u2", "d2", "tok_bbb", "ios")

	push := &fakePushNotifier{}
	sender := infranotify.NewBroadcastSender(store, push)

	n := appnotify.Notification{
		Type:  appnotify.TypeNews,
		Title: "Headline",
		Body:  "Details here",
		Key:   "test",
	}

	if err := sender.Broadcast(context.Background(), n); err != nil {
		t.Fatalf("broadcast error: %v", err)
	}
	if len(push.sent) != 2 {
		t.Fatalf("expected 2 pushes, got %d", len(push.sent))
	}
	for _, p := range push.sent {
		if p.title != "Headline" || p.body != "Details here" {
			t.Fatalf("unexpected push: %+v", p)
		}
	}
}

func TestBroadcastSender_NoDevices(t *testing.T) {
	store := openTestDeviceStore(t)
	push := &fakePushNotifier{}
	sender := infranotify.NewBroadcastSender(store, push)

	err := sender.Broadcast(context.Background(), appnotify.Notification{
		Type: appnotify.TypeMarket, Title: "t", Body: "b", Key: "k",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(push.sent) != 0 {
		t.Fatal("expected no pushes for empty store")
	}
}
