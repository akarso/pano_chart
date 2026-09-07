package infra_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	infra "pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/ports"
)

func TestBinanceFuturesClient_ImplementsPort(t *testing.T) {
	// compile-time check
	var _ ports.FuturesDataPort = infra.NewBinanceFuturesClient("", http.DefaultClient)
}

// --- FundingRate ---

func TestBinanceFuturesClient_FundingRate_ParsesRealResponseShape(t *testing.T) {
	// Fixture matches a live GET /fapi/v1/premiumIndex?symbol=BTCUSDT
	// response verified against the real API during PR-081 implementation.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbol":"BTCUSDT","markPrice":"79599.53797101","indexPrice":"79635.62369565","estimatedSettlePrice":"79584.97426575","lastFundingRate":"0.00003858","interestRate":"0.00010000","nextFundingTime":1788796800000,"time":1788787264000}`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	rate, err := c.FundingRate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 0.00003858 {
		t.Errorf("expected rate 0.00003858, got %v", rate)
	}
}

func TestBinanceFuturesClient_FundingRate_InvalidSymbolReturnsError(t *testing.T) {
	// Fixture matches Binance's real HTTP 400 body for an unknown symbol,
	// verified live during PR-081 implementation.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":-1121,"msg":"Invalid symbol."}`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.FundingRate(context.Background(), "NOTASYMBOL")
	if err == nil {
		t.Fatal("expected error for invalid symbol")
	}
}

// --- OpenInterestHistory ---

func TestBinanceFuturesClient_OpenInterestHistory_ParsesRealResponseShape(t *testing.T) {
	// Fixture matches a live GET /futures/data/openInterestHist response
	// (oldest-to-newest, as Binance actually returns it).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"symbol":"BTCUSDT","sumOpenInterest":"107259.19900000","sumOpenInterestValue":"8533728551.63567300","CMCCirculatingSupply":"20080771.00000000","timestamp":1788786300000},
			{"symbol":"BTCUSDT","sumOpenInterest":"107240.84300000","sumOpenInterestValue":"8529275487.55589900","CMCCirculatingSupply":"20080771.00000000","timestamp":1788786600000},
			{"symbol":"BTCUSDT","sumOpenInterest":"107227.24700000","sumOpenInterestValue":"8534238034.17940000","CMCCirculatingSupply":"20080771.00000000","timestamp":1788786900000}
		]`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	oi, err := c.OpenInterestHistory(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []float64{107259.199, 107240.843, 107227.247}
	if len(oi) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(oi))
	}
	for i := range want {
		if oi[i] != want[i] {
			t.Errorf("entry %d: expected %v, got %v", i, want[i], oi[i])
		}
	}
}

func TestBinanceFuturesClient_OpenInterestHistory_EmptyArrayReturnsError(t *testing.T) {
	// Binance returns HTTP 200 with an empty array for a symbol with no
	// futures market — verified live during PR-081 implementation
	// (unlike premiumIndex, which returns an HTTP 400 for the same case).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.OpenInterestHistory(context.Background(), "NOTASYMBOL")
	if err == nil {
		t.Fatal("expected error for an empty (no futures market) response")
	}
}

// --- LongShortRatio ---

func TestBinanceFuturesClient_LongShortRatio_ParsesRealResponseShape(t *testing.T) {
	// Fixture matches a live GET /futures/data/globalLongShortAccountRatio response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"symbol":"BTCUSDT","longAccount":"0.5268","longShortRatio":"1.1133","shortAccount":"0.4732","timestamp":1788786900000}]`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	ratio, err := c.LongShortRatio(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ratio != 0.5268 {
		t.Errorf("expected ratio 0.5268, got %v", ratio)
	}
}

func TestBinanceFuturesClient_LongShortRatio_EmptyArrayReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.LongShortRatio(context.Background(), "NOTASYMBOL")
	if err == nil {
		t.Fatal("expected error for an empty (no futures market) response")
	}
}
