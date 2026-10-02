package http_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	httpAdapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/application/setups"
	vol "pano_chart/backend/infrastructure/volatility"
)

// fixedClock is a fixed, deterministic instant used across these tests —
// CR follow-up: avoids racing time.Now() against a real minute boundary,
// which the previous version of this test file did by computing "now"
// separately (once in the test, once inside the provider).
// 2026-01-05 is a Monday (time.Monday == 1), 03:04 UTC.
var fixedClock = time.Date(2026, 1, 5, 3, 4, 0, 0, time.UTC)

func fixedNow() time.Time { return fixedClock }

const (
	fixedMinuteOfDay  = 3*60 + 4          // 184
	fixedMinuteOfWeek = 1*1440 + 3*60 + 4 // Monday(1)*1440 + 184
)

type fakeVolatilityResultSource struct {
	result *vol.FullResult
	err    error
}

func (f *fakeVolatilityResultSource) CurrentResult() (*vol.FullResult, error) {
	return f.result, f.err
}

func TestVolatilitySeasonalityProvider_ImplementsSeasonalityProvider(t *testing.T) {
	// compile-time check
	var _ setups.SeasonalityProvider = httpAdapter.NewVolatilitySeasonalityProvider(&fakeVolatilityResultSource{})
}

func TestVolatilitySeasonalityProvider_ReturnsSpikeProbForCurrentMinute(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{
				Timeframe: vol.TF1m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: (fixedMinuteOfDay + 100) % 1440, SpikeProb: 0.99}, // a different minute — must not match
					{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.42},
				},
			},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.42 {
		t.Errorf("expected 0.42, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_FastPathDenseArray(t *testing.T) {
	// A dense, index-aligned array (buckets[i].MinuteOfDay == i for all i)
	// must resolve via the O(1) direct-index fast path, not the fallback
	// scan — this fixture is large enough that an accidental full scan
	// would still pass functionally, so this mainly documents/pins the
	// intended fast path rather than proving its complexity, but the
	// MinuteOfDay-mismatch fixture below (sparse/gapped) is what actually
	// forces the fallback to be exercised and correct.
	buckets := make([]vol.BucketResult, 1440)
	for i := range buckets {
		buckets[i] = vol.BucketResult{MinuteOfDay: i, SpikeProb: float64(i) / 1440.0}
	}
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{Timeframe: vol.TF1m, Buckets: buckets}},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := float64(fixedMinuteOfDay) / 1440.0
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestVolatilitySeasonalityProvider_FallbackScanOnGappedArray(t *testing.T) {
	// A sparse/gapped array (some early minutes missing, so index !=
	// MinuteOfDay past the gap) must still find the right bucket via the
	// scan fallback, not silently return the wrong (or no) value.
	buckets := []vol.BucketResult{
		{MinuteOfDay: 0, SpikeProb: 0.1},
		// gap: minutes 1..(fixedMinuteOfDay-1) missing, so
		// buckets[fixedMinuteOfDay] (if it existed) would NOT be the
		// fixedMinuteOfDay entry — forces the fallback path.
		{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.77},
		{MinuteOfDay: fixedMinuteOfDay + 1, SpikeProb: 0.2},
	}
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{Timeframe: vol.TF1m, Buckets: buckets}},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.77 {
		t.Errorf("expected 0.77 (found via fallback scan), got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_IgnoresTimeframeParam_AlwaysUses1m(t *testing.T) {
	// Regression test for the documented simplification: the provider
	// always answers from the 1m buckets regardless of the requested
	// timeframe, since coarser derived timeframes group buckets by array
	// position rather than an explicit time range.
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.5}}},
			{Timeframe: vol.TF5m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.99}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	for _, tf := range []string{"", "1m", "5m", "15m", "1h", "4h", "1d", "garbage"} {
		got, err := p.CurrentSpikeProbability(context.Background(), tf)
		if err != nil {
			t.Fatalf("timeframe %q: unexpected error: %v", tf, err)
		}
		if got != 0.5 {
			t.Errorf("timeframe %q: expected the 1m bucket's 0.5, got %v", tf, got)
		}
	}
}

// TestVolatilitySeasonalityProvider_NearbyMinuteWithinRadius_UsedInsteadOfFailing
// is the CR follow-up regression test: a sparse-but-otherwise-healthy
// profile missing a bucket for the exact current minute (a thin market, a
// brief data hole during aggregation — hasUsable1mBuckets only checks that
// *some* 1m buckets exist, not that this specific minute is one of them)
// must not fail outright and silently degrade SeasonalityFit to neutral.
// A nearby minute within maxMinuteSearchRadius is used instead.
func TestVolatilitySeasonalityProvider_NearbyMinuteWithinRadius_UsedInsteadOfFailing(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: (fixedMinuteOfDay + 1) % 1440, SpikeProb: 0.5}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("expected the nearby minute (1 away) to be used instead of failing, got error: %v", err)
	}
	if got != 0.5 {
		t.Errorf("expected the nearby bucket's 0.5, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_NoBucketWithinRadius_ReturnsError(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			// well outside maxMinuteSearchRadius (15) in both directions
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: (fixedMinuteOfDay + 100) % 1440, SpikeProb: 0.5}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected an error when no bucket exists within the search radius")
	}
}

func TestVolatilitySeasonalityProvider_NearbyMinute_WrapsAroundDayBoundary(t *testing.T) {
	// fixedMinuteOfDay is 184 (03:04 UTC) — far from midnight, so use an
	// explicit near-midnight case instead of the shared fixture: minute 5,
	// with data only at minute 1439 (== -1, i.e. 6 minutes away across the
	// day boundary) must still resolve via the cyclic search.
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: 1439, SpikeProb: 0.6}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, func() time.Time {
		return time.Date(2026, 1, 5, 0, 5, 0, 0, time.UTC) // minute-of-day 5
	})

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("expected the wrap-around neighbor to be used, got error: %v", err)
	}
	if got != 0.6 {
		t.Errorf("expected the wrapped-around bucket's 0.6, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_No1mTimeframe_ReturnsError(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF5m, Buckets: []vol.BucketResult{{MinuteOfDay: 0, SpikeProb: 0.5}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected an error when no 1m timeframe entry exists")
	}
}

func TestVolatilitySeasonalityProvider_SourceError_Propagates(t *testing.T) {
	source := &fakeVolatilityResultSource{err: errors.New("data not loaded")}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected the source error to propagate")
	}
}

// --- Weekly (day-of-week) seasonality fold-in (CR follow-up) ---

func TestVolatilitySeasonalityProvider_WeeklySpikeHigherThanDaily_UsesWeekly(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.2}}},
		},
		Weekly: vol.WeeklyResult{
			Buckets: []vol.WeeklyBucket{{MinuteOfWeek: fixedMinuteOfWeek, SpikeProb: 0.8}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.8 {
		t.Errorf("expected the higher weekly spike probability 0.8 to win, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_DailySpikeHigherThanWeekly_UsesDaily(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.9}}},
		},
		Weekly: vol.WeeklyResult{
			Buckets: []vol.WeeklyBucket{{MinuteOfWeek: fixedMinuteOfWeek, SpikeProb: 0.1}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.9 {
		t.Errorf("expected the higher daily spike probability 0.9 to win, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_NoWeeklyData_FallsBackToDailyOnly(t *testing.T) {
	// Weekly.Buckets can legitimately be empty (e.g. not enough history
	// yet) — must not fail the call, just use the intraday-only reading.
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.3}}},
		},
		// Weekly deliberately left zero-value (no buckets).
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.3 {
		t.Errorf("expected the daily-only reading 0.3, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_NoMatchingWeeklyBucket_FallsBackToDailyOnly(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.3}}},
		},
		Weekly: vol.WeeklyResult{
			// present, but for a different minute-of-week entirely
			Buckets: []vol.WeeklyBucket{{MinuteOfWeek: (fixedMinuteOfWeek + 1000) % 10080, SpikeProb: 0.9}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.3 {
		t.Errorf("expected the daily-only reading 0.3 when no weekly bucket matches, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_MissingSectorFile_FallsBackToMarket(t *testing.T) {
	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.21}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(t.TempDir() + "/vol") // no files written

	got, err := p.CurrentSpikeProbabilityFor(context.Background(), "defi", "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.21 {
		t.Errorf("missing sector file must equal market-wide value, got %v want 0.21", got)
	}

	// Second call must not re-probe (negative cache); still market.
	got2, err := p.CurrentSpikeProbabilityFor(context.Background(), "defi", "15m")
	if err != nil || got2 != 0.21 {
		t.Fatalf("neg-cached miss: got=%v err=%v", got2, err)
	}
}

func TestVolatilitySeasonalityProvider_SectorFileOverridesMarket(t *testing.T) {
	dir := t.TempDir()
	prefix := dir + "/vol"
	sectorPath, err := metrics.SectorProfilePath(prefix, "defi")
	if err != nil {
		t.Fatal(err)
	}
	sector := vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.77}}},
		},
	}
	if err := vol.SaveFullResult(sector, sectorPath); err != nil {
		t.Fatal(err)
	}

	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.21}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(prefix)

	got, err := p.CurrentSpikeProbabilityFor(context.Background(), "DeFi", "15m") // mixed case
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.77 {
		t.Errorf("sector file should win, got %v want 0.77", got)
	}

	fb, err := p.CurrentSpikeProbabilityFor(context.Background(), "meme", "15m")
	if err != nil {
		t.Fatal(err)
	}
	if fb != 0.21 {
		t.Errorf("fallback=%v want 0.21", fb)
	}
}

func TestVolatilitySeasonalityProvider_CorruptSectorJSON_FallsBackAndLogs(t *testing.T) {
	dir := t.TempDir()
	prefix := dir + "/vol"
	path, err := metrics.SectorProfilePath(prefix, "l1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var logs []string
	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.11}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(prefix)
	p.SetLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	got, err := p.CurrentSpikeProbabilityFor(context.Background(), "l1", "15m")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.11 {
		t.Fatalf("corrupt sector must fall back to market, got %v", got)
	}
	if len(logs) == 0 {
		t.Fatal("expected warn log for corrupt sector profile")
	}
}

func TestVolatilitySeasonalityProvider_PathContractWithOutStem(t *testing.T) {
	// Round-trip: write as vol_aggregate --out does, read as API does.
	dir := t.TempDir()
	outStem := dir + "/vol"
	path, err := metrics.SectorProfilePath(outStem, "ai")
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.SaveFullResult(vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.55}},
		}},
	}, path); err != nil {
		t.Fatal(err)
	}

	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.01}},
		}},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(outStem) // VOL_SECTOR_PREFIX == --out

	got, err := p.CurrentSpikeProbabilityFor(context.Background(), "ai", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.55 {
		t.Fatalf("path contract broken: got %v want 0.55 (path=%s)", got, path)
	}
}

type countingSectorHandler struct {
	result    *vol.FullResult
	reloadErr error
	opens     *int
	reloads   *int
}

func (c *countingSectorHandler) CurrentResult() (*vol.FullResult, error) {
	return c.result, nil
}

func (c *countingSectorHandler) Reload() error {
	if c.reloads != nil {
		*c.reloads++
	}
	return c.reloadErr
}

func TestVolatilitySeasonalityProvider_ReloadSectors_ReloadFailureDoesNotHang(t *testing.T) {
	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.2}},
		}},
	}}
	opens, reloads := 0, 0
	var logs []string
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(t.TempDir() + "/vol")
	p.SetLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	p.SetSectorHandlerFactory(func(path string) httpAdapter.SectorProfileHandler {
		opens++
		return &countingSectorHandler{
			result: &vol.FullResult{
				Intraday: []vol.TimeframeResult{{
					Timeframe: vol.TF1m,
					Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.9}},
				}},
			},
			reloadErr: errors.New("disk mid-write"),
			opens:     &opens,
			reloads:   &reloads,
		}
	})

	if _, err := p.CurrentSpikeProbabilityFor(context.Background(), "l1", "15m"); err != nil {
		t.Fatal(err)
	}
	if opens != 1 {
		t.Fatalf("opens=%d want 1", opens)
	}

	done := make(chan struct{})
	go func() {
		p.ReloadSectors()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ReloadSectors hung (likely mutex re-lock on warnf)")
	}
	if reloads != 1 {
		t.Fatalf("reloads=%d want 1", reloads)
	}
	if len(logs) == 0 {
		t.Fatal("expected reload-failure log")
	}
}

func TestVolatilitySeasonalityProvider_SparseMinuteKeepsCachedHandler(t *testing.T) {
	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.33}},
		}},
	}}
	opens := 0
	// Sector profile has only minute 0 — fixedClock minute misses → market fallback.
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(t.TempDir() + "/vol")
	p.SetLogger(func(string, ...any) {}) // silence sparse-minute warnings
	p.SetSectorHandlerFactory(func(path string) httpAdapter.SectorProfileHandler {
		opens++
		return &countingSectorHandler{
			result: &vol.FullResult{
				Intraday: []vol.TimeframeResult{{
					Timeframe: vol.TF1m,
					Buckets:   []vol.BucketResult{{MinuteOfDay: 0, SpikeProb: 0.99}},
				}},
			},
			opens: &opens,
		}
	})

	got, err := p.CurrentSpikeProbabilityFor(context.Background(), "l1", "15m")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.33 {
		t.Fatalf("sparse sector must fall back to market, got %v", got)
	}
	if opens != 1 {
		t.Fatalf("first opens=%d want 1", opens)
	}

	got2, err := p.CurrentSpikeProbabilityFor(context.Background(), "l1", "15m")
	if err != nil {
		t.Fatal(err)
	}
	if got2 != 0.33 {
		t.Fatalf("second call got %v", got2)
	}
	if opens != 1 {
		t.Fatalf("sparse-minute fallback must not drop/reopen handler, opens=%d", opens)
	}
}

func TestVolatilitySeasonalityProvider_MissingFileNegCacheSkipsSecondOpen(t *testing.T) {
	market := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: fixedMinuteOfDay, SpikeProb: 0.12}},
		}},
	}}
	opens := 0
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(market, fixedNow)
	p.SetSectorPrefix(t.TempDir() + "/vol")
	p.SetLogger(func(string, ...any) {})
	p.SetSectorHandlerFactory(func(path string) httpAdapter.SectorProfileHandler {
		opens++
		return &failingSectorHandler{err: errors.New("no such file")}
	})

	_, _ = p.CurrentSpikeProbabilityFor(context.Background(), "defi", "15m")
	_, _ = p.CurrentSpikeProbabilityFor(context.Background(), "defi", "15m")
	if opens != 1 {
		t.Fatalf("neg-cache must open once, opens=%d", opens)
	}
}

type failingSectorHandler struct{ err error }

func (f *failingSectorHandler) CurrentResult() (*vol.FullResult, error) { return nil, f.err }
func (f *failingSectorHandler) Reload() error                           { return f.err }
