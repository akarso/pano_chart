package infra_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infra "pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// sampleResponse is the external API payload shape used in tests.
type sampleResponseItem struct {
	Timestamp string  `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
}

func TestFreeTierCandleRepository_ImplementsPort(t *testing.T) {
	// compile-time check
	var _ ports.CandleRepositoryPort = infra.NewFreeTierCandleRepository("", http.DefaultClient, nil)
	_ = t
}

func TestFreeTierCandleRepository_MapsValidResponseToCandleSeries(t *testing.T) {
	// Setup httptest server returning a valid payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := []sampleResponseItem{{
			Timestamp: "2026-01-01T12:00:00Z",
			Open:      100,
			High:      110,
			Low:       90,
			Close:     105,
			Volume:    1000,
		}}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)

	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	series, err := repo.GetSeries(context.Background(), sym, tf, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if series.Len() != 1 {
		t.Fatalf("expected 1 candle, got %d", series.Len())
	}
	c, _ := series.At(0)
	if !c.Timestamp().Equal(from) {
		t.Fatalf("expected timestamp %v, got %v", from, c.Timestamp())
	}
	if c.Open() != 100 || c.Close() != 105 {
		t.Fatalf("unexpected OHLC values")
	}
}

func TestFreeTierCandleRepository_ReturnsErrorOnHTTPFailure(t *testing.T) {
	// Server returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)

	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(context.Background(), sym, tf, from, to)
	if err == nil {
		t.Fatal("expected error for HTTP failure")
	}
}

func TestFreeTierCandleRepository_ReturnsErrorOnInvalidPayload(t *testing.T) {
	// Server returns malformed JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)

	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(context.Background(), sym, tf, from, to)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// TestFreeTierCandleRepository_GetSeries_AbortsOnContextCancellation proves
// the propagated ctx actually reaches the underlying HTTP request: the
// handler blocks until the client has connected, then the test cancels ctx
// and asserts GetSeries returns promptly with an error wrapping
// context.Canceled, rather than waiting for the handler to ever respond.
func TestFreeTierCandleRepository_GetSeries_AbortsOnContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)

	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	start := time.Now()
	_, err := repo.GetSeries(ctx, sym, tf, from, to)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error wrapping context.Canceled, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected GetSeries to abort promptly on cancellation, took %v", elapsed)
	}
}

// TestFreeTierCandleRepository_GetLastNCandles_AbortsOnContextCancellation
// is the same proof at the GetLastNCandles entry point (the method the
// original CR finding named) rather than GetSeries directly.
func TestFreeTierCandleRepository_GetLastNCandles_AbortsOnContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)

	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	start := time.Now()
	_, err := repo.GetLastNCandles(ctx, sym, tf, 10)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error wrapping context.Canceled, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected GetLastNCandles to abort promptly on cancellation, took %v", elapsed)
	}
}

func TestFreeTierCandleRepository_GetSeries_MapsProviderResponse(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf := domain.Timeframe1h
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode([]map[string]interface{}{
			{"timestamp": "2024-01-01T00:00:00Z", "open": 1.0, "high": 2.0, "low": 0.5, "close": 1.5, "volume": 100},
		}); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client(), nil)
	series, err := repo.GetSeries(context.Background(), sym, tf, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if series.Len() != 1 {
		t.Fatalf("expected 1 candle, got %d", series.Len())
	}
	c, _ := series.At(0)
	if c.Close() != 1.5 {
		t.Errorf("expected close 1.5, got %v", c.Close())
	}
}
