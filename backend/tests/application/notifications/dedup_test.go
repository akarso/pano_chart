package notifications_test

import (
	"testing"
	"time"

	"pano_chart/backend/application/notifications"
)

func TestDedup_SeenBeforeMark(t *testing.T) {
	d := notifications.NewDeduplicator()
	if d.Seen("key1") {
		t.Fatal("expected unseen key to return false")
	}
}

func TestDedup_MarkThenSeen(t *testing.T) {
	d := notifications.NewDeduplicator()
	d.Mark("key1", time.Hour)
	if !d.Seen("key1") {
		t.Fatal("expected marked key to return true")
	}
}

func TestDedup_Expiry(t *testing.T) {
	d := notifications.NewDeduplicator()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	d.SetClock(func() time.Time { return now })

	d.Mark("key1", 10*time.Minute)

	// Still within TTL.
	d.SetClock(func() time.Time { return now.Add(9 * time.Minute) })
	if !d.Seen("key1") {
		t.Fatal("expected key to still be valid before TTL")
	}

	// Past TTL.
	d.SetClock(func() time.Time { return now.Add(11 * time.Minute) })
	if d.Seen("key1") {
		t.Fatal("expected key to be expired after TTL")
	}
}

func TestDedup_Len(t *testing.T) {
	d := notifications.NewDeduplicator()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	d.SetClock(func() time.Time { return now })

	d.Mark("a", 10*time.Minute)
	d.Mark("b", 5*time.Minute)
	d.Mark("c", 20*time.Minute)

	if got := d.Len(); got != 3 {
		t.Fatalf("expected Len()=3, got %d", got)
	}

	// Advance past b's TTL.
	d.SetClock(func() time.Time { return now.Add(6 * time.Minute) })
	if got := d.Len(); got != 2 {
		t.Fatalf("expected Len()=2 after b expired, got %d", got)
	}
}

func TestDedup_DifferentKeys(t *testing.T) {
	d := notifications.NewDeduplicator()
	d.Mark("key1", time.Hour)
	if d.Seen("key2") {
		t.Fatal("different key should not be seen")
	}
}
