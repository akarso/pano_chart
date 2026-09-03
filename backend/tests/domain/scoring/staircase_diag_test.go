package scoring_test

import (
	"math"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"testing"
	"time"
)

// TestSidewaysV5_StaircaseRejected verifies that a _/‾ pattern
// (flat-bottom → slope → flat-top) is NOT scored as sideways.
// Both flat regions have identical oscillation amplitude and produce
// valid extrema, but they are at different price levels — this is a
// trend (staircase), not a horizontal channel.
func TestSidewaysV5_StaircaseRejected(t *testing.T) {
	const n = 110
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	candles := make([]domain.Candle, n)

	for i := 0; i < n; i++ {
		ts := time.Unix(int64(i)*3600, 0).UTC()
		var center float64
		switch {
		case i < 37: // first 1/3: oscillate at 100
			center = 100.0 + 0.8*math.Sin(float64(i)*2*math.Pi/12)
		case i < 74: // middle 1/3: slope from 100 to 106
			frac := float64(i-37) / 37.0
			center = 100.0 + 6.0*frac
		default: // last 1/3: oscillate at 106
			center = 106.0 + 0.8*math.Sin(float64(i)*2*math.Pi/12)
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, ts,
			center, center+0.5, center-0.5, center+0.1, 1000,
		)
	}

	cfg := scoring.SidewaysV5Config{
		N: 3, CandleCount: 110, IdealATRRange: 4.0, RangeTolerance: 1.5,
		ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0, ExtremaCount: 6,
	}
	res := scoring.DetectSidewaysV5(candles, cfg)

	if res.Score > 0.01 {
		t.Errorf("Staircase _/‾ should be rejected (score ≈ 0), got %.4f", res.Score)
	}
}
