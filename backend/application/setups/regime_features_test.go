package setups

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestDominantRegimeLearned_CompressionAndExpansionFold(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	series := flatSeries(t, 20)
	scores := map[string]float64{
		"Trend Predictability": 0.1,
		"Sideways Consistency": 0.2,
		"Compression":          0.3,
		"Breakout Up":          0.9,
		"Breakout Down":        0.1,
	}

	scoring.SetRegimeModel(&scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: -8},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: 5},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: -8},
		},
	})
	got, ok := dominantRegimeLearned(scores, series)
	if !ok || got != "compression" {
		t.Fatalf("got (%q,%v) want compression", got, ok)
	}

	scoring.SetRegimeModel(&scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: -8},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: -8},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: 5},
		},
	})
	got, ok = dominantRegimeLearned(scores, series)
	if !ok || got != "sideways" {
		t.Fatalf("expansion→sideways got (%q,%v)", got, ok)
	}
}

func TestDominantRegimeLearned_TrendResolvesDirection(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	series := risingSeries(t, 30)
	scores := map[string]float64{
		"Trend Predictability": 0.8,
		"Sideways Consistency": 0.1,
		"Compression":          0.1,
		"Breakout Up":          0.1,
		"Breakout Down":        0.1,
	}
	scoring.SetRegimeModel(&scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: 5},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: -8},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: -8},
		},
	})
	got := dominantRegime(scores, series, "up", nil)
	if got != "uptrend" {
		t.Fatalf("got %q want uptrend", got)
	}
}

func TestSetupRegimeFeatures_EmitClassifyParity(t *testing.T) {
	series := risingSeries(t, 40)
	scores := map[string]float64{
		"Trend Predictability": 0.7,
		"Sideways Consistency": 0.2,
		"Compression":          0.15,
		"Breakout Up":          0.4,
		"Breakout Down":        0.1,
	}
	feats := setupRegimeFeatures(scores, series)
	if feats["expansion"] != 0.4 {
		t.Fatalf("expansion=%v want raw Breakout Up 0.4", feats["expansion"])
	}
	if feats["atr_pct"] <= 0 {
		t.Fatalf("atr_pct=%v want >0 (TrueATR)", feats["atr_pct"])
	}
	price, atr := seriesPriceATR(series)
	if atr <= 0 || price <= 0 {
		t.Fatalf("seriesPriceATR price=%v atr=%v", price, atr)
	}
	wantPct := atr / price
	if math.Abs(feats["atr_pct"]-wantPct) > 1e-12 {
		t.Fatalf("emit ATR vs feature atr_pct: feats=%.12f atr/price=%.12f", feats["atr_pct"], wantPct)
	}

	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)
	scoring.SetRegimeModel(&scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: -8},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: -8},
			"expansion":   {Weights: map[string]float64{"expansion": 20}, Bias: 0},
		},
	})
	got, ok := dominantRegimeLearned(scores, series)
	if !ok || got != "sideways" {
		t.Fatalf("dominantRegimeLearned=(%q,%v) want sideways from expansion head", got, ok)
	}
	_, _, _, _, dom, ok := scoring.ClassifyStructure(feats, *scoring.ActiveRegimeModel())
	if !ok || dom != scoring.ClassExpansion {
		t.Fatalf("ClassifyStructure on setupRegimeFeatures dom=%q ok=%v", dom, ok)
	}
}

func TestStructureRegimeCode_IncludesExpansion(t *testing.T) {
	code := structureRegimeCode(0.1, 0.2, 0.15, 0.9)
	if code != 3 {
		t.Fatalf("code=%v want 3 (expansion)", code)
	}
	if structureRegimeCode(0.8, 0.1, 0.1, 0.1) != 1 {
		t.Fatal("want trend")
	}
	if structureRegimeCode(0.1, 0.1, 0.9, 0.1) != 2 {
		t.Fatal("want compression")
	}
}

func TestSeriesPriceATR_ShortSeriesFallback(t *testing.T) {
	series := risingSeries(t, 5) // < TrueATR period+1
	_, atr := seriesPriceATR(series)
	if atr <= 0 {
		t.Fatalf("short series atr=%v want SimpleATR fallback >0", atr)
	}
}

func flatSeries(t *testing.T, n int) domain.CandleSeries {
	t.Helper()
	return buildSeries(t, n, func(i int) float64 { return 100 })
}

func risingSeries(t *testing.T, n int) domain.CandleSeries {
	t.Helper()
	return buildSeries(t, n, func(i int) float64 { return 100 + float64(i) })
}

func buildSeries(t *testing.T, n int, closeAt func(i int) float64) domain.CandleSeries {
	t.Helper()
	sym, err := domain.NewSymbol("BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	tf, err := domain.NewTimeframe("1h")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	candles := make([]domain.Candle, n)
	for i := 0; i < n; i++ {
		px := closeAt(i)
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*time.Hour), px, px+1, px-1, px, 1000)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return series
}
