package candle_repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	cr "pano_chart/backend/adapters/candle_repository"
	"pano_chart/backend/domain"
	"testing"
	"time"
)

func TestCoinGeckoCandleRepository_GetSeries_MapsResponse(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf := domain.Timeframe1h
	// CoinGecko returns [[timestamp, open, high, low, close], ...]
	mock := [][]interface{}{
		{float64(1672531200000), 47000.0, 47200.0, 46900.0, 47150.0}, // 2023-01-01T00:00:00Z
		{float64(1672534800000), 47150.0, 47300.0, 47100.0, 47200.0}, // 2023-01-01T01:00:00Z
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(mock); err != nil {
			panic(err)
		}
	}))
	defer server.Close()
	repo := cr.NewCoinGeckoCandleRepository(server.Client())
	repo.BaseURL = server.URL
	series, err := repo.GetSeries(context.Background(), sym, tf, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if series.Len() != 2 {
		t.Fatalf("expected 2 candles, got %d", series.Len())
	}
	c, _ := series.At(0)
	if c.Close() != 47150.0 {
		t.Errorf("expected close 47150, got %v", c.Close())
	}
}

// TestCoinGeckoCandleRepository_GetSeries_AbortsOnContextCancellation proves
// the propagated ctx reaches the underlying HTTP request: the handler
// blocks until the client has connected, then the test cancels ctx and
// asserts GetSeries returns promptly with an error wrapping
// context.Canceled, rather than waiting for the handler to ever respond.
func TestCoinGeckoCandleRepository_GetSeries_AbortsOnContextCancellation(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf := domain.Timeframe1h

	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)

	repo := cr.NewCoinGeckoCandleRepository(server.Client())
	repo.BaseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	type callResult struct {
		err error
	}
	resultCh := make(chan callResult, 1)
	start := time.Now()
	go func() {
		_, err := repo.GetSeries(ctx, sym, tf, time.Time{}, time.Time{})
		resultCh <- callResult{err: err}
	}()

	var res callResult
	select {
	case res = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("GetSeries did not return within the test timeout — cancellation is not propagating")
	}
	elapsed := time.Since(start)

	if res.err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(res.err, context.Canceled) {
		t.Fatalf("expected error wrapping context.Canceled, got %v", res.err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected GetSeries to abort promptly on cancellation, took %v", elapsed)
	}
}

func TestCoinGeckoCandleRepository_UnsupportedTimeframe(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	repo := cr.NewCoinGeckoCandleRepository(nil)
	_, err := repo.GetSeries(context.Background(), sym, domain.Timeframe5m, time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error for unsupported timeframe")
	}
}
