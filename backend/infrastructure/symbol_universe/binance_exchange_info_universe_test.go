package symbol_universe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockExchangeInfoResponse(symbols []map[string]interface{}) []byte {
	resp := map[string]interface{}{"symbols": symbols}
	b, _ := json.Marshal(resp)
	return b
}

func TestBinanceExchangeInfoUniverse_FiltersAndSorts(t *testing.T) {
	mockSymbols := []map[string]interface{}{
		{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "ETHUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "FOOBTC", "status": "TRADING", "quoteAsset": "BTC", "isSpotTradingAllowed": true},
		{"symbol": "XRPUSDT", "status": "BREAK", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "ADAUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": false},
		{"symbol": "BNBUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(mockExchangeInfoResponse(mockSymbols))
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), ts.URL, 0)
	syms, err := uni.Symbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"BNBUSDT", "BTCUSDT", "ETHUSDT"}
	if len(syms) != len(want) {
		t.Fatalf("expected %d symbols, got %d", len(want), len(syms))
	}
	for i, s := range syms {
		if s.String() != want[i] {
			t.Errorf("expected %s at %d, got %s", want[i], i, s.String())
		}
	}
}

func TestBinanceExchangeInfoUniverse_Limit(t *testing.T) {
	mockSymbols := []map[string]interface{}{
		{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "ETHUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "BNBUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(mockExchangeInfoResponse(mockSymbols))
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), ts.URL, 2)
	syms, err := uni.Symbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(syms))
	}
}

func TestBinanceExchangeInfoUniverse_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(mockExchangeInfoResponse(nil))
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), ts.URL, 0)
	syms, err := uni.Symbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(syms) != 0 {
		t.Fatalf("expected 0 symbols, got %d", len(syms))
	}
}
