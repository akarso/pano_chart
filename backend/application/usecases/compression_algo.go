package usecases

import (
	"pano_chart/backend/domain/scoring"
)

// CompressionAlgoMode selects the compression score engine (PR-105).
type CompressionAlgoMode string

const (
	CompressionAlgoAbsolute   CompressionAlgoMode = "absolute"
	CompressionAlgoPercentile CompressionAlgoMode = "percentile"
)

// ParseCompressionAlgo normalises s to a CompressionAlgoMode.
// ok is false for unrecognized non-empty values.
// Empty string is a valid default → absolute, ok=true.
func ParseCompressionAlgo(s string) (CompressionAlgoMode, bool) {
	switch CompressionAlgoMode(s) {
	case "", CompressionAlgoAbsolute:
		return CompressionAlgoAbsolute, true
	case CompressionAlgoPercentile:
		return CompressionAlgoPercentile, true
	default:
		return CompressionAlgoAbsolute, false
	}
}

// CompressionCalcFor returns the compression calculator for the selected algo.
// Absolute uses the legacy structural detector; percentile uses history ranks.
func CompressionCalcFor(algo CompressionAlgoMode) scoring.SymbolScoreCalculator {
	if algo == CompressionAlgoPercentile {
		return &scoring.CompressionPercentileScoreCalculator{
			Legacy: scoring.DefaultCompressionConfig(),
		}
	}
	return &scoring.CompressionScoreCalculator{
		Config: scoring.DefaultCompressionConfig(),
	}
}
