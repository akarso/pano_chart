package setups

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestMeanReversionFromSeries_UsesSidewaysTrailingWindow(t *testing.T) {
	window := sidewaysCandleCount()
	if window != 110 {
		t.Fatalf("CandleCount=%d want 110 default", window)
	}
	// 200 bars: early geometric drift, then oscillating drives. Trailing
	// CandleCount MRS must match MeanReversionScore on that slice and
	// differ from the full-series MRS.
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Unix(1_700_000_000, 0).UTC()
	candles := make([]domain.Candle, 200)
	price := 100.0
	for i := 0; i < 90; i++ {
		price *= 1.01
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), price, price*1.001, price*0.999, price, 1)
	}
	var r float64
	for i := 90; i < 200; i++ {
		drive := 0.01
		if i%2 == 0 {
			drive = -0.01
		}
		r = -0.5*r + drive
		price *= math.Exp(r)
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), price, price*1.001, price*0.999, price, 1)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}

	got := meanReversionFromSeries(series)

	tailCloses := make([]float64, window)
	for i := 0; i < window; i++ {
		c, err := series.At(series.Len() - window + i)
		if err != nil {
			t.Fatal(err)
		}
		tailCloses[i] = c.Close()
	}
	want := scoring.MeanReversionScore(tailCloses)
	if got != want {
		t.Fatalf("meanReversionFromSeries=%g want MeanReversionScore(last %d)=%g", got, window, want)
	}

	allCloses := make([]float64, series.Len())
	for i := 0; i < series.Len(); i++ {
		c, _ := series.At(i)
		allCloses[i] = c.Close()
	}
	full := scoring.MeanReversionScore(allCloses)
	if full == want {
		t.Fatalf("sanity: full-series MRS=%g equals trailing; fixture may not discriminate", full)
	}
}
