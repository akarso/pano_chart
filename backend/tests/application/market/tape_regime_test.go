package market_test

import (
	"context"
	"math"
	"testing"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

type volCandle struct {
	symbol    domain.Symbol
	timeframe domain.Timeframe
	ts        time.Time
	close     float64
	volume    float64
}

type weightedCandleProvider struct {
	symbols []domain.Symbol
	candles map[string][]volCandle
}

func (f *weightedCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, nil
}

func (f *weightedCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	rows, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, errNoData(key)
	}
	out := make([]domain.Candle, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.NewCandleUnsafe(
			r.symbol, r.timeframe, r.ts, r.close, r.close, r.close, r.close, r.volume,
		))
	}
	if n < len(out) {
		out = out[len(out)-n:]
	}
	return domain.NewCandleSeries(sym, tf, out)
}

func errNoData(key string) error {
	return &simpleError{msg: "no data for " + key}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func TestCompositeIndex_VolumeWeightedPrefersHeavierSymbol(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	alt := makeSymbol2("ALTUSDT")
	tf := makeTimeframe2("4h")
	provider := &weightedCandleProvider{
		symbols: []domain.Symbol{btc, alt},
		candles: map[string][]volCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100, volume: 1_000_000},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 100, volume: 1_000_000},
			},
			"ALTUSDT:4h": {
				{symbol: alt, timeframe: tf, ts: ts4h(0), close: 10, volume: 1},
				{symbol: alt, timeframe: tf, ts: ts4h(1), close: 20, volume: 1},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Points) != 2 || len(idx.VolumeWeightedPoints) != 2 {
		t.Fatalf("points=%d vw=%d", len(idx.Points), len(idx.VolumeWeightedPoints))
	}
	if math.Abs(idx.Points[1].Value-100*math.Sqrt(2)) > 0.01 {
		t.Errorf("median expected ~%.2f (log), got %.2f", 100*math.Sqrt(2), idx.Points[1].Value)
	}
	if idx.VolumeWeightedPoints[1].Value > 105 {
		t.Errorf("volume-weighted should stay near 100, got %.2f", idx.VolumeWeightedPoints[1].Value)
	}
}

func TestScoreMarketTape_ShortSeriesRejected(t *testing.T) {
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	tf, _ := domain.NewTimeframe("15m")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	// 10 bars is enough to look directional but too short for Wilder ATR(14).
	candles := make([]domain.Candle, 10)
	for i := range candles {
		v := 100 + float64(i)
		ts := base.Add(time.Duration(i) * 15 * time.Minute)
		candles[i] = domain.NewCandleUnsafe(sym, tf, ts, v, v+0.05, v-0.05, v, 1000)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(series, "15m", "composite_median")
	if tape.TrendScore != 0 || tape.Confidence != 0 {
		t.Fatalf("short series must not score: %+v", tape)
	}
	if tape.State != mkt.StateSideways {
		t.Fatalf("state=%s", tape.State)
	}
}

func TestScoreMarketTape_RisingSeriesIsTrendBiased(t *testing.T) {
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	tf, _ := domain.NewTimeframe("15m")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 80)
	for i := range candles {
		// Smooth linear grind higher — strong TapeTrend.
		v := 100 + float64(i)*0.4
		ts := base.Add(time.Duration(i) * 15 * time.Minute)
		candles[i] = domain.NewCandleUnsafe(sym, tf, ts, v, v+0.05, v-0.05, v, 1000)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(series, "15m", "composite_median")
	if tape.State != mkt.StateTrend {
		t.Fatalf("expected TREND on rising tape, got %s structure=%+v score=%.3f", tape.State, tape.Structure, tape.TrendScore)
	}
	if tape.TrendScore < 0.5 {
		t.Fatalf("TREND requires TapeTrend≥0.5, got %.3f", tape.TrendScore)
	}
	if tape.Bias != "up" {
		t.Errorf("expected bias=up, got %s", tape.Bias)
	}
	if tape.Confidence != tape.TrendScore {
		t.Errorf("confidence must be raw TapeTrend, got %.3f vs %.3f", tape.Confidence, tape.TrendScore)
	}
}

func TestScoreMarketTape_FlatOscillationFavorsSideways(t *testing.T) {
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	tf, _ := domain.NewTimeframe("15m")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 80)
	for i := range candles {
		// Oscillate in a tight band.
		phase := float64(i % 8)
		var v float64
		if phase < 4 {
			v = 100 + phase*0.15
		} else {
			v = 100.6 - (phase-4)*0.15
		}
		ts := base.Add(time.Duration(i) * 15 * time.Minute)
		candles[i] = domain.NewCandleUnsafe(sym, tf, ts, v, v+0.05, v-0.05, v, 500)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	tape := appmarket.ScoreMarketTape(series, "15m", "composite_median")
	if tape.Structure.Trend > tape.Structure.Sideways && tape.Structure.Trend > tape.Structure.Compression {
		t.Fatalf("flat oscillation should not be trend-led, got %+v state=%s", tape.Structure, tape.State)
	}
}
