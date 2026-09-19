package market_test

import (
	"testing"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/tests/fixtures/candles"
)

// TestGoldenTape_MessyUptrendFavorsTrend is the PR-084 regression guard:
// a tape that nets +8–12% with chop must still read Trend-dominant over
// Sideways (not get misclassified as a range).
func TestGoldenTape_MessyUptrendFavorsTrend(t *testing.T) {
	series := fixtures.Load(t, "messy_uptrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "golden_fixture")
	if tape.Structure.Trend <= tape.Structure.Sideways {
		t.Fatalf("messy_uptrend: want Structure.Trend > Sideways, got Trend=%.4f Sideways=%.4f state=%s",
			tape.Structure.Trend, tape.Structure.Sideways, tape.State)
	}
	if tape.Bias != "up" {
		t.Fatalf("messy_uptrend: want bias=up, got %q", tape.Bias)
	}
}

// TestGoldenTape_CleanDowntrendFavorsTrend is the down-path twin of the
// PR-084 guard. A smooth −10% grind must stay Trend-dominant with bias
// down — not get health-dampened into sideways because the window return
// is largely negative in ATR units (that return *is* the downtrend).
func TestGoldenTape_CleanDowntrendFavorsTrend(t *testing.T) {
	series := fixtures.Load(t, "clean_downtrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "golden_fixture")
	if tape.Structure.Trend <= tape.Structure.Sideways {
		t.Fatalf("clean_downtrend: want Structure.Trend > Sideways, got Trend=%.4f Sideways=%.4f state=%s effT=%.3f",
			tape.Structure.Trend, tape.Structure.Sideways, tape.State, tape.EffectiveTrend)
	}
	if tape.Bias != "down" {
		t.Fatalf("clean_downtrend: want bias=down, got %q", tape.Bias)
	}
}
