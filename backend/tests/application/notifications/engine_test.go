package notifications_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"pano_chart/backend/application/notifications"
)

// spySender records broadcast calls.
type spySender struct {
	mu       sync.Mutex
	calls    []notifications.Notification
	userSent []userSendRecord
}

type userSendRecord struct {
	userID string
	n      notifications.Notification
}

func (s *spySender) Broadcast(_ context.Context, n notifications.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, n)
	return nil
}

func (s *spySender) SendToUser(_ context.Context, userID string, n notifications.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userSent = append(s.userSent, userSendRecord{userID, n})
	return nil
}

func (s *spySender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *spySender) userCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.userSent)
}

func (s *spySender) last() notifications.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

func (s *spySender) lastUserSend() userSendRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.userSent[len(s.userSent)-1]
}

func TestEngine_SendWithinHours(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	// 14:00 — well within 07–22.
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 14, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type:  notifications.TypeNews,
		Title: "Breaking",
		Body:  "Big news",
		Key:   "test_1",
	}

	if err := eng.Send(context.Background(), n); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.count() != 1 {
		t.Fatalf("expected 1 broadcast, got %d", spy.count())
	}
	if spy.last().Title != "Breaking" {
		t.Fatalf("unexpected title: %s", spy.last().Title)
	}
}

func TestEngine_SuppressedOutsideHours(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	// 03:00 — outside 07–22.
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 3, 0, 0, 0, time.UTC)
	})

	err := eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeMarket, Title: "t", Body: "b", Key: "k",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.count() != 0 {
		t.Fatal("expected suppressed during quiet hours")
	}
}

func TestEngine_Dedup(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMacro, Title: "FOMC", Body: "30 min", Key: "macro_fomc",
	}

	_ = eng.Send(context.Background(), n)
	_ = eng.Send(context.Background(), n) // duplicate

	if spy.count() != 1 {
		t.Fatalf("expected 1 (dedup'd), got %d", spy.count())
	}
}

func TestEngine_DifferentKeysNotDeduped(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	_ = eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeNews, Title: "A", Body: "a", Key: "k1",
	})
	_ = eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeNews, Title: "B", Body: "b", Key: "k2",
	})

	if spy.count() != 2 {
		t.Fatalf("expected 2 different keys, got %d", spy.count())
	}
}

// failThenSucceedSender fails the first N calls to each method, then succeeds.
type failThenSucceedSender struct {
	mu             sync.Mutex
	failBroadcasts int
	failUserSends  int
	broadcastCalls int
	userSendCalls  int
}

func (s *failThenSucceedSender) Broadcast(_ context.Context, _ notifications.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcastCalls++
	if s.broadcastCalls <= s.failBroadcasts {
		return errors.New("transient broadcast error")
	}
	return nil
}

func (s *failThenSucceedSender) SendToUser(_ context.Context, _ string, _ notifications.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userSendCalls++
	if s.userSendCalls <= s.failUserSends {
		return errors.New("transient send error")
	}
	return nil
}

func TestEngine_SendToUser_FailedSendDoesNotBlockRetry(t *testing.T) {
	sender := &failThenSucceedSender{failUserSends: 1}
	eng := notifications.NewEngine(sender, notifications.DefaultEngineConfig())

	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMarket, Title: "t", Body: "b", Key: "market_1h_Uptrend_2025-06-01",
	}

	if err := eng.SendToUser(context.Background(), "u1", n); err == nil {
		t.Fatal("expected first send to return the sender's error")
	}

	if err := eng.SendToUser(context.Background(), "u1", n); err != nil {
		t.Fatalf("expected retry with same key to succeed, got error: %v", err)
	}

	if sender.userSendCalls != 2 {
		t.Fatalf("expected the retry to actually reach the sender, got %d calls", sender.userSendCalls)
	}
}

func TestEngine_Send_FailedBroadcastDoesNotBlockRetry(t *testing.T) {
	sender := &failThenSucceedSender{failBroadcasts: 1}
	eng := notifications.NewEngine(sender, notifications.DefaultEngineConfig())

	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMacro, Title: "FOMC", Body: "30 min", Key: "macro_fomc_retry",
	}

	if err := eng.Send(context.Background(), n); err == nil {
		t.Fatal("expected first broadcast to return the sender's error")
	}

	if err := eng.Send(context.Background(), n); err != nil {
		t.Fatalf("expected retry with same key to succeed, got error: %v", err)
	}

	if sender.broadcastCalls != 2 {
		t.Fatalf("expected the retry to actually reach the sender, got %d calls", sender.broadcastCalls)
	}
}

func TestEngine_SendToUser_ConcurrentSameKeyOnlyDeliversOnce(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	})

	n := notifications.Notification{
		Type: notifications.TypeMarket, Title: "t", Body: "b", Key: "market_1h_Uptrend_2025-06-01",
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = eng.SendToUser(context.Background(), "u1", n)
		}()
	}
	wg.Wait()

	if got := spy.userCount(); got != 1 {
		t.Fatalf("expected exactly 1 delivery for concurrent same-key sends, got %d", got)
	}
}

func TestEngine_BoundaryHours(t *testing.T) {
	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())

	// 07:00 — exactly at start.
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 7, 0, 0, 0, time.UTC)
	})
	_ = eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeNews, Title: "T", Body: "B", Key: "boundary_start",
	})
	if spy.count() != 1 {
		t.Fatal("expected 07:00 to be allowed")
	}

	// 22:00 — exactly at end.
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 22, 0, 0, 0, time.UTC)
	})
	_ = eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeNews, Title: "T", Body: "B", Key: "boundary_end",
	})
	if spy.count() != 2 {
		t.Fatal("expected 22:00 to be allowed")
	}

	// 23:00 — past end.
	eng.SetClock(func() time.Time {
		return time.Date(2025, 6, 1, 23, 0, 0, 0, time.UTC)
	})
	_ = eng.Send(context.Background(), notifications.Notification{
		Type: notifications.TypeNews, Title: "T", Body: "B", Key: "boundary_after",
	})
	if spy.count() != 2 {
		t.Fatal("expected 23:00 to be suppressed")
	}
}
