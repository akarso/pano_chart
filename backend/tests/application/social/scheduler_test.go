package social_test

import (
	"testing"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
)

func TestScheduler_EmptyReturnsNoAccount(t *testing.T) {
	sched := appsocial.NewScheduler()

	_, ok := sched.Next()
	if ok {
		t.Fatal("expected no account from empty scheduler")
	}
}

func TestScheduler_RotatesAccounts(t *testing.T) {
	sched := appsocial.NewScheduler()
	accounts := []domain.Account{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	sched.SetAccounts(accounts)

	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		acc, ok := sched.Next()
		if !ok {
			t.Fatalf("expected account at iteration %d", i)
		}
		ids = append(ids, acc.ID)
	}

	// Should rotate: a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, id := range ids {
		if id != expected[i] {
			t.Fatalf("iteration %d: expected %s, got %s", i, expected[i], id)
		}
	}
}

func TestScheduler_SetAccountsResetsIndexIfOutOfRange(t *testing.T) {
	sched := appsocial.NewScheduler()
	sched.SetAccounts([]domain.Account{{ID: "a"}, {ID: "b"}, {ID: "c"}})

	// Advance index to 2.
	sched.Next() // a (index→1)
	sched.Next() // b (index→2)

	// Replace with shorter list.
	sched.SetAccounts([]domain.Account{{ID: "x"}})

	acc, ok := sched.Next()
	if !ok || acc.ID != "x" {
		t.Fatalf("expected 'x', got %v (ok=%v)", acc.ID, ok)
	}
}

func TestScheduler_Len(t *testing.T) {
	sched := appsocial.NewScheduler()
	if sched.Len() != 0 {
		t.Fatalf("expected 0, got %d", sched.Len())
	}

	sched.SetAccounts([]domain.Account{{ID: "a"}, {ID: "b"}})
	if sched.Len() != 2 {
		t.Fatalf("expected 2, got %d", sched.Len())
	}
}
