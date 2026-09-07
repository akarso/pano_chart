package infra_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	var unavailable *infra.ErrSymbolDataUnavailable
	if !errors.As(err, &unavailable) {
		t.Errorf("expected *infra.ErrSymbolDataUnavailable (a confirmed, cacheable symbol-data signal), got %T: %v", err, err)
	}
}

// --- OpenInterestHistory ---

// openInterestHistEntryJSON builds one GET /futures/data/openInterestHist
// entry in Binance's real response shape (verified live during PR-081
// implementation), for building fixtures of arbitrary length.
func openInterestHistEntryJSON(sumOpenInterest string, timestamp int64) string {
	return fmt.Sprintf(
		`{"symbol":"BTCUSDT","sumOpenInterest":%q,"sumOpenInterestValue":"8533728551.63567300","CMCCirculatingSupply":"20080771.00000000","timestamp":%d}`,
		sumOpenInterest, timestamp)
}

func TestBinanceFuturesClient_OpenInterestHistory_ParsesRealResponseShape(t *testing.T) {
	// 12 entries — matches openInterestHistLimit and clears
	// oiExpansionMinPoints, so this exercises the full happy path rather
	// than the insufficient-history rejection (covered separately below).
	const n = 12
	values := make([]float64, n)
	entries := make([]string, n)
	base := 107259.199
	baseTS := int64(1788786300000)
	for i := 0; i < n; i++ {
		values[i] = base - float64(i)*10
		entries[i] = openInterestHistEntryJSON(fmt.Sprintf("%.8f", values[i]), baseTS+int64(i)*300000)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[%s]`, strings.Join(entries, ","))
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	oi, err := c.OpenInterestHistory(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(oi) != n {
		t.Fatalf("expected %d entries, got %d", n, len(oi))
	}
	for i := range values {
		if oi[i] != values[i] {
			t.Errorf("entry %d: expected %v, got %v", i, values[i], oi[i])
		}
	}
}

func TestBinanceFuturesClient_OpenInterestHistory_InsufficientHistoryReturnsError(t *testing.T) {
	// CR follow-up: Binance can return a nonzero but short series for a
	// newly-listed or thin futures market — fewer points than
	// oiExpansion (application/risk/oi_model.go) actually needs (10).
	// Before this fix, that series passed straight through and silently
	// scored as "no OI expansion" instead of surfacing as insufficient
	// data.
	const tooFew = 3
	entries := make([]string, tooFew)
	for i := 0; i < tooFew; i++ {
		entries[i] = openInterestHistEntryJSON("107259.19900000", 1788786300000+int64(i)*300000)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[%s]`, strings.Join(entries, ","))
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.OpenInterestHistory(context.Background(), "BTCUSDT")
	if err == nil {
		t.Fatal("expected an error for a series shorter than oiExpansion's minimum")
	}
	var unavailable *infra.ErrSymbolDataUnavailable
	if !errors.As(err, &unavailable) {
		t.Errorf("expected *infra.ErrSymbolDataUnavailable (a confirmed, cacheable symbol-data signal), got %T: %v", err, err)
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
	var unavailable *infra.ErrSymbolDataUnavailable
	if !errors.As(err, &unavailable) {
		t.Errorf("expected *infra.ErrSymbolDataUnavailable (a confirmed, cacheable symbol-data signal), got %T: %v", err, err)
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
	var unavailable *infra.ErrSymbolDataUnavailable
	if !errors.As(err, &unavailable) {
		t.Errorf("expected *infra.ErrSymbolDataUnavailable (a confirmed, cacheable symbol-data signal), got %T: %v", err, err)
	}
}

// --- getJSON edge cases (CR follow-up) ---

func TestBinanceFuturesClient_NonJSONErrorBody_IncludesRawBodyInError(t *testing.T) {
	// A proxy/WAF error page or a 429/418 rate-limit body won't match
	// binanceAPIError's {"code":...,"msg":...} shape — the raw body should
	// still show up in the error for production triage, not just the bare
	// status code.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `<html><body>Too Many Requests</body></html>`)
	}))
	defer server.Close()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.FundingRate(context.Background(), "BTCUSDT")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "Too Many Requests") {
		t.Errorf("expected the raw response body to appear in the error, got: %v", err)
	}
	// CR follow-up: a 429 doesn't match Binance's structured error shape,
	// so it must NOT be *ErrSymbolDataUnavailable — RedisCachedFuturesData
	// relies on that distinction to avoid caching a rate-limit response as
	// if it were a confirmed "this symbol doesn't exist" signal.
	var unavailable *infra.ErrSymbolDataUnavailable
	if errors.As(err, &unavailable) {
		t.Errorf("expected a plain (non-cacheable) error for a 429, got *infra.ErrSymbolDataUnavailable: %v", err)
	}
}

func TestBinanceFuturesClient_NetworkError_PropagatesAsError(t *testing.T) {
	// A closed/unreachable server (client.Do failure) must surface as an
	// error, not panic or hang.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // closed immediately — connection will be refused

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	_, err := c.FundingRate(context.Background(), "BTCUSDT")
	if err == nil {
		t.Fatal("expected an error for a network-level failure")
	}
	// CR follow-up: a network-level failure says nothing about the symbol
	// itself, so it must not be classified as a cacheable symbol-data
	// signal either.
	var unavailable *infra.ErrSymbolDataUnavailable
	if errors.As(err, &unavailable) {
		t.Errorf("expected a plain (non-cacheable) error for a network failure, got *infra.ErrSymbolDataUnavailable: %v", err)
	}
}

func TestBinanceFuturesClient_ContextCancellation_PropagatesPromptly(t *testing.T) {
	// A slow/hanging server combined with an already-cancelled context must
	// abort quickly via ctx, not block until the server responds.
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never responds until the test closes this
	}))
	// Single deferred cleanup, ordered explicitly: unblock the handler
	// first so its in-flight connection can actually finish, then close the
	// server — server.Close() waits for active connections, so closing
	// block via a second, later-registered defer (LIFO: Close() would run
	// BEFORE close(block)) would itself hang.
	defer func() {
		close(block)
		server.Close()
	}()

	c := infra.NewBinanceFuturesClient(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.FundingRate(ctx, "BTCUSDT")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a context-deadline error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FundingRate did not return promptly after context deadline — cancellation isn't propagating")
	}
}
