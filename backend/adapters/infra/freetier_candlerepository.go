package infra

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"pano_chart/backend/domain"
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

	// If baseURL is not Binance, use it as-is (for tests)
	if r.baseURL.Host != "api.binance.com" {
		// Build test/mock URL with query params
		u := *r.baseURL
		q := u.Query()
		q.Set("symbol", symbol.String())
		q.Set("timeframe", timeframe.String())
		q.Set("from", from.Format(time.RFC3339))
		q.Set("to", to.Format(time.RFC3339))
		u.RawQuery = q.Encode()
		fmt.Printf("[freetier] Test/mock URL: %s\n", u.String())
		resp, err := r.client.Get(u.String())
		if err != nil {
			return domain.CandleSeries{}, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return domain.CandleSeries{}, fmt.Errorf("mock: http %d", resp.StatusCode)
		}
		// Expect mock payload (array of objects)
		var items []struct {
			Timestamp string  `json:"timestamp"`
			Open      float64 `json:"open"`
			High      float64 `json:"high"`
			Low       float64 `json:"low"`
			Close     float64 `json:"close"`
			Volume    float64 `json:"volume"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			return domain.CandleSeries{}, err
		}
		candles := make([]domain.Candle, 0, len(items))
		for _, it := range items {
			ts, err := time.Parse(time.RFC3339, it.Timestamp)
			if err != nil {
				return domain.CandleSeries{}, err
			}
			c, err := domain.NewCandle(symbol, timeframe, ts, it.Open, it.High, it.Low, it.Close, it.Volume)
			if err != nil {
				return domain.CandleSeries{}, err
			}
			candles = append(candles, c)
		}
		return domain.NewCandleSeries(symbol, timeframe, candles)
	}

	// Otherwise, use Binance URL and mapping
	interval := timeframe.String()
	binanceURL := fmt.Sprintf("https://api.binance.com/api/v3/uiKlines?symbol=%s&interval=%s", symbol.String(), interval)
	fmt.Printf("[freetier] Binance URL: %s\n", binanceURL)
	resp, err := r.client.Get(binanceURL)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return domain.CandleSeries{}, fmt.Errorf("binance: http %d", resp.StatusCode)
	}
	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return domain.CandleSeries{}, err
	}
	candles := make([]domain.Candle, 0, len(raw))
	for _, arr := range raw {
		if len(arr) < 6 {
			return domain.CandleSeries{}, fmt.Errorf("binance: expected at least 6 fields, got %d", len(arr))
		}
		ts, _ := arr[0].(float64)
		open, _ := arr[1].(string)
		high, _ := arr[2].(string)
		low, _ := arr[3].(string)
		closep, _ := arr[4].(string)
		volume, _ := arr[5].(string)
		openF, _ := strconv.ParseFloat(open, 64)
		highF, _ := strconv.ParseFloat(high, 64)
		lowF, _ := strconv.ParseFloat(low, 64)
		closeF, _ := strconv.ParseFloat(closep, 64)
		volF, _ := strconv.ParseFloat(volume, 64)
		tm := time.UnixMilli(int64(ts)).UTC()
		c, err := domain.NewCandle(symbol, timeframe, tm, openF, highF, lowF, closeF, volF)
		if err != nil {
			return domain.CandleSeries{}, err
		}
		candles = append(candles, c)
	}
	return domain.NewCandleSeries(symbol, timeframe, candles)
}
