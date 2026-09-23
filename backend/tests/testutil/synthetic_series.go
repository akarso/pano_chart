package testutil

import (
	"math"
	"math/rand"
	"time"

	"pano_chart/backend/domain"
)

// TrendSeriesSpec builds a deterministic close path for tape coherence tests.
type TrendSeriesSpec struct {
	Bars         int
	Start        float64
	NetReturn    float64 // e.g. 0.12 = +12%
	TailPullback float64 // fraction of final peak, applied on the last 10% of bars
	Noise        float64 // absolute close noise amplitude
	Seed         int64
	Down         bool
	Timeframe    string
}

// BuildTrendSeries produces an OHLC series whose closes follow the spec.
// High/low wiggle around close so TrueATR is well-defined.
func BuildTrendSeries(spec TrendSeriesSpec) (domain.CandleSeries, error) {
	if spec.Bars < 2 {
		spec.Bars = 110
	}
	if spec.Start <= 0 {
		spec.Start = 100
	}
	if spec.Timeframe == "" {
		spec.Timeframe = "4h"
	}
	tf, err := domain.NewTimeframe(spec.Timeframe)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	rng := rand.New(rand.NewSource(spec.Seed))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	dur := tf.Duration()

	end := spec.Start * (1 + spec.NetReturn)
	if spec.Down {
		end = spec.Start * (1 - spec.NetReturn)
	}
	candles := make([]domain.Candle, spec.Bars)
	for i := 0; i < spec.Bars; i++ {
		t := float64(i) / float64(spec.Bars-1)
		v := spec.Start + (end-spec.Start)*t
		if spec.Noise > 0 {
			v += (rng.Float64()*2 - 1) * spec.Noise
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*dur),
			v, v+0.15, v-0.15, v, 1000,
		)
	}
	if spec.TailPullback > 0 && spec.Bars >= 10 {
		tailStart := spec.Bars - spec.Bars/10
		peak := candles[tailStart-1].Close()
		for i := tailStart; i < spec.Bars; i++ {
			frac := float64(i-tailStart+1) / float64(spec.Bars-tailStart)
			var v float64
			if spec.Down {
				v = peak * (1 + spec.TailPullback*frac) // bounce against downtrend
			} else {
				v = peak * (1 - spec.TailPullback*frac)
			}
			if spec.Noise > 0 {
				v += (rng.Float64()*2 - 1) * spec.Noise * 0.25
			}
			candles[i] = domain.NewCandleUnsafe(
				sym, tf, base.Add(time.Duration(i)*dur),
				v, v+0.15, v-0.15, v, 1000,
			)
		}
	}
	return domain.NewCandleSeries(sym, tf, candles)
}

// BuildTightRange builds a 100–104 oscillation with three clear extrema.
func BuildTightRange(bars int, seed int64) (domain.CandleSeries, error) {
	if bars <= 0 {
		bars = 110
	}
	tf, err := domain.NewTimeframe("4h")
	if err != nil {
		return domain.CandleSeries{}, err
	}
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	dur := tf.Duration()
	candles := make([]domain.Candle, bars)
	for i := 0; i < bars; i++ {
		// Three full swings between 100 and 104.
		phase := float64(i) / float64(bars) * 3 * 2 * math.Pi
		v := 102 + 2*math.Sin(phase)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*dur),
			v, v+0.1, v-0.1, v, 500,
		)
	}
	_ = seed
	return domain.NewCandleSeries(sym, tf, candles)
}

// BuildRandomWalk builds a unit-step random walk around start.
// Wick size matches the step so Mag is not ATR-inflated artificially.
func BuildRandomWalk(bars int, start float64, seed int64) (domain.CandleSeries, error) {
	if bars <= 0 {
		bars = 110
	}
	if start <= 0 {
		start = 100
	}
	tf, err := domain.NewTimeframe("4h")
	if err != nil {
		return domain.CandleSeries{}, err
	}
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	rng := rand.New(rand.NewSource(seed))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	dur := tf.Duration()
	candles := make([]domain.Candle, bars)
	v := start
	const step = 0.35
	for i := 0; i < bars; i++ {
		if i > 0 {
			if rng.Float64() < 0.5 {
				v += step
			} else {
				v -= step
			}
		}
		wick := step * 0.5
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*dur),
			v, v+wick, v-wick, v, 800,
		)
	}
	return domain.NewCandleSeries(sym, tf, candles)
}
