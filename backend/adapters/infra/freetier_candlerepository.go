package infra

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/akarso/pano_chart/backend/domain"
)

// FreeTierCandleRepository implements ports.CandleRepositoryPort using a free-tier HTTP API.
type FreeTierCandleRepository struct {
	baseURL *url.URL
	client  *http.Client
}

// NewFreeTierCandleRepository constructs the adapter. BaseURL must be a valid URL.
func NewFreeTierCandleRepository(base string, client *http.Client) *FreeTierCandleRepository {
	u, _ := url.Parse(base)
	if client == nil {
		client = http.DefaultClient
	}
	return &FreeTierCandleRepository{baseURL: u, client: client}
}

// GetSeries implements CandleRepositoryPort. It performs a single request to the external API
// and translates the response into domain.CandleSeries.
func (r *FreeTierCandleRepository) GetSeries(symbol domain.Symbol, timeframe domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	if r.baseURL == nil {
		return domain.CandleSeries{}, fmt.Errorf("invalid base URL")
	}

	q := r.baseURL.Query()
	q.Set("symbol", symbol.String())
	q.Set("timeframe", timeframe.String())
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))

	endpoint := *r.baseURL
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return domain.CandleSeries{}, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.CandleSeries{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Expected payload: JSON array of objects with timestamp (RFC3339), open, high, low, close, volume
	var items []struct {
		Timestamp string  `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    float64 `json:"volume"`
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&items); err != nil {
		return domain.CandleSeries{}, err
	}

	candles := make([]domain.Candle, 0, len(items))
	for _, it := range items {
		ts, err := time.Parse(time.RFC3339, it.Timestamp)
		if err != nil {
			return domain.CandleSeries{}, err
		}
		// Ensure UTC
		if ts.Location() != time.UTC {
			ts = ts.UTC()
		}
		c, err := domain.NewCandle(symbol, timeframe, ts, it.Open, it.High, it.Low, it.Close, it.Volume)
		if err != nil {
			return domain.CandleSeries{}, err
		}
		candles = append(candles, c)
	}

	return domain.NewCandleSeries(symbol, timeframe, candles)
}
