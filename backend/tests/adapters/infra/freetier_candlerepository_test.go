package infra_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
	infra "pano_chart/backend/adapters/infra"
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
	var _ ports.CandleRepositoryPort = infra.NewFreeTierCandleRepository("", http.DefaultClient)
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

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client())

	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	series, err := repo.GetSeries(sym, tf, from, to)
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

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client())

	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(sym, tf, from, to)
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

	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client())

	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(sym, tf, from, to)
	if err == nil {
		t.Fatal("expected error for invalid payload")
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
	repo := infra.NewFreeTierCandleRepository(server.URL, server.Client())
	series, err := repo.GetSeries(sym, tf, time.Time{}, time.Time{})
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
