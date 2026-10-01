package scoring

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestCompressionPercentile_NameAndWindowHint(t *testing.T) {
	calc := &scoring.CompressionPercentileScoreCalculator{}
	if calc.Name() != "Compression Pct" {
		t.Fatalf("Name=%q", calc.Name())
	}
	if calc.WindowHint() != 500 {
		t.Fatalf("WindowHint=%d want 500", calc.WindowHint())
	}
}

func TestMaxWindowHint(t *testing.T) {
	abs := &scoring.CompressionScoreCalculator{Config: scoring.DefaultCompressionConfig()}
	pct := &scoring.CompressionPercentileScoreCalculator{}
	if got := scoring.MaxWindowHint(110, abs); got != 110 {
		t.Fatalf("absolute hint=%d want 110", got)
	}
	if got := scoring.MaxWindowHint(110, abs, pct); got != 500 {
		t.Fatalf("with percentile=%d want 500", got)
	}
}

func TestCompressionPercentile_ShortSeriesFallsBackToLegacy(t *testing.T) {
	// <200 bars → absolute detector path (legacy).
	data := convergingChannel(120)
	series := makeCompressionSeries(data)
	legacy := scoring.DefaultCompressionConfig()
	legacy.CandleCount = 100
	want, err := (&scoring.CompressionScoreCalculator{Config: legacy}).Score(series)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&scoring.CompressionPercentileScoreCalculator{Legacy: legacy}).Score(series)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("short fallback=%g want legacy %g", got, want)
	}
}

func TestCompressionPercentile_TightTailHighScore(t *testing.T) {
	// 500 bars: early wide ranges, last 20 half the range → score ≥ 0.7.
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Unix(1_700_000_000, 0).UTC()
	candles := make([]domain.Candle, 500)
	price := 100.0
	for i := 0; i < 500; i++ {
		half := 2.0
		if i >= 480 {
			half = 1.0 // half the range of the rest
		}
		o := price
		h := price + half
		l := price - half
		c := price + half*0.1
		if i%2 == 0 {
			c = price - half*0.1
		}
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), o, h, l, c, 1000)
		price = c
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&scoring.CompressionPercentileScoreCalculator{}).Score(series)
	if err != nil {
		t.Fatal(err)
	}
	if got < 0.7 {
		t.Fatalf("tight-tail score=%g want ≥ 0.7", got)
	}
	if got > 1 {
		t.Fatalf("score=%g > 1", got)
	}
}

func TestCompressionPercentile_UniformNoiseNearHalf(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Unix(1_700_000_000, 0).UTC()
	candles := make([]domain.Candle, 500)
	for i := 0; i < 500; i++ {
		// Stationary iid ranges around a fixed mid — last bar is a typical draw
		// so percentile ranks sit near 0.5 and squeeze stays short.
		mid := 100.0
		half := 0.8 + rng.Float64()*1.2
		o := mid + (rng.Float64()-0.5)*0.2
		c := mid + (rng.Float64()-0.5)*0.2
		h := math.Max(o, c) + half
		l := math.Min(o, c) - half
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), o, h, l, c, 1000)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&scoring.CompressionPercentileScoreCalculator{}).Score(series)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-0.5) > 0.15 {
		t.Fatalf("uniform noise score=%g want ≈0.5±0.15", got)
	}
}
