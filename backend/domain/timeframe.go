package domain

import (
	"fmt"
	"strings"
	"time"
)

// Timeframe represents a candle aggregation interval.
// It is a value object that is immutable and supports a fixed set of canonical values.
type Timeframe string

// Supported canonical timeframe values
const (
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe15m Timeframe = "15m"
	Timeframe1h  Timeframe = "1h"
	Timeframe4h  Timeframe = "4h"
	Timeframe1d  Timeframe = "1d"
)

// validTimeframes is the set of supported timeframe values
var validTimeframes = map[string]Timeframe{
	"1m":  Timeframe1m,
	"5m":  Timeframe5m,
	"15m": Timeframe15m,
	"1h":  Timeframe1h,
	"4h":  Timeframe4h,
	"1d":  Timeframe1d,
}

// NewTimeframe creates a new Timeframe from a string.
// The timeframe is normalized to lowercase and validated against supported values.
// Returns an error if the timeframe is invalid or unsupported.
func NewTimeframe(s string) (Timeframe, error) {
	// Reject empty string
	if s == "" {
		return "", fmt.Errorf("timeframe cannot be empty")
	}

	// Normalize to lowercase
	normalized := strings.ToLower(strings.TrimSpace(s))

	// Validate against supported values
	if tf, ok := validTimeframes[normalized]; ok {
		return tf, nil
	}

	return "", fmt.Errorf("unsupported timeframe: %q", s)
}

// String returns the canonical string representation of the Timeframe.
func (tf Timeframe) String() string {
	return string(tf)
}

// Duration returns the time.Duration equivalent of the Timeframe.
func (tf Timeframe) Duration() time.Duration {
	switch tf {
	case Timeframe1m:
		return 1 * time.Minute
	case Timeframe5m:
		return 5 * time.Minute
	case Timeframe15m:
		return 15 * time.Minute
	case Timeframe1h:
		return 1 * time.Hour
	case Timeframe4h:
		return 4 * time.Hour
	case Timeframe1d:
		return 24 * time.Hour
	default:
		// This should never happen if validation is correct
		return 0
	}
}

// NewTimeframeUnsafe creates a Timeframe without validation.
// Use only in tests.
func NewTimeframeUnsafe(s string) Timeframe {
	return Timeframe(strings.ToLower(s))
}
