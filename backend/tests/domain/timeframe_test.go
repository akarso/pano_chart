package domain_test

import (
	"testing"
	"time"

	"github.com/akarso/pano_chart/backend/domain"
)

func TestTimeframe_AcceptsSupportedValues(t *testing.T) {
	supportedValues := []string{"1m", "5m", "15m", "1h", "4h", "1d"}

	for _, value := range supportedValues {
		t.Run(value, func(t *testing.T) {
			tf, err := domain.NewTimeframe(value)
			if err != nil {
				t.Fatalf("NewTimeframe(%q) returned error: %v", value, err)
			}

			if tf.String() != value {
				t.Errorf("expected %q, got %q", value, tf.String())
			}
		})
	}
}

func TestTimeframe_RejectsEmptyString(t *testing.T) {
	_, err := domain.NewTimeframe("")
	if err == nil {
		t.Error("NewTimeframe(\"\") should return error, got nil")
	}
}

func TestTimeframe_RejectsUnsupportedValues(t *testing.T) {
	unsupported := []string{
		"2m", "30m", "0m", "1hour", "15",
		"10m", "2h", "12h", "3d",
		"1ms", "1s", "1w", "  ",
	}

	for _, value := range unsupported {
		t.Run("reject_"+value, func(t *testing.T) {
			_, err := domain.NewTimeframe(value)
			if err == nil {
				t.Errorf("NewTimeframe(%q) should reject unsupported value, got nil error", value)
			}
		})
	}
}

func TestTimeframe_NormalizesCanonicalForm(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1m", "1m"},
		{"5m", "5m"},
		{"15m", "15m"},
		{"1h", "1h"},
		{"4h", "4h"},
		{"1d", "1d"},
		{"1M", "1m"},
		{"5M", "5m"},
		{"15M", "15m"},
		{"1H", "1h"},
		{"4H", "4h"},
		{"1D", "1d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tf, err := domain.NewTimeframe(tt.input)
			if err != nil {
				t.Fatalf("NewTimeframe(%q) returned error: %v", tt.input, err)
			}

			if tf.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tf.String())
			}
		})
	}
}

func TestTimeframe_EqualityByValue(t *testing.T) {
	tf1, _ := domain.NewTimeframe("1m")
	tf2, _ := domain.NewTimeframe("1M")
	tf3, _ := domain.NewTimeframe("5m")

	// Same canonical value should be equal
	if tf1 != tf2 {
		t.Errorf("timeframes with same canonical value should be equal: %q vs %q", tf1.String(), tf2.String())
	}

	// Different canonical values should not be equal
	if tf1 == tf3 {
		t.Errorf("timeframes with different values should not be equal: %q vs %q", tf1.String(), tf3.String())
	}
}

func TestTimeframe_ExposesCorrectDuration(t *testing.T) {
	tests := []struct {
		value    string
		duration time.Duration
	}{
		{"1m", 1 * time.Minute},
		{"5m", 5 * time.Minute},
		{"15m", 15 * time.Minute},
		{"1h", 1 * time.Hour},
		{"4h", 4 * time.Hour},
		{"1d", 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			tf, err := domain.NewTimeframe(tt.value)
			if err != nil {
				t.Fatalf("NewTimeframe(%q) returned error: %v", tt.value, err)
			}

			duration := tf.Duration()
			if duration != tt.duration {
				t.Errorf("expected duration %v, got %v", tt.duration, duration)
			}
		})
	}
}

func TestTimeframe_IsImmutable(t *testing.T) {
	tf, _ := domain.NewTimeframe("1m")
	original := tf.String()

	tf2, _ := domain.NewTimeframe(original)
	if tf.String() != tf2.String() {
		t.Errorf("timeframe should remain unchanged: %q vs %q", original, tf.String())
	}
}
