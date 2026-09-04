package market_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake CandleProvider ---

type fakeCandle struct {
	symbol    domain.Symbol
	timeframe domain.Timeframe
	ts        time.Time
	close     float64
}

type fakeCandleProvider struct {
	symbols []domain.Symbol
	candles map[string][]fakeCandle
	err     error
}

func (f *fakeCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}

func (f *fakeCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	fcs, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("no data for %s", key)
	}
	candles := make([]domain.Candle, 0, len(fcs))
	for _, fc := range fcs {
		c := domain.NewCandleUnsafe(fc.symbol, fc.timeframe, fc.ts, fc.close, fc.close, fc.close, fc.close, 1000)
		candles = append(candles, c)
	}
	if n < len(candles) {
		candles = candles[len(candles)-n:]
	}
	return domain.NewCandleSeries(sym, tf, candles)
}

func makeSymbol2(s string) domain.Symbol {
	sym, _ := domain.NewSymbol(s)
	return sym
}

func makeTimeframe2(s string) domain.Timeframe {
	tf, _ := domain.NewTimeframe(s)
	return tf
}

func ts4h(idx int) time.Time {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(idx) * 4 * time.Hour)
}

// --- Composite Index Service Tests ---

func TestCompositeIndex_SingleSymbol(t *testing.T) {
	sym := makeSymbol2("BTCUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: sym, timeframe: tf, ts: ts4h(0), close: 50000},
				{symbol: sym, timeframe: tf, ts: ts4h(1), close: 51000},
				{symbol: sym, timeframe: tf, ts: ts4h(2), close: 49000},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Errorf("expected 1 symbol, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(idx.Points))
	}
	if idx.Points[0].Value != 100 {
		t.Errorf("expected first value 100, got %.2f", idx.Points[0].Value)
	}
	if math.Abs(idx.Points[1].Value-102) > 0.01 {
		t.Errorf("expected ~102, got %.2f", idx.Points[1].Value)
	}
	if math.Abs(idx.Points[2].Value-98) > 0.01 {
		t.Errorf("expected ~98, got %.2f", idx.Points[2].Value)
	}
}

func TestCompositeIndex_MedianOfThree(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	sol := makeSymbol2("SOLUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth, sol},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"ETHUSDT:4h": {
				{symbol: eth, timeframe: tf, ts: ts4h(0), close: 200},
				{symbol: eth, timeframe: tf, ts: ts4h(1), close: 200},
			},
			"SOLUSDT:4h": {
				{symbol: sol, timeframe: tf, ts: ts4h(0), close: 50},
				{symbol: sol, timeframe: tf, ts: ts4h(1), close: 52.5},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 3 {
		t.Errorf("expected 3 symbols, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(idx.Points))
	}
	if idx.Points[0].Value != 100 {
		t.Errorf("expected first value 100, got %.2f", idx.Points[0].Value)
	}
	// Median of [100, 105, 110] = 105
	if math.Abs(idx.Points[1].Value-105) > 0.01 {
		t.Errorf("expected median ~105, got %.2f", idx.Points[1].Value)
	}
}

func TestCompositeIndex_EmptyUniverse(t *testing.T) {
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{},
		candles: map[string][]fakeCandle{},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 0 {
		t.Errorf("expected 0, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 0 {
		t.Errorf("expected 0 points, got %d", len(idx.Points))
	}
}

func TestCompositeIndex_SkipsFailedSymbols(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 105},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SymbolCount != 1 {
		t.Errorf("expected 1 contributing symbol, got %d", idx.SymbolCount)
	}
	if len(idx.Points) != 2 {
		t.Errorf("expected 2 points, got %d", len(idx.Points))
	}
}

func TestCompositeIndex_InvalidTimeframe(t *testing.T) {
	provider := &fakeCandleProvider{symbols: []domain.Symbol{}}
	svc := metrics.NewCompositeIndexService(provider, 4)
	_, err := svc.Calculate(context.Background(), "invalid", 200)
	if err == nil {
		t.Error("expected error for invalid timeframe")
	}
}

func TestCompositeIndex_DefaultLimit(t *testing.T) {
	provider := &fakeCandleProvider{symbols: []domain.Symbol{}}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "1h", 0)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", idx.Timeframe)
	}
}

func TestCompositeIndex_MedianEvenCount(t *testing.T) {
	btc := makeSymbol2("BTCUSDT")
	eth := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")
	provider := &fakeCandleProvider{
		symbols: []domain.Symbol{btc, eth},
		candles: map[string][]fakeCandle{
			"BTCUSDT:4h": {
				{symbol: btc, timeframe: tf, ts: ts4h(0), close: 100},
				{symbol: btc, timeframe: tf, ts: ts4h(1), close: 110},
			},
			"ETHUSDT:4h": {
				{symbol: eth, timeframe: tf, ts: ts4h(0), close: 200},
				{symbol: eth, timeframe: tf, ts: ts4h(1), close: 200},
			},
		},
	}
	svc := metrics.NewCompositeIndexService(provider, 4)
	idx, err := svc.Calculate(context.Background(), "4h", 200)
	if err != nil {
		t.Fatal(err)
	}
	// Median of [100, 110] = (100+110)/2 = 105
	if math.Abs(idx.Points[1].Value-105) > 0.01 {
		t.Errorf("expected median 105, got %.2f", idx.Points[1].Value)
	}
}

// --- HTTP Handler Tests ---

type fakeCompositeCalc struct {
	index mkt.CompositeIndex
	err   error
}

func (f *fakeCompositeCalc) Calculate(_ context.Context, _ string, _ int) (mkt.CompositeIndex, error) {
	if f.err != nil {
		return mkt.CompositeIndex{}, f.err
	}
	return f.index, nil
}

func TestCompositeHandler_DefaultParams(t *testing.T) {
	calc := &fakeCompositeCalc{
		index: mkt.CompositeIndex{
			Timeframe:   "4h",
			SymbolCount: 5,
			Points: []mkt.IndexPoint{
				{Timestamp: 1000, Value: 100.0},
				{Timestamp: 2000, Value: 101.23},
			},
		},
	}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["timeframe"] != "4h" {
		t.Errorf("expected 4h, got %v", resp["timeframe"])
	}
	if int(resp["symbolCount"].(float64)) != 5 {
		t.Errorf("expected 5, got %v", resp["symbolCount"])
	}
	pts := resp["points"].([]interface{})
	if len(pts) != 2 {
		t.Errorf("expected 2 points, got %d", len(pts))
	}
}

func TestCompositeHandler_Error(t *testing.T) {
	calc := &fakeCompositeCalc{err: fmt.Errorf("provider down")}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestCompositeHandler_PointRounding(t *testing.T) {
	calc := &fakeCompositeCalc{
		index: mkt.CompositeIndex{
			Timeframe:   "4h",
			SymbolCount: 1,
			Points: []mkt.IndexPoint{
				{Timestamp: 1000, Value: 100.12345},
			},
		},
	}
	handler := adhttp.NewMarketCompositeHandler(calc)
	req := httptest.NewRequest("GET", "/api/market/composite", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var resp struct {
		Points []struct {
			V float64 `json:"v"`
		} `json:"points"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Points[0].V != 100.12 {
		t.Errorf("expected 100.12, got %.5f", resp.Points[0].V)
	}
}
