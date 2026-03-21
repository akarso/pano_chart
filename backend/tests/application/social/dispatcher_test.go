package social_test

import (
	"testing"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
)

func TestDispatcher_DispatchAndReceive(t *testing.T) {
	d := appsocial.NewDispatcher(10)
	posts := []domain.Post{{ID: "1"}, {ID: "2"}}

	d.Dispatch(posts)

	select {
	case got := <-d.Events():
		if len(got) != 2 {
			t.Fatalf("expected 2 posts, got %d", len(got))
		}
	default:
		t.Fatal("expected event on channel")
	}
}

func TestDispatcher_DropsWhenBufferFull(t *testing.T) {
	d := appsocial.NewDispatcher(1)

	d.Dispatch([]domain.Post{{ID: "1"}}) // fills buffer
	d.Dispatch([]domain.Post{{ID: "2"}}) // should drop

	// Only one should be in the channel.
	select {
	case got := <-d.Events():
		if got[0].ID != "1" {
			t.Fatalf("expected first dispatch, got %s", got[0].ID)
		}
	default:
		t.Fatal("expected at least one event")
	}

	select {
	case <-d.Events():
		t.Fatal("expected no second event (should have been dropped)")
	default:
		// good — nothing else
	}
}
