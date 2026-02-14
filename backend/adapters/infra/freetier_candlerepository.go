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

// GetLastNCandles retrieves the last N completed candles for a given symbol and timeframe.
// Implementation strategy:
// 1. Calculate time range required to fetch at least N+1 candles (to exclude in-progress)
// 2. Fetch series for that range
// 3. Exclude the last (in-progress) candle if present
// 4. Return last N candles
func (r *FreeTierCandleRepository) GetLastNCandles(symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error) {
	if n <= 0 {
		return domain.NewCandleSeries(symbol, timeframe, []domain.Candle{})
	}

	// Calculate time range: fetch enough candles to get N completed ones.
	// Multiply by 1.5 to account for potential gaps and in-progress candle.
	now := time.Now().UTC()
	durationPerCandle := timeframe.Duration()
	timeRangeNeeded := time.Duration(float64(n)*1.5) * durationPerCandle

	from := now.Add(-timeRangeNeeded)
	to := now

	fmt.Printf("[freetier] GetLastNCandles: symbol=%s, tf=%s, n=%d, fetching from %s to %s\n", symbol.String(), timeframe.String(), n, from.Format(time.RFC3339), to.Format(time.RFC3339))

	// Fetch the series
	series, err := r.GetSeries(symbol, timeframe, from, to)
	if err != nil {
		return domain.CandleSeries{}, err
	}

	totalCandles := series.Len()
	fmt.Printf("[freetier] GetLastNCandles: fetched %d candles, needed %d\n", totalCandles, n)

	// Exclude the last (in-progress) candle if it exists and is too recent
	availableCandles := totalCandles
	if totalCandles > 0 {
		lastCandle, _ := series.Last()
		// If the last candle is too recent (within the last timeframe duration), it may be in-progress.
		// Exclude it to be safe.
		if now.Sub(lastCandle.Timestamp()) < durationPerCandle {
			availableCandles--
			fmt.Printf("[freetier] GetLastNCandles: excluded in-progress candle, available=%d\n", availableCandles)
		}
	}

	// Extract the last N candles from the series
	start := 0
	if availableCandles > n {
		start = availableCandles - n
	}

	// Build result series from the extracted range
	resultCandles := make([]domain.Candle, 0, min(n, availableCandles))
	for i := start; i < availableCandles && i < totalCandles; i++ {
		candle, err := series.At(i)
		if err == nil {
			resultCandles = append(resultCandles, candle)
		}
	}

	result, err := domain.NewCandleSeries(symbol, timeframe, resultCandles)
	fmt.Printf("[freetier] GetLastNCandles: returning %d candles\n", len(resultCandles))
	return result, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
