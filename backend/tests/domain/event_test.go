package domain_test

import (
	"testing"
	"time"

	"pano_chart/backend/domain"
)

func TestNewEvent_Valid(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	ev, err := domain.NewEvent("", "United States", "PMI Final", domain.EventImpactHigh, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID() == "" {
		t.Error("expected auto-generated ID, got empty")
	}
	if ev.Country() != "United States" {
		t.Errorf("country = %q, want %q", ev.Country(), "United States")
	}
	if ev.Title() != "PMI Final" {
		t.Errorf("title = %q, want %q", ev.Title(), "PMI Final")
	}
	if ev.Impact() != domain.EventImpactHigh {
		t.Errorf("impact = %q, want %q", ev.Impact(), domain.EventImpactHigh)
	}
	if !ev.Timestamp().Equal(ts) {
		t.Errorf("timestamp = %v, want %v", ev.Timestamp(), ts)
	}
}

func TestNewEvent_ExplicitID(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	ev, err := domain.NewEvent("custom-id", "US", "CPI", domain.EventImpactMedium, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID() != "custom-id" {
		t.Errorf("id = %q, want %q", ev.ID(), "custom-id")
	}
}

func TestNewEvent_DeterministicID(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	ev1, _ := domain.NewEvent("", "US", "CPI", domain.EventImpactHigh, ts)
	ev2, _ := domain.NewEvent("", "US", "CPI", domain.EventImpactHigh, ts)
	if ev1.ID() != ev2.ID() {
		t.Errorf("same inputs should produce same ID: %q vs %q", ev1.ID(), ev2.ID())
	}
}

func TestNewEvent_DifferentInputsDifferentID(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	ev1, _ := domain.NewEvent("", "US", "CPI", domain.EventImpactHigh, ts)
	ev2, _ := domain.NewEvent("", "US", "PMI", domain.EventImpactHigh, ts)
	if ev1.ID() == ev2.ID() {
		t.Errorf("different titles should produce different IDs")
	}
}

func TestNewEvent_EmptyCountry(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	_, err := domain.NewEvent("", "", "CPI", domain.EventImpactHigh, ts)
	if err == nil {
		t.Error("expected error for empty country")
	}
}

func TestNewEvent_EmptyTitle(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	_, err := domain.NewEvent("", "US", "", domain.EventImpactHigh, ts)
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestNewEvent_ZeroTimestamp(t *testing.T) {
	_, err := domain.NewEvent("", "US", "CPI", domain.EventImpactHigh, time.Time{})
	if err == nil {
		t.Error("expected error for zero timestamp")
	}
}

func TestNewEvent_TimestampAlwaysUTC(t *testing.T) {
	eastern, _ := time.LoadLocation("America/New_York")
	ts := time.Date(2025, 3, 3, 9, 45, 0, 0, eastern)
	ev, err := domain.NewEvent("", "US", "CPI", domain.EventImpactHigh, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Timestamp().Location() != time.UTC {
		t.Errorf("expected UTC, got %v", ev.Timestamp().Location())
	}
}

func TestParseEventImpact(t *testing.T) {
	tests := []struct {
		input string
		want  domain.EventImpact
	}{
		{"High", domain.EventImpactHigh},
		{"high", domain.EventImpactHigh},
		{"HIGH", domain.EventImpactHigh},
		{"Moderate", domain.EventImpactMedium},
		{"moderate", domain.EventImpactMedium},
		{"medium", domain.EventImpactMedium},
		{"Low", domain.EventImpactLow},
		{"low", domain.EventImpactLow},
		{"", domain.EventImpactMedium},
		{"unknown", domain.EventImpactMedium},
		{"  High  ", domain.EventImpactHigh},
	}

	for _, tc := range tests {
		got := domain.ParseEventImpact(tc.input)
		if got != tc.want {
			t.Errorf("ParseEventImpact(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
