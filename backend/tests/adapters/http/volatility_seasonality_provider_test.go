package http_test

import (
	"context"
	"errors"
	"testing"
	"time"

	httpAdapter "pano_chart/backend/adapters/http"
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

func TestVolatilitySeasonalityProvider_NoBucketForCurrentMinute_ReturnsError(t *testing.T) {
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: (fixedMinuteOfDay + 1) % 1440, SpikeProb: 0.5}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProviderWithClock(source, fixedNow)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected an error when no bucket matches the current minute")
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
