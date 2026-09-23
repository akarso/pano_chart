package market_test

import (
	"testing"

	appmarket "pano_chart/backend/application/market"
	mkt "pano_chart/backend/domain/market"
	fixtures "pano_chart/backend/tests/fixtures/candles"
)

// TestGoldenTape_MessyUptrendFavorsTrend is the PR-084 / PR-115 regression
// guard: a tape that nets +8–12% with chop must read TREND via TapeTrend ≥ 0.5
// (not a mix-share promotion, and not health-dampened into sideways).
func TestGoldenTape_MessyUptrendFavorsTrend(t *testing.T) {
	series := fixtures.Load(t, "messy_uptrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "golden_fixture")
	if tape.State != mkt.StateTrend {
		t.Fatalf("messy_uptrend: want TREND, got %s score=%.3f structure=%+v", tape.State, tape.TrendScore, tape.Structure)
	}
	if tape.TrendScore < 0.5 {
		t.Fatalf("messy_uptrend: TapeTrend=%.3f want ≥0.5", tape.TrendScore)
	}
	if tape.Structure.Trend <= tape.Structure.Sideways {
		t.Fatalf("messy_uptrend: want Structure.Trend > Sideways, got Trend=%.4f Sideways=%.4f state=%s",
			tape.Structure.Trend, tape.Structure.Sideways, tape.State)
	}
	if tape.Bias != "up" {
		t.Fatalf("messy_uptrend: want bias=up, got %q", tape.Bias)
	}
}

// TestGoldenTape_CleanDowntrendFavorsTrend is the down-path twin.
func TestGoldenTape_CleanDowntrendFavorsTrend(t *testing.T) {
	series := fixtures.Load(t, "clean_downtrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "golden_fixture")
	if tape.State != mkt.StateTrend {
		t.Fatalf("clean_downtrend: want TREND, got %s score=%.3f", tape.State, tape.TrendScore)
	}
	if tape.TrendScore < 0.5 {
		t.Fatalf("clean_downtrend: TapeTrend=%.3f want ≥0.5", tape.TrendScore)
	}
	if tape.Structure.Trend <= tape.Structure.Sideways {
		t.Fatalf("clean_downtrend: want Structure.Trend > Sideways, got Trend=%.4f Sideways=%.4f state=%s effT=%.3f",
			tape.Structure.Trend, tape.Structure.Sideways, tape.State, tape.EffectiveTrend)
	}
	if tape.Bias != "down" {
		t.Fatalf("clean_downtrend: want bias=down, got %q", tape.Bias)
	}
}
