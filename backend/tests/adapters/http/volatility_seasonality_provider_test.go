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

// currentMinuteOfDay mirrors VolatilitySeasonalityProvider's own lookup key,
// so tests can build a fixture bucket the provider is guaranteed to match
// against the real clock, without needing to override time.Now internally
// (this package's tests are all external/black-box, matching every other
// adapters/http test file's convention).
func currentMinuteOfDay(t *testing.T) int {
	t.Helper()
	now := time.Now().UTC()
	return now.Hour()*60 + now.Minute()
}

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
	minute := currentMinuteOfDay(t)
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{
				Timeframe: vol.TF1m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: (minute + 100) % 1440, SpikeProb: 0.99}, // a different minute — must not match
					{MinuteOfDay: minute, SpikeProb: 0.42},
				},
			},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProvider(source)

	got, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.42 {
		t.Errorf("expected 0.42, got %v", got)
	}
}

func TestVolatilitySeasonalityProvider_IgnoresTimeframeParam_AlwaysUses1m(t *testing.T) {
	// Regression test for the documented simplification: the provider
	// always answers from the 1m buckets regardless of the requested
	// timeframe, since coarser derived timeframes group buckets by array
	// position rather than an explicit time range.
	minute := currentMinuteOfDay(t)
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: minute, SpikeProb: 0.5}}},
			{Timeframe: vol.TF5m, Buckets: []vol.BucketResult{{MinuteOfDay: minute, SpikeProb: 0.99}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProvider(source)

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
	minute := currentMinuteOfDay(t)
	source := &fakeVolatilityResultSource{result: &vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{{MinuteOfDay: (minute + 1) % 1440, SpikeProb: 0.5}}},
		},
	}}
	p := httpAdapter.NewVolatilitySeasonalityProvider(source)

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
	p := httpAdapter.NewVolatilitySeasonalityProvider(source)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected an error when no 1m timeframe entry exists")
	}
}

func TestVolatilitySeasonalityProvider_SourceError_Propagates(t *testing.T) {
	source := &fakeVolatilityResultSource{err: errors.New("data not loaded")}
	p := httpAdapter.NewVolatilitySeasonalityProvider(source)

	_, err := p.CurrentSpikeProbability(context.Background(), "15m")
	if err == nil {
		t.Fatal("expected the source error to propagate")
	}
}
