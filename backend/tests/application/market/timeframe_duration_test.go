package market_test

import (
	"testing"

	"pano_chart/backend/application/market/transition"
)

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		tf      string
		candles int
		want    string
	}{
		// Minutes range.
		{"1m", 5, "5m"},
		{"5m", 10, "50m"},
		// Hours range.
		{"1h", 5, "5h"},
		{"4h", 3, "12h"},
		{"1h", 1, "1h"},
		{"15m", 5, "1.2h"},
		// Days range.
		{"4h", 12, "2d"},
		{"4h", 20, "3.3d"},
		{"1d", 3, "3d"},
		{"1h", 48, "2d"},
		// Weeks range.
		{"1d", 14, "2w"},
		{"4h", 42, "1w"},
		// Edge cases.
		{"unknown", 10, ""},
		{"4h", 0, ""},
		{"4h", -1, ""},
	}
	for _, tc := range tests {
		got := transition.HumanDuration(tc.tf, tc.candles)
		if got != tc.want {
			t.Errorf("HumanDuration(%q, %d) = %q, want %q", tc.tf, tc.candles, got, tc.want)
		}
	}
}
