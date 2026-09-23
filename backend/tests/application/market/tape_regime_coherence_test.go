package market_test

import (
	"math"
	"testing"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/tests/testutil"
)

func TestTapeRegimeCoherence_Uptrends(t *testing.T) {
	cases := []struct {
		name string
		spec testutil.TrendSeriesSpec
	}{
		{"clean_+12%", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, Seed: 7}},
		{"+12%_1.5%_pullback", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, TailPullback: 0.015, Seed: 7}},
		{"+12%_3%_pullback", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, TailPullback: 0.03, Seed: 7}},
		{"+6%_1%_pullback", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.06, TailPullback: 0.01, Seed: 7}},
		{"noisy_+12%", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, Noise: 0.6, Seed: 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series, err := testutil.BuildTrendSeries(tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			tape := appmarket.ScoreMarketTape(series, "4h", "composite_median")
			if tape.State != mkt.StateTrend {
				t.Fatalf("state=%s structure=%+v trendScore=%.3f", tape.State, tape.Structure, tape.TrendScore)
			}
			if tape.TrendScore < 0.5 {
				t.Fatalf("TREND without TapeTrend gate: score=%.3f", tape.TrendScore)
			}
			if tape.Confidence != tape.TrendScore {
				t.Fatalf("confidence=%.3f want raw TapeTrend %.3f", tape.Confidence, tape.TrendScore)
			}
			if tape.Bias != "up" {
				t.Fatalf("bias=%s", tape.Bias)
			}
			if tape.Structure.Trend < 0.6 {
				t.Fatalf("Structure.Trend=%.3f want ≥0.6", tape.Structure.Trend)
			}
			if tape.Structure.Sideways > 0.2 {
				t.Fatalf("Structure.Sideways=%.3f want ≤0.2", tape.Structure.Sideways)
			}
			if tape.Label == "No clear trend" || tape.Label == "Mixed conditions" {
				t.Fatalf("label=%q", tape.Label)
			}
			if tape.WindowBars != series.Len() {
				t.Fatalf("windowBars=%d want %d", tape.WindowBars, series.Len())
			}
		})
	}
}

func TestTapeRegimeCoherence_Downtrends(t *testing.T) {
	cases := []struct {
		name string
		spec testutil.TrendSeriesSpec
	}{
		{"clean_-12%", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, Down: true, Seed: 7}},
		{"-12%_1.5%_bounce", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.12, TailPullback: 0.015, Down: true, Seed: 7}},
		{"-6%_1%_bounce", testutil.TrendSeriesSpec{Bars: 110, NetReturn: 0.06, TailPullback: 0.01, Down: true, Seed: 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series, err := testutil.BuildTrendSeries(tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			tape := appmarket.ScoreMarketTape(series, "4h", "composite_median")
			if tape.State != mkt.StateTrend {
				t.Fatalf("state=%s structure=%+v", tape.State, tape.Structure)
			}
			if tape.Bias != "down" {
				t.Fatalf("bias=%s", tape.Bias)
			}
			if tape.Structure.Trend < 0.6 {
				t.Fatalf("Structure.Trend=%.3f", tape.Structure.Trend)
			}
		})
	}
}

func TestTapeRegimeCoherence_NoSidewaysDump(t *testing.T) {
	series, err := testutil.BuildTrendSeries(testutil.TrendSeriesSpec{
		Bars: 110, NetReturn: 0.12, Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(series, "4h", "composite_median")
	if tape.Structure.Sideways > 0.05 {
		t.Fatalf("Structure.Sideways=%.4f — measured mix must stay near 0", tape.Structure.Sideways)
	}
}

func TestTapeRegimeCoherence_TightRange(t *testing.T) {
	series, err := testutil.BuildTightRange(110, 7)
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(series, "4h", "composite_median")
	if tape.State == mkt.StateTrend {
		t.Fatalf("tight range classified as trend: %+v score=%.3f", tape.Structure, tape.TrendScore)
	}
	if tape.Label == "Strong trend" || tape.Label == "Trend weakening" {
		t.Fatalf("trend caption on non-TREND: %q", tape.Label)
	}
}

func TestTapeRegimeCoherence_RandomWalk(t *testing.T) {
	var trendHits int
	var scoreSum float64
	const seeds = 100
	for seed := int64(1); seed <= seeds; seed++ {
		series, err := testutil.BuildRandomWalk(110, 100, seed)
		if err != nil {
			t.Fatal(err)
		}
		tape := appmarket.ScoreMarketTape(series, "4h", "composite_median")
		scoreSum += tape.TrendScore
		if tape.State == mkt.StateTrend {
			trendHits++
			if tape.TrendScore < 0.5 {
				t.Fatalf("seed %d: TREND with score=%.3f", seed, tape.TrendScore)
			}
		}
	}
	mean := scoreSum / float64(seeds)
	if trendHits > 30 {
		t.Fatalf("random walk trend hits=%d/100 want ≤30", trendHits)
	}
	if mean > 0.3 {
		t.Fatalf("mean trendScore=%.3f want ≤0.3", mean)
	}
}

func TestTapeRegimeCoherence_HealthTwoATR(t *testing.T) {
	h := appmarket.ComputeTrendHealthV2("uptrend", 100, 110, 90, 5, 0.5, 0)
	if math.Abs(h-0.6) > 0.01 {
		t.Fatalf("health=%.3f want ≈0.6", h)
	}
	if got := appmarket.BuildTapeLabel(mkt.StateTrend, h); got != "Trend weakening" {
		t.Fatalf("label=%q want Trend weakening", got)
	}

	// Freeze ATR before rewriting the last bar so Wilder does not absorb the dump.
	series, err := testutil.BuildTrendSeries(testutil.TrendSeriesSpec{
		Bars: 110, NetReturn: 0.12, Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	candles := append([]domain.Candle(nil), series.All()...)
	n := len(candles)
	atrFrozen := scoring.TrueATR(candles, 14)
	hi := candles[0].High()
	for i := 0; i < n-1; i++ {
		if candles[i].High() > hi {
			hi = candles[i].High()
		}
	}
	last := candles[n-1]
	dropped := hi - 2*atrFrozen
	// Keep the last bar's true range small so ATR barely moves.
	candles[n-1] = domain.NewCandleUnsafe(
		last.Symbol(), last.Timeframe(), last.Timestamp(),
		dropped, dropped+0.01, dropped-0.01, dropped, last.Volume(),
	)
	pulled, err := domain.NewCandleSeries(series.Symbol(), series.Timeframe(), candles)
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(pulled, "4h", "composite_median")
	if tape.State != mkt.StateTrend {
		t.Fatalf("state=%s after 2 ATR pullback (want trend)", tape.State)
	}
	if math.Abs(tape.EffectiveTrend-0.6) > 0.05 {
		t.Fatalf("EffectiveTrend=%.3f want ≈0.6 (frozenATR=%.4f)", tape.EffectiveTrend, atrFrozen)
	}
	if tape.Label != "Trend weakening" {
		t.Fatalf("label=%q want Trend weakening (health=%.3f)", tape.Label, tape.EffectiveTrend)
	}
}

func TestTapeRegimeCoherence_CrashTailPenalty(t *testing.T) {
	// Full-window net still up, but the last 8 bars dump > 1.5 ATR against the trend.
	hHealthy := appmarket.ComputeTrendHealthV2("uptrend", 110, 110, 90, 5, 0.2, 0)
	hCrash := appmarket.ComputeTrendHealthV2("uptrend", 110, 110, 90, 5, -2.0, 0)
	if math.Abs(hHealthy-1.0) > 0.01 {
		t.Fatalf("healthy=%f", hHealthy)
	}
	if math.Abs(hCrash-0.3) > 0.01 {
		t.Fatalf("crash=%f want ~0.3", hCrash)
	}

	series, err := testutil.BuildTrendSeries(testutil.TrendSeriesSpec{
		Bars: 110, NetReturn: 0.12, Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	candles := append([]domain.Candle(nil), series.All()...)
	atr14 := scoring.TrueATR(candles, 14)
	n := len(candles)
	// Dump the last 8 closes by ~2 ATR each step's worth total against the uptrend.
	peak := candles[n-9].Close()
	for i := n - 8; i < n; i++ {
		frac := float64(i-(n-8)+1) / 8
		v := peak - 2*atr14*frac
		c := candles[i]
		candles[i] = domain.NewCandleUnsafe(
			c.Symbol(), c.Timeframe(), c.Timestamp(),
			v, v+0.05, v-0.05, v, c.Volume(),
		)
	}
	pulled, err := domain.NewCandleSeries(series.Symbol(), series.Timeframe(), candles)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := testutil.BuildTrendSeries(testutil.TrendSeriesSpec{
		Bars: 110, NetReturn: 0.12, Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseTape := appmarket.ScoreMarketTape(baseline, "4h", "composite_median")
	crashTape := appmarket.ScoreMarketTape(pulled, "4h", "composite_median")
	if crashTape.State != mkt.StateTrend {
		t.Fatalf("state=%s after crash tail", crashTape.State)
	}
	if crashTape.EffectiveTrend >= baseTape.EffectiveTrend*0.5 {
		t.Fatalf("crash health=%.3f not penalised vs baseline %.3f", crashTape.EffectiveTrend, baseTape.EffectiveTrend)
	}
}
