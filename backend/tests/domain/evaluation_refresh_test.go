package domain_test

import (
	"testing"
	"time"

	"pano_chart/backend/domain"
)

func TestEvaluationStaleAfter_KnownTimeframes(t *testing.T) {
	cases := []struct {
		tf   domain.Timeframe
		want time.Duration
	}{
		// refresh = max(30s, tf/4); stale = 2× refresh
		{domain.Timeframe15m, 2 * (15 * time.Minute / 4)}, // 7m30s
		{domain.Timeframe1h, 2 * (time.Hour / 4)},         // 30m
		{domain.Timeframe4h, 2 * (4 * time.Hour / 4)},     // 2h
		{domain.Timeframe1d, 2 * (24 * time.Hour / 4)},    // 12h
	}
	for _, tc := range cases {
		got := domain.EvaluationStaleAfter(tc.tf)
		if got != tc.want {
			t.Errorf("%s: StaleAfter=%v want %v (refresh=%v)",
				tc.tf, got, tc.want, domain.EvaluationRefreshInterval(tc.tf))
		}
	}
}

func TestEvaluationStaleAfter_FloorAppliesToShortTF(t *testing.T) {
	// 1m → refresh floors at 30s → stale = 1m
	got := domain.EvaluationStaleAfter(domain.Timeframe1m)
	if got != time.Minute {
		t.Fatalf("1m StaleAfter=%v want 1m", got)
	}
}

func TestEvaluationStoreFresh(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tf := domain.Timeframe1h
	staleAfter := domain.EvaluationStaleAfter(tf)

	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"fresh recent", now.Add(-time.Minute), true},
		{"exact boundary", now.Add(-staleAfter), true},
		{"just stale", now.Add(-(staleAfter + time.Second)), false},
		{"future at", now.Add(time.Minute), false},
		{"zero at", time.Time{}, false},
	}
	for _, tc := range cases {
		if got := domain.EvaluationStoreFresh(tc.at, now, tf); got != tc.want {
			t.Errorf("%s: Fresh=%v want %v", tc.name, got, tc.want)
		}
	}
}
