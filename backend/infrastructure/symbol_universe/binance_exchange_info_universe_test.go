package symbol_universe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockExchangeInfoResponse(symbols []map[string]interface{}) []byte {
	resp := map[string]interface{}{"symbols": symbols}
	b, _ := json.Marshal(resp)
	fmt.Printf("[test] mock exchangeInfo JSON: %s\n", string(b))
	return b
}

func mockTicker24hrResponse(tickers []map[string]interface{}) []byte {
	b, _ := json.Marshal(tickers)
	fmt.Printf("[test] mock ticker24hr JSON: %s\n", string(b))
	return b
}

func TestBinanceExchangeInfoUniverse_VolumeSortAndTieBreaker(t *testing.T) {
	// 3 symbols, all valid, with different volumes, and a tie
	mockSymbols := []map[string]interface{}{
		{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "ETHUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "BNBUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
	}
	t.Logf("mock symbols: %+v", mockSymbols)
	mockTickers := []map[string]interface{}{
		{"symbol": "BTCUSDT", "quoteVolume": "1000"},
		{"symbol": "ETHUSDT", "quoteVolume": "2000"},
		{"symbol": "BNBUSDT", "quoteVolume": "2000"}, // tie with ETHUSDT
	}
	t.Logf("mock tickers: %+v", mockTickers)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if r.URL.Path == "/api/v3/exchangeInfo" || r.URL.Path == "" {
			_, _ = w.Write(mockExchangeInfoResponse(mockSymbols))
		} else {
			_, _ = w.Write(mockTicker24hrResponse(mockTickers))
		}
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), 0)
	exchangeInfoURL, tickerURL := BuildBinanceURLs(ts.URL + "/api/v3")
	syms, err := uni.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// BNBUSDT and ETHUSDT have same volume, should be sorted alphabetically between them
	want := []string{"BNBUSDT", "ETHUSDT", "BTCUSDT"}
	if len(syms) != len(want) {
		t.Fatalf("expected %d symbols, got %d", len(want), len(syms))
	}
	for i, s := range syms {
		if s.String() != want[i] {
			t.Errorf("expected %s at %d, got %s", want[i], i, s.String())
		}
	}
}

func TestBinanceExchangeInfoUniverse_LimitAfterSort(t *testing.T) {
	mockSymbols := []map[string]interface{}{
		{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "ETHUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
		{"symbol": "BNBUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
	}
	t.Logf("mock symbols: %+v", mockSymbols)
	mockTickers := []map[string]interface{}{
		{"symbol": "BTCUSDT", "quoteVolume": "1000"},
		{"symbol": "ETHUSDT", "quoteVolume": "2000"},
		{"symbol": "BNBUSDT", "quoteVolume": "3000"},
	}
	t.Logf("mock tickers: %+v", mockTickers)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if r.URL.Path == "/api/v3/exchangeInfo" || r.URL.Path == "" {
			_, _ = w.Write(mockExchangeInfoResponse(mockSymbols))
		} else {
			_, _ = w.Write(mockTicker24hrResponse(mockTickers))
		}
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), 2)
	exchangeInfoURL, tickerURL := BuildBinanceURLs(ts.URL + "/api/v3")
	syms, err := uni.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"BNBUSDT", "ETHUSDT"} // top 2 by volume
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(syms))
	}
	for i, s := range syms {
		if s.String() != want[i] {
			t.Errorf("expected %s at %d, got %s", want[i], i, s.String())
		}
	}
}

func TestBinanceExchangeInfoUniverse_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if r.URL.Path == "/api/v3/exchangeInfo" || r.URL.Path == "" {
			_, _ = w.Write(mockExchangeInfoResponse(nil))
		} else {
			_, _ = w.Write(mockTicker24hrResponse(nil))
		}
	}))
	defer ts.Close()

	uni := NewBinanceExchangeInfoUniverse(ts.Client(), 0)
	exchangeInfoURL, tickerURL := BuildBinanceURLs(ts.URL + "/api/v3")
	syms, err := uni.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(syms) != 0 {
		t.Fatalf("expected 0 symbols, got %d", len(syms))
	}
}

func TestBinanceExchangeInfoUniverse_ErrorCases(t *testing.T) {
	// Non-200 exchangeInfo
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts1.Close()
	uni := NewBinanceExchangeInfoUniverse(ts1.Client(), 0)
	exchangeInfoURL, tickerURL := BuildBinanceURLs(ts1.URL + "/api/v3")
	_, err := uni.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err == nil {
		t.Error("expected error on non-200 exchangeInfo")
	}

	// Malformed JSON exchangeInfo
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts2.Close()
	uni2 := NewBinanceExchangeInfoUniverse(ts2.Client(), 0)
	exchangeInfoURL, tickerURL = BuildBinanceURLs(ts2.URL + "/api/v3")
	_, err = uni2.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err == nil {
		t.Error("expected error on malformed exchangeInfo json")
	}

	// Non-200 ticker
	ts3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/exchangeInfo" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write(mockExchangeInfoResponse([]map[string]interface{}{
				{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
			}))
		} else {
			w.WriteHeader(500)
		}
	}))
	defer ts3.Close()
	uni3 := NewBinanceExchangeInfoUniverse(ts3.Client(), 0)
	exchangeInfoURL, tickerURL = BuildBinanceURLs(ts3.URL + "/api/v3")
	_, err = uni3.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err == nil {
		t.Error("expected error on non-200 ticker")
	}

	// Malformed JSON ticker
	ts4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/exchangeInfo" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write(mockExchangeInfoResponse([]map[string]interface{}{
				{"symbol": "BTCUSDT", "status": "TRADING", "quoteAsset": "USDT", "isSpotTradingAllowed": true},
			}))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("not json"))
		}
	}))
	defer ts4.Close()
	uni4 := NewBinanceExchangeInfoUniverse(ts4.Client(), 0)
	exchangeInfoURL, tickerURL = BuildBinanceURLs(ts4.URL + "/api/v3")
	_, err = uni4.Symbols(context.Background(), exchangeInfoURL, tickerURL)
	if err == nil {
		t.Error("expected error on malformed ticker json")
	}
}
