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
// looking up the current UTC minute-of-day's historical spike probability
// from the same precomputed volatility profile VolatilityHandler serves —
// see PR-082.
//
// It always answers from the 1-minute-of-day buckets, regardless of the
// timeframe argument: infrastructure/volatility's coarser derived
// timeframes (5m/15m/1h/4h — see DeriveTimeframe) group 1-minute buckets by
// array position, not by an explicit time range each resulting bucket
// covers, so matching "the bucket containing right now" against one of
// them would require reverse-engineering that grouping. The 1-minute data
// has no such ambiguity (each entry's MinuteOfDay means exactly what it
// says) and is strictly finer-grained than any chart timeframe a caller
// might ask for, so this is a simplification, not a loss of information.
//
// Also — per PR-082's own scoping note — cmd/vol_aggregate currently
// computes this profile for a single reference symbol (BTCUSDT) and applies
// it market-wide; this provider inherits that limitation as-is.
type VolatilitySeasonalityProvider struct {
	source volatilityResultSource
}

// NewVolatilitySeasonalityProvider constructs the provider over an existing
// *VolatilityHandler (or any type satisfying volatilityResultSource).
func NewVolatilitySeasonalityProvider(source volatilityResultSource) *VolatilitySeasonalityProvider {
	return &VolatilitySeasonalityProvider{source: source}
}

// CurrentSpikeProbability implements setups.SeasonalityProvider.
func (p *VolatilitySeasonalityProvider) CurrentSpikeProbability(_ context.Context, _ string) (float64, error) {
	result, err := p.source.CurrentResult()
	if err != nil {
		return 0, fmt.Errorf("volatility seasonality: %w", err)
	}

	var buckets []vol.BucketResult
	for _, entry := range result.Intraday {
		if entry.Timeframe == vol.TF1m {
			buckets = entry.Buckets
			break
		}
	}
	if len(buckets) == 0 {
		return 0, fmt.Errorf("volatility seasonality: no 1m buckets available")
	}

	t := time.Now().UTC()
	minuteOfDay := t.Hour()*60 + t.Minute()

	for _, b := range buckets {
		if b.MinuteOfDay == minuteOfDay {
			return b.SpikeProb, nil
		}
	}
	return 0, fmt.Errorf("volatility seasonality: no bucket for minute-of-day %d", minuteOfDay)
}
