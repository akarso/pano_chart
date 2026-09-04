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

// TestDetectSidewaysV5_UpperLowerSlopesFitIndependently directly verifies
// the PR-074 boundary-fit correction: for a channel whose upper boundary
// visibly drifts up over time while the lower boundary stays flat,
// regressionThroughHighs and regressionThroughLows (called with the real
// highsIdx/lowsIdx from detectExtrema, exactly as DetectSidewaysV5 calls
// them) must fit two genuinely different slopes. Before the fix both
// boundaries were fit through the same combined highs+lows point set via
// linearRegression, making upperSlope == lowerSlope always — this isolates
// that specific correction rather than inferring it from the aggregate CCS
// score, which also mixes in deviationScore/widthStabilityScore and so
// isn't identical before/after purely because of parallelScore.
func TestDetectSidewaysV5_UpperLowerSlopesFitIndependently(t *testing.T) {
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	cycles := 5.0
	const upperDriftPerCandle = 0.015
	for i := range candles {
		phase := 2 * math.Pi * cycles * float64(i) / 110.0
		drift := upperDriftPerCandle * float64(i)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+1.0+drift+0.5*math.Sin(phase), // upper boundary: drifts up over time
			base-1.0+0.5*math.Sin(phase),       // lower boundary: flat
			base+0.2*math.Sin(phase),
			1000,
		)
	}

	cfg := NewSidewaysV5ConfigForTimeframe("1h")
	highsIdx, lowsIdx, _ := detectExtrema(candles, cfg.N)
	if len(highsIdx) < 2 || len(lowsIdx) < 2 {
		t.Fatalf("test setup invalid: need at least 2 highs and lows, got %d highs, %d lows", len(highsIdx), len(lowsIdx))
	}

	upperSlope, _ := regressionThroughHighs(candles, highsIdx)
	lowerSlope, _ := regressionThroughLows(candles, lowsIdx)

	if upperSlope <= lowerSlope {
		t.Errorf("expected upperSlope (%v) > lowerSlope (%v) for a channel whose upper boundary drifts up while the lower stays flat", upperSlope, lowerSlope)
	}
}

// TestQuickStructure_InsufficientHighsOrLows_ReturnsZero is a direct
// regression test for the PR-074 CR follow-up: quickStructure (used to
// evaluate the pre/post halves of a spike for SRM recovery scoring) must
// not fall through to regressionThroughHighs/Lows's degenerate origin-line
// default when a half has fewer than 2 highs or lows. Before this guard,
// TestSidewaysV5_BrokenStructureHeavilyPenalized (a black-box test through
// the full DetectSidewaysV5 pipeline) still passed by coincidence — the
// degenerate math happened to also produce a low score for that specific
// input — which is why this needs a direct, deterministic check rather
// than relying on that test alone.
func TestQuickStructure_InsufficientHighsOrLows_ReturnsZero(t *testing.T) {
	cfg := SidewaysV5Config{N: 6}
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	// Fewer candles than detectExtrema's window (N=6) needs on either side
	// of a candidate extremum — guarantees zero highs/lows/extrema found.
	candles := make([]domain.Candle, 5)
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(sym, tf, time.Unix(int64(i)*3600, 0).UTC(), 100, 101, 99, 100, 1000)
	}

	ccs, oqs, dcs := quickStructure(candles, cfg)
	if ccs != 0 || oqs != 0 || dcs != 0 {
		t.Errorf("expected (0, 0, 0) for insufficient highs/lows, got (%v, %v, %v)", ccs, oqs, dcs)
	}
}
