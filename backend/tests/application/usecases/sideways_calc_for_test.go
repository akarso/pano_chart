package usecases

import (
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestSidewaysCalcFor_V3_DiffersByTimeframe(t *testing.T) {
	// Algo-override path must use TimeframeAwareCalculator — V3 bakes
	// RMin/RMax at construction with no Score-time re-resolve.
	calc := usecases.SidewaysCalcFor(usecases.SidewaysAlgoV3)
	if calc.Name() != "Sideways Consistency" {
		t.Fatalf("Name() = %q", calc.Name())
	}

	s15, err := calc.Score(v3SidewaysSeries(t, "15m"))
	if err != nil {
		t.Fatalf("15m: %v", err)
	}
	s1d, err := calc.Score(v3SidewaysSeries(t, "1d"))
	if err != nil {
		t.Fatalf("1d: %v", err)
	}
	if s15 == 0 {
		t.Fatal("expected non-zero 15m score for in-band channel")
	}
	if s1d != 0 {
		t.Fatalf("expected zero 1d score (below RMin), got %f", s1d)
	}

	// Hardcoded-"1h" calculator would produce the same score for both TFs.
	fixed := &scoring.SidewaysV3ScoreCalculator{
		Config: scoring.DefaultSidewaysV3Config("1h"),
	}
	fixed15, _ := fixed.Score(v3SidewaysSeries(t, "15m"))
	fixed1d, _ := fixed.Score(v3SidewaysSeries(t, "1d"))
	if fixed15 != fixed1d {
		t.Fatalf("control: fixed 1h config should ignore series TF; got %f vs %f", fixed15, fixed1d)
	}
}

// v3SidewaysSeries builds a mild range (~1.2% channel) that sits inside
// SidewaysV3's 15m RMin/RMax band but below the 1d RMin — so a TF-aware
// calculator scores them differently while a hardcoded "1h" config does not.
func v3SidewaysSeries(t *testing.T, tfStr string) domain.CandleSeries {
	t.Helper()
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		t.Fatal(err)
	}
	sym := domain.NewSymbolUnsafe("TESTUSDT")
	n := 80
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	step := tf.Duration()
	const mid = 100.0
	const half = 0.6 // envelope ~1.2% of mid — 15m in-band, 1d below RMin
	for i := 0; i < n; i++ {
		// Alternate near upper / lower edges so balance + containment pass.
		nearUpper := i%2 == 0
		c := mid - half + 0.05
		if nearUpper {
			c = mid + half - 0.05
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*step),
			c, mid+half, mid-half, c, 1000,
		)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return series
}
