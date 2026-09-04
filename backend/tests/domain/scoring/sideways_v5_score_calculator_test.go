package scoring_test

import (
	"math"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"testing"
	"time"
)

func TestSidewaysV5_StableSidewaysRanksHighest(t *testing.T) {
	// Arrange: create synthetic candle series with stable sideways structure
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	cycles := 5.0
	for i := range candles {
		phase := 2 * math.Pi * cycles * float64(i) / 110.0
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+1.0+0.5*math.Sin(phase),
			base-1.0+0.5*math.Sin(phase),
			base+0.2*math.Sin(phase),
			1000,
		)
	}
	cfg := scoring.SidewaysV5Config{N: 6, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}
	res := scoring.DetectSidewaysV5(candles, cfg)
	// Assert: score is high, spikeDetected is false, components normalized
	if res.Score < 0.7 {
		t.Errorf("Expected high score for stable sideways, got %v", res.Score)
	}
	if res.SpikeDetected {
		t.Errorf("Expected no spike detected for stable sideways")
	}
	for k, v := range res.Components {
		if v < 0 || v > 1 {
			t.Errorf("Component %s not normalized: %v", k, v)
		}
	}
}

func TestSidewaysV5_RecoveredSidewaysRanksSlightlyLower(t *testing.T) {
	// Arrange: create candle series with spike, then recovery
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+1.0+0.5*math.Sin(float64(i)/10),
			base-1.0+0.5*math.Sin(float64(i)/10),
			base+0.2*math.Sin(float64(i)/10),
			1000,
		)
	}
	// Insert spike
	spikeIdx := 55
	orig := candles[spikeIdx]
	candles[spikeIdx] = domain.NewCandleUnsafe(
		orig.Symbol(),
		orig.Timeframe(),
		orig.Timestamp(),
		orig.Open(),
		base+10.0, // High
		base-10.0, // Low
		orig.Close(),
		orig.Volume(),
	)
	cfg := scoring.SidewaysV5Config{N: 3, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}
	res := scoring.DetectSidewaysV5(candles, cfg)
	if res.Score > 0.95 {
		t.Errorf("Recovered sideways should not exceed stable score, got %v", res.Score)
	}
	if !res.SpikeDetected {
		t.Errorf("Expected spike detected for recovered sideways")
	}
	if srm, ok := res.Components["SRM"]; !ok || srm >= 1.0 {
		t.Errorf("SRM should be < 1.0 for recovered sideways, got %v", srm)
	}
}

func TestSidewaysV5_BrokenStructureHeavilyPenalized(t *testing.T) {
	// Arrange: create candle series with structural break after spike
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+1.0+0.5*math.Sin(float64(i)/10),
			base-1.0+0.5*math.Sin(float64(i)/10),
			base+0.2*math.Sin(float64(i)/10),
			1000,
		)
	}
	// Insert spike
	spikeIdx := 55
	orig := candles[spikeIdx]
	candles[spikeIdx] = domain.NewCandleUnsafe(
		orig.Symbol(),
		orig.Timeframe(),
		orig.Timestamp(),
		orig.Open(),
		base+10.0, // High
		base-10.0, // Low
		orig.Close(),
		orig.Volume(),
	)
	// After spike, break structure
	for i := spikeIdx + 1; i < len(candles); i++ {
		orig := candles[i]
		candles[i] = domain.NewCandleUnsafe(
			orig.Symbol(),
			orig.Timeframe(),
			orig.Timestamp(),
			orig.Open(),
			base+5.0+float64(i-spikeIdx), // High
			base-5.0-float64(i-spikeIdx), // Low
			orig.Close(),
			orig.Volume(),
		)
	}
	cfg := scoring.SidewaysV5Config{N: 3, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}
	res := scoring.DetectSidewaysV5(candles, cfg)
	if res.Score > 0.5 {
		t.Errorf("Broken structure should be heavily penalized, got %v", res.Score)
	}
	if srm, ok := res.Components["SRM"]; !ok || srm > 0.5 {
		t.Errorf("SRM should be <= 0.5 for broken structure, got %v", srm)
	}
}

func TestSidewaysV5_MicroFlatNoiseRejected(t *testing.T) {
	// Arrange: create candle series with micro-flat noise
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+0.01,
			base-0.01,
			base,
			1000,
		)
	}
	cfg := scoring.SidewaysV5Config{N: 3, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0, ExtremaCount: 8}
	res := scoring.DetectSidewaysV5(candles, cfg)
	if res.Score > 0.3 {
		t.Errorf("Micro-flat noise should be rejected, got %v", res.Score)
	}
	if vos, ok := res.Components["VOS"]; !ok || vos > 0.3 {
		t.Errorf("VOS should be low for micro-flat noise, got %v", vos)
	}
}

func TestSidewaysV5_HighVolatilityChaosRejected(t *testing.T) {
	// Arrange: create candle series with chaotic expansion
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+float64(i),
			base-float64(i),
			base+float64(i)/2,
			1000,
		)
	}
	cfg := scoring.SidewaysV5Config{N: 3, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0, ExtremaCount: 8}
	res := scoring.DetectSidewaysV5(candles, cfg)
	if res.Score > 0.3 {
		t.Errorf("High-volatility chaos should be rejected, got %v", res.Score)
	}
	if vos, ok := res.Components["VOS"]; !ok || vos > 0.3 {
		t.Errorf("VOS should be low for high-volatility chaos, got %v", vos)
	}
}

// TestDetectSidewaysV5_NonParallelChannel is the regression test for
// PR-074: DetectSidewaysV5 used to fit both channel boundaries through the
// same combined highs+lows point set, making upperSlope == lowerSlope
// always and CCS's parallelScore component a guaranteed 1.0 regardless of
// the real channel shape. A visibly widening channel (upper boundary
// trending up, lower boundary flat) must now score CCS meaningfully below
// a genuinely parallel channel's — before the fix both scored identically
// (parallelScore == 1.0 either way).
func TestDetectSidewaysV5_NonParallelChannel(t *testing.T) {
	buildChannel := func(upperDriftPerCandle float64) []domain.Candle {
		candles := make([]domain.Candle, 110)
		base := 100.0
		sym, _ := domain.NewSymbol("TEST")
		tf := domain.Timeframe1h
		cycles := 5.0
		for i := range candles {
			phase := 2 * math.Pi * cycles * float64(i) / 110.0
			drift := upperDriftPerCandle * float64(i)
			candles[i] = domain.NewCandleUnsafe(
				sym,
				tf,
				time.Unix(int64(i)*3600, 0).UTC(),
				base,
				base+1.0+drift+0.5*math.Sin(phase), // upper boundary: drifts up over time
				base-1.0+0.5*math.Sin(phase),       // lower boundary: flat
				base+0.2*math.Sin(phase),
				1000,
			)
		}
		return candles
	}

	cfg := scoring.SidewaysV5Config{N: 6, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}

	parallel := scoring.DetectSidewaysV5(buildChannel(0), cfg)
	widening := scoring.DetectSidewaysV5(buildChannel(0.015), cfg)

	parallelCCS, ok := parallel.Components["CCS"]
	if !ok {
		t.Fatal("expected CCS component in parallel-channel result")
	}
	wideningCCS, ok := widening.Components["CCS"]
	if !ok {
		t.Fatal("expected CCS component in widening-channel result")
	}

	if wideningCCS >= parallelCCS {
		t.Errorf("expected widening channel's CCS (%v) to score below the parallel channel's (%v) — "+
			"before the fix both boundaries were fit through the same point set and always scored identically",
			wideningCCS, parallelCCS)
	}
}

func TestSidewaysV5_ComponentsNormalizedAndOutput(t *testing.T) {
	// Arrange: create generic sideways structure
	candles := make([]domain.Candle, 110)
	base := 100.0
	sym, _ := domain.NewSymbol("TEST")
	tf := domain.Timeframe1h
	for i := range candles {
		candles[i] = domain.NewCandleUnsafe(
			sym,
			tf,
			time.Unix(int64(i)*3600, 0).UTC(),
			base,
			base+1.0+0.5*math.Sin(float64(i)/10),
			base-1.0+0.5*math.Sin(float64(i)/10),
			base+0.2*math.Sin(float64(i)/10),
			1000,
		)
	}
	cfg := scoring.SidewaysV5Config{N: 3, CandleCount: 110, IdealATRRange: 3.0, RangeTolerance: 1.5, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}
	res := scoring.DetectSidewaysV5(candles, cfg)
	for k, v := range res.Components {
		if v < 0 || v > 1 {
			t.Errorf("Component %s not normalized: %v", k, v)
		}
	}
	if res.Score < 0 || res.Score > 1 {
		t.Errorf("Score not normalized: %v", res.Score)
	}
}
