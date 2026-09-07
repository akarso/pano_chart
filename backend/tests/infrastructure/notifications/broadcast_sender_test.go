package notifications_test

import (
	"context"
	"errors"
	"testing"

	appnotify "pano_chart/backend/application/notifications"
	infranotify "pano_chart/backend/infrastructure/notifications"
)

// fakeTokenStore returns a fixed set of tokens regardless of user filter.
type fakeTokenStore struct {
	tokens []string
}

func (f *fakeTokenStore) Register(_, _, _, _ string) error { return nil }
func (f *fakeTokenStore) Unregister(_, _ string) error      { return nil }
func (f *fakeTokenStore) TokensForUsers(_ []string) ([]string, error) {
	return f.tokens, nil
}
func (f *fakeTokenStore) AllTokens() ([]string, error) { return f.tokens, nil }

// failingTokensPusher fails Send for any token in failFor, succeeds otherwise.
type failingTokensPusher struct {
	failFor map[string]bool
	sent    []string
}

func (p *failingTokensPusher) Send(_ context.Context, token, _, _ string, _ map[string]string) error {
	p.sent = append(p.sent, token)
	if p.failFor[token] {
		return errors.New("provider error for " + token)
	}
	return nil
}

func TestBroadcastSender_SendToUser_PartialFailureTreatedAsDelivered(t *testing.T) {
	store := &fakeTokenStore{tokens: []string{"tok-a", "tok-b", "tok-c"}}
	pusher := &failingTokensPusher{failFor: map[string]bool{"tok-b": true}}
	sender := infranotify.NewBroadcastSender(store, pusher)

	n := appnotify.Notification{Type: appnotify.TypeMarket, Title: "t", Body: "b", Key: "k"}

	if err := sender.SendToUser(context.Background(), "u1", n); err != nil {
		t.Fatalf("expected partial failure to be reported as success, got: %v", err)
	}
	if len(pusher.sent) != 3 {
		t.Fatalf("expected all 3 tokens attempted, got %d", len(pusher.sent))
	}
}

func TestBroadcastSender_SendToUser_TotalFailureReturnsError(t *testing.T) {
	store := &fakeTokenStore{tokens: []string{"tok-a", "tok-b"}}
	pusher := &failingTokensPusher{failFor: map[string]bool{"tok-a": true, "tok-b": true}}
	sender := infranotify.NewBroadcastSender(store, pusher)

	n := appnotify.Notification{Type: appnotify.TypeMarket, Title: "t", Body: "b", Key: "k"}

	if err := sender.SendToUser(context.Background(), "u1", n); err == nil {
		t.Fatal("expected total failure across all tokens to return an error")
	}
}

func TestBroadcastSender_Broadcast_PartialFailureTreatedAsDelivered(t *testing.T) {
	store := &fakeTokenStore{tokens: []string{"tok-a", "tok-b"}}
	pusher := &failingTokensPusher{failFor: map[string]bool{"tok-a": true}}
	sender := infranotify.NewBroadcastSender(store, pusher)

	n := appnotify.Notification{Type: appnotify.TypeNews, Title: "t", Body: "b", Key: "k"}

	if err := sender.Broadcast(context.Background(), n); err != nil {
		t.Fatalf("expected partial failure to be reported as success, got: %v", err)
	}
}

func TestBroadcastSender_Broadcast_TotalFailureReturnsError(t *testing.T) {
	store := &fakeTokenStore{tokens: []string{"tok-a"}}
	pusher := &failingTokensPusher{failFor: map[string]bool{"tok-a": true}}
	sender := infranotify.NewBroadcastSender(store, pusher)

	n := appnotify.Notification{Type: appnotify.TypeNews, Title: "t", Body: "b", Key: "k"}

	if err := sender.Broadcast(context.Background(), n); err == nil {
		t.Fatal("expected total failure to return an error")
	}
}
