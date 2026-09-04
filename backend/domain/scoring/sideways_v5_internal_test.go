package scoring

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/domain"
)

// TestStddevFromLine_UsesRealIndicesNotLoopPosition is a direct, in-package
// unit test for the PR-074 CR blocker: stddevFromLine must evaluate the
// fitted line at each point's true chronological index (matching how
// regressionThroughHighs/Lows fit it), not at its loop position within a
// compacted candle slice. Black-box tests through DetectSidewaysV5 don't
// reliably catch this — the composite Score averages several components
// and many realistic channel shapes have a near-zero slope, which makes
// the mismatch invisible (predicted ≈ intercept regardless of x). This
// test isolates the function with widely-spaced real indices and a
// deliberately nonzero slope, where the mismatch is unmissable.
func TestStddevFromLine_UsesRealIndicesNotLoopPosition(t *testing.T) {
	const slope = 0.5
	const intercept = 100.0
	// Real chronological indices, widely spaced — e.g. sparse extrema
	// across a 110-candle series (the CR's "typical" scenario).
	indices := []int{0, 50, 100}

	candles := make([]domain.Candle, len(indices))
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i, idx := range indices {
		// Placed exactly on the line at its real index: y = slope*idx + intercept.
		y := slope*float64(idx) + intercept
		candles[i] = domain.NewCandleUnsafe(sym, tf, time.Unix(int64(i)*3600, 0).UTC(), y, y, y, y, 1000)
	}

	got := stddevFromLine(candles, indices, slope, intercept)
	if got > 1e-9 {
		t.Errorf("expected ~0 deviation for points exactly on the line at their real indices, got %v", got)
	}

	// Sanity check the mechanism: evaluating the SAME candles against their
	// compacted loop position (0,1,2) instead of their real index (0,50,100)
	// — the bug this test guards against — must NOT still read as ~0,
	// otherwise this test wouldn't actually be sensitive to the regression.
	compactedIndices := []int{0, 1, 2}
	buggy := stddevFromLine(candles, compactedIndices, slope, intercept)
	if buggy < 1.0 {
		t.Fatalf("test setup invalid: expected evaluating at compacted indices to produce a large deviation (demonstrating this test is sensitive to the fix), got %v", buggy)
	}
	if math.Abs(got-buggy) < 1.0 {
		t.Errorf("expected real-index and compacted-index evaluation to diverge sharply for widely-spaced points; got real=%v compacted=%v", got, buggy)
	}
}
