package usecases

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

func seriesFromReturns(sym domain.Symbol, tf domain.Timeframe, base time.Time, start float64, rets []float64) (domain.CandleSeries, []float64) {
	n := len(rets) + 1
	bars := make([]domain.Candle, n)
	closes := make([]float64, n)
	closes[0] = start
	bars[0] = mustNewCandleAt(sym, tf, base, start)
	px := start
	for i, r := range rets {
		px *= math.Exp(r)
		closes[i+1] = px
		bars[i+1] = mustNewCandleAt(sym, tf, base.Add(time.Duration(i+1)*time.Hour), px)
	}
	cs, _ := domain.NewCandleSeries(sym, tf, bars)
	return cs, closes
}

func TestGetRankings_RelativeStrengthViaTapeProvider(t *testing.T) {
	a := domain.NewSymbolUnsafe("AAAUSDT")
	b := domain.NewSymbolUnsafe("BBBUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mktR := make([]float64, 24)
	for i := range mktR {
		mktR[i] = 0.01 * float64((i%5)-2)
		if mktR[i] == 0 {
			mktR[i] = 0.005
		}
	}
	aCS, _ := seriesFromReturns(a, tf, base, 100, mktR)
	bRets := make([]float64, len(mktR))
	for i, r := range mktR {
		bRets[i] = 2 * r
	}
	bCS, bCloses := seriesFromReturns(b, tf, base, 100, bRets)
	tapeCloses := make([]float64, len(mktR)+1)
	tapeCloses[0] = 100
	for i, r := range mktR {
		tapeCloses[i+1] = tapeCloses[i] * math.Exp(r)
	}

	calc := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"AAAUSDT": 0.5, "BBBUSDT": 0.5}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{a, b}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"AAAUSDT": 1, "BBBUSDT": 1}},
		NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{a: aCS, b: bCS}, nil),
		"", "", len(tapeCloses), usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	tape := &stubTapeProvider{closes: tapeCloses, base: base}
	uc.SetTapeProvider(tape)

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByLeaders,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.RSAvailable || out.Sort != usecases.SortByLeaders {
		t.Fatalf("available=%v sort=%s", out.RSAvailable, out.Sort)
	}
	if out.RequestedSort != usecases.SortByLeaders {
		t.Fatalf("requestedSort=%s", out.RequestedSort)
	}
	if tape.limit != metrics.CompositeTapeWindow {
		t.Fatalf("CalculateTape limit=%d want CompositeTapeWindow %d", tape.limit, metrics.CompositeTapeWindow)
	}
	if out.Results[0].Symbol.String() != "BBBUSDT" {
		t.Fatalf("leaders[0]=%s", out.Results[0].Symbol)
	}
	bySym := map[string]usecases.RankedResult{}
	for _, r := range out.Results {
		bySym[r.Symbol.String()] = r
	}
	aaa, bbb := bySym["AAAUSDT"], bySym["BBBUSDT"]
	if aaa.RelativeStrength == nil || math.Abs(*aaa.RelativeStrength) > 1e-9 {
		t.Fatalf("AAA RS=%v", aaa.RelativeStrength)
	}
	if aaa.Beta == nil || math.Abs(*aaa.Beta-1) > 1e-6 {
		t.Fatalf("AAA Beta=%v", aaa.Beta)
	}
	if bbb.Beta == nil || math.Abs(*bbb.Beta-2) > 1e-6 {
		t.Fatalf("BBB Beta=%v", bbb.Beta)
	}
	wantRS := math.Log(bCloses[len(bCloses)-1]/bCloses[0]) - math.Log(tapeCloses[len(tapeCloses)-1]/tapeCloses[0])
	if bbb.RelativeStrength == nil || math.Abs(*bbb.RelativeStrength-wantRS) > 1e-9 {
		t.Fatalf("BBB RS=%v want %v", bbb.RelativeStrength, wantRS)
	}
}

func TestGetRankings_ZeroOverlapFallsBack(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Spark at t0..t24; tape only at far-future stamps → zero overlap.
	rets := make([]float64, 24)
	for i := range rets {
		rets[i] = 0.01
	}
	cs, _ := seriesFromReturns(sym, tf, base, 100, rets)
	tapeCloses := make([]float64, 25)
	tapeCloses[0] = 100
	for i := 1; i < 25; i++ {
		tapeCloses[i] = tapeCloses[i-1] * 1.01
	}
	tapeBase := base.Add(1000 * time.Hour) // no shared Unix stamps
	calc := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"BTCUSDT": 0.5}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{sym}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"BTCUSDT": 1}},
		NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{sym: cs}, nil),
		"", "", 25, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	uc.SetTapeProvider(&stubTapeProvider{closes: tapeCloses, base: tapeBase})

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByLeaders,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RSAvailable {
		t.Fatal("zero overlap must set rsAvailable=false")
	}
	if out.Sort != usecases.SortByTotal || out.RequestedSort != usecases.SortByLeaders {
		t.Fatalf("sort=%s requested=%s", out.Sort, out.RequestedSort)
	}
	if out.Results[0].RelativeStrength != nil {
		t.Fatal("row must stay unset")
	}
}

func TestGetRankings_TapeErrorFallsBackFromLeaders(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	alt := domain.NewSymbolUnsafe("ETHUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(s domain.Symbol, start float64) domain.CandleSeries {
		cs, _ := domain.NewCandleSeries(s, tf, []domain.Candle{
			mustNewCandleAt(s, tf, base, start),
			mustNewCandleAt(s, tf, base.Add(time.Hour), start+10),
		})
		return cs
	}
	calc := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"BTCUSDT": 0.9, "ETHUSDT": 0.1}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{sym, alt}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"BTCUSDT": 1, "ETHUSDT": 1}},
		NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
			sym: mk(sym, 100), alt: mk(alt, 200),
		}, nil),
		"", "", 2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	uc.SetTapeProvider(&stubTapeProvider{err: errors.New("tape down")})

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByLeaders,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RSAvailable || out.Sort != usecases.SortByTotal {
		t.Fatalf("available=%v sort=%s", out.RSAvailable, out.Sort)
	}
	if out.Results[0].Symbol.String() != "BTCUSDT" {
		t.Fatalf("fallback[0]=%s", out.Results[0].Symbol)
	}
}

func TestGetRankings_NilTapeLeavesRSUnavailable(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cs, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{
		mustNewCandleAt(sym, tf, base, 100),
		mustNewCandleAt(sym, tf, base.Add(time.Hour), 110),
	})
	calc := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"BTCUSDT": 0.5}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{sym}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"BTCUSDT": 1}},
		NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{sym: cs}, nil),
		"", "", 2, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByLaggards,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RSAvailable || out.Sort != usecases.SortByTotal {
		t.Fatalf("available=%v sort=%s", out.RSAvailable, out.Sort)
	}
}

func TestGetRankings_ExcludedStableNotLaggardOnUpTape(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	usd := domain.NewSymbolUnsafe("USDCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rets := make([]float64, 24)
	for i := range rets {
		rets[i] = 0.01
	}
	btcCS, _ := seriesFromReturns(btc, tf, base, 100, rets)
	flat := make([]float64, 24)
	usdCS, _ := seriesFromReturns(usd, tf, base, 100, flat)
	tapeCloses := make([]float64, 25)
	tapeCloses[0] = 100
	for i := 1; i < 25; i++ {
		tapeCloses[i] = tapeCloses[i-1] * math.Exp(0.01)
	}
	calc := &stubCalculator{name: "Gain/Loss", scores: map[string]float64{"BTCUSDT": 0.5, "USDCUSDT": 0.5}}
	weights := []usecases.ScoreWeight{{Calculator: calc, Weight: 1.0}}
	uc := usecases.NewGetRankings(
		&fakeUniverse{symbols: []domain.Symbol{btc, usd}},
		usecases.NewDefaultRankSymbols(weights),
		&fakeVolumes{vols: map[string]float64{"BTCUSDT": 1, "USDCUSDT": 1}},
		NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{btc: btcCS, usd: usdCS}, nil),
		"", "", 25, usecases.SidewaysAlgoV1, weights, 4, nil,
	)
	uc.SetTapeProvider(&stubTapeProvider{closes: tapeCloses, base: base})
	uc.SetRSFilter(skipMap{"USDCUSDT": {}})

	out, err := uc.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf, Sort: usecases.SortByLaggards,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Results[0].Symbol.String() == "USDCUSDT" {
		t.Fatal("excluded stable must not win laggards")
	}
	if out.Results[len(out.Results)-1].Symbol.String() != "USDCUSDT" {
		t.Fatalf("excluded should sort last")
	}
}

type skipMap map[string]struct{}

func (s skipMap) Skip(sym string) bool {
	_, ok := s[sym]
	return ok
}

type stubTapeProvider struct {
	closes []float64
	base   time.Time
	err    error
	limit  int
}

func (s *stubTapeProvider) CalculateTape(_ context.Context, _ string, limit int) (metrics.CompositeTape, error) {
	s.limit = limit
	if s.err != nil {
		return metrics.CompositeTape{}, s.err
	}
	base := s.base
	if base.IsZero() {
		base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	pts := make([]mkt.IndexPoint, len(s.closes))
	candles := make([]domain.Candle, len(s.closes))
	sym := domain.NewSymbolUnsafe("COMPOSITE")
	tf := domain.NewTimeframeUnsafe("1h")
	for i, c := range s.closes {
		ts := base.Add(time.Duration(i) * time.Hour)
		pts[i] = mkt.IndexPoint{Timestamp: ts.Unix(), Value: c}
		candles[i] = domain.NewCandleUnsafe(sym, tf, ts, c, c, c, c, 1)
	}
	series, _ := domain.NewCandleSeries(sym, tf, candles)
	return metrics.CompositeTape{
		Index: mkt.CompositeIndex{
			Points:               pts,
			VolumeWeightedPoints: pts,
		},
		MedianSeries:    series,
		WeightedSeries:  series,
		PreferredSource: "composite_volume_weighted",
	}, nil
}
