package http

import (
	"context"
	"fmt"
	"time"

	vol "pano_chart/backend/infrastructure/volatility"
)

// volatilityResultSource is the minimal surface VolatilitySeasonalityProvider
// needs from VolatilityHandler — kept as a small interface (rather than a
// concrete *VolatilityHandler field) so tests can fake it without spinning
// up a real handler/JSON file.
type volatilityResultSource interface {
	CurrentResult() (*vol.FullResult, error)
}

// VolatilitySeasonalityProvider implements setups.SeasonalityProvider by
// looking up the current moment's historical spike probability from the
// same precomputed volatility profile VolatilityHandler serves — see
// PR-082. The reported probability is the max of two independent seasonal
// reads: the current UTC minute-of-day (intraday) and the current
// minute-of-week (day-of-week — e.g. weekend lull, Monday-open effects).
// Either being elevated is reason for caution on its own, so max avoids a
// calm-looking minute-of-day diluting a genuinely risky day-of-week (or
// vice versa) the way an average would — CR follow-up.
//
// The intraday half always answers from the 1-minute-of-day buckets,
// regardless of the timeframe argument: infrastructure/volatility's coarser
// derived timeframes (5m/15m/1h/4h — see DeriveTimeframe) group 1-minute
// buckets by array position, not by an explicit time range each resulting
// bucket covers, so matching "the bucket containing right now" against one
// of them would require reverse-engineering that grouping. The 1-minute
// data has no such ambiguity (each entry's MinuteOfDay means exactly what
// it says) and is strictly finer-grained than any chart timeframe a caller
// might ask for, so this is a simplification, not a loss of information.
//
// Also — per PR-082's own scoping note — cmd/vol_aggregate currently
// computes this profile for a single reference symbol (BTCUSDT) and applies
// it market-wide; this provider inherits that limitation as-is.
type VolatilitySeasonalityProvider struct {
	source volatilityResultSource
	now    func() time.Time // injectable for tests; defaults to time.Now
}

// NewVolatilitySeasonalityProvider constructs the provider over an existing
// *VolatilityHandler (or any type satisfying volatilityResultSource).
func NewVolatilitySeasonalityProvider(source volatilityResultSource) *VolatilitySeasonalityProvider {
	return &VolatilitySeasonalityProvider{source: source, now: time.Now}
}

// NewVolatilitySeasonalityProviderWithClock is NewVolatilitySeasonalityProvider
// with an injectable clock, so tests can pin "now" instead of racing
// time.Now() against a minute boundary — CR follow-up.
func NewVolatilitySeasonalityProviderWithClock(source volatilityResultSource, now func() time.Time) *VolatilitySeasonalityProvider {
	return &VolatilitySeasonalityProvider{source: source, now: now}
}

// CurrentSpikeProbability implements setups.SeasonalityProvider.
func (p *VolatilitySeasonalityProvider) CurrentSpikeProbability(_ context.Context, _ string) (float64, error) {
	result, err := p.source.CurrentResult()
	if err != nil {
		return 0, fmt.Errorf("volatility seasonality: %w", err)
	}

	var intraday []vol.BucketResult
	for _, entry := range result.Intraday {
		if entry.Timeframe == vol.TF1m {
			intraday = entry.Buckets
			break
		}
	}
	if len(intraday) == 0 {
		return 0, fmt.Errorf("volatility seasonality: no 1m buckets available")
	}

	nowFn := p.now
	if nowFn == nil {
		nowFn = time.Now
	}
	t := nowFn().UTC()
	minuteOfDay := t.Hour()*60 + t.Minute()

	spikeProb, ok := lookupBucketSpikeProb(intraday, minuteOfDay)
	if !ok {
		return 0, fmt.Errorf("volatility seasonality: no bucket for minute-of-day %d", minuteOfDay)
	}

	// Day-of-week convention matches BuildWeekly/DeriveDailyOfWeek
	// (infrastructure/volatility/weekly.go): dow*1440+minute, dow from
	// time.Weekday() (0=Sunday). Weekly data is optional — Weekly.Buckets
	// can be empty (e.g. not enough history yet) without failing the call;
	// the intraday read alone is still a valid, if narrower, signal.
	minuteOfWeek := int(t.Weekday())*1440 + minuteOfDay
	if weeklySpikeProb, ok := lookupWeeklyBucketSpikeProb(result.Weekly.Buckets, minuteOfWeek); ok && weeklySpikeProb > spikeProb {
		spikeProb = weeklySpikeProb
	}

	return spikeProb, nil
}

// lookupBucketSpikeProb finds the intraday bucket for minuteOfDay.
// Aggregate builds a full 1440-slot array pre-indexed by MinuteOfDay before
// filtering out empty minutes, so a dense/gapless result (the common case)
// resolves via direct index in O(1); the MinuteOfDay equality check guards
// against a data gap having shifted that alignment, falling back to a full
// scan only then.
func lookupBucketSpikeProb(buckets []vol.BucketResult, minuteOfDay int) (float64, bool) {
	if minuteOfDay >= 0 && minuteOfDay < len(buckets) && buckets[minuteOfDay].MinuteOfDay == minuteOfDay {
		return buckets[minuteOfDay].SpikeProb, true
	}
	for _, b := range buckets {
		if b.MinuteOfDay == minuteOfDay {
			return b.SpikeProb, true
		}
	}
	return 0, false
}

// lookupWeeklyBucketSpikeProb is lookupBucketSpikeProb's counterpart for
// the (up to 10 080-slot) weekly buckets — same fast-path-then-scan
// rationale.
func lookupWeeklyBucketSpikeProb(buckets []vol.WeeklyBucket, minuteOfWeek int) (float64, bool) {
	if minuteOfWeek >= 0 && minuteOfWeek < len(buckets) && buckets[minuteOfWeek].MinuteOfWeek == minuteOfWeek {
		return buckets[minuteOfWeek].SpikeProb, true
	}
	for _, b := range buckets {
		if b.MinuteOfWeek == minuteOfWeek {
			return b.SpikeProb, true
		}
	}
	return 0, false
}
