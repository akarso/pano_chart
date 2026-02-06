package candle_repository_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	cr "pano_chart/backend/adapters/candle_repository"
	"pano_chart/backend/domain"
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
	series, err := repo.GetSeries(sym, tf, time.Time{}, time.Time{})
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

func TestCoinGeckoCandleRepository_UnsupportedTimeframe(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	repo := cr.NewCoinGeckoCandleRepository(nil)
	_, err := repo.GetSeries(sym, domain.Timeframe5m, time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error for unsupported timeframe")
	}
}
