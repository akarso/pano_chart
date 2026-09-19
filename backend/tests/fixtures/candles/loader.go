package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"pano_chart/backend/domain"
)

// RequiredBars is the candle count every golden fixture must contain.
// Matches production sparklinePrecision default (cmd/api/main.go). Keep
// fixtures and that default in sync for golden relevance.
const RequiredBars = 110

// candleFile is the on-disk JSON shape for one archetype fixture.
type candleFile struct {
	Symbol    string         `json:"symbol"`
	Timeframe string         `json:"timeframe"`
	Archetype string         `json:"archetype"`
	Source    string         `json:"source"`
	Recorded  string         `json:"recorded"`
	Candles   []candleBarDTO `json:"candles"`
}

type candleBarDTO struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

// Dir returns the absolute path to tests/fixtures/candles.
func Dir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("fixtures: cannot resolve package directory")
	}
	return filepath.Dir(file)
}

// Load reads name.json (without extension) from the candles fixture directory
// and returns a CandleSeries. Exactly RequiredBars bars are required.
func Load(t *testing.T, name string) domain.CandleSeries {
	t.Helper()
	path := filepath.Join(Dir(), name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixtures.Load(%q): read: %v", name, err)
	}
	var doc candleFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("fixtures.Load(%q): decode: %v", name, err)
	}
	if doc.Archetype != "" && doc.Archetype != name {
		t.Fatalf("fixtures.Load(%q): archetype field %q does not match filename", name, doc.Archetype)
	}
	if len(doc.Candles) != RequiredBars {
		t.Fatalf("fixtures.Load(%q): want %d candles, got %d", name, RequiredBars, len(doc.Candles))
	}
	sym, err := domain.NewSymbol(doc.Symbol)
	if err != nil {
		t.Fatalf("fixtures.Load(%q): symbol: %v", name, err)
	}
	tf, err := domain.NewTimeframe(doc.Timeframe)
	if err != nil {
		t.Fatalf("fixtures.Load(%q): timeframe: %v", name, err)
	}
	candles := make([]domain.Candle, len(doc.Candles))
	for i, b := range doc.Candles {
		ts := time.Unix(b.T, 0).UTC()
		candles[i] = domain.NewCandleUnsafe(sym, tf, ts, b.O, b.H, b.L, b.C, b.V)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("fixtures.Load(%q): series: %v", name, err)
	}
	return series
}
