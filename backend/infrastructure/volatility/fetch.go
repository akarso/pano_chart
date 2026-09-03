package volatility

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

const binanceKlinesURL = "https://api.binance.com/api/v3/klines"

// Fetcher retrieves 1-minute candles from the Binance REST API.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a Fetcher with an injected HTTP client.
func NewFetcher(client *http.Client) *Fetcher {
	return &Fetcher{client: client}
}

// FetchCandles retrieves up to 1000 1-minute candles for the given
// symbol and time range (millisecond timestamps, inclusive).
func (f *Fetcher) FetchCandles(ctx context.Context, symbol string, startTime, endTime int64) ([]Candle, error) {
	url := fmt.Sprintf(
		"%s?symbol=%s&interval=1m&startTime=%d&endTime=%d&limit=1000",
		binanceKlinesURL, symbol, startTime, endTime,
	)
	return f.FetchCandlesFromURL(ctx, url)
}

// FetchCandlesFromURL fetches and parses klines from an arbitrary URL.
// Useful for testing with httptest servers.
func (f *Fetcher) FetchCandlesFromURL(ctx context.Context, url string) ([]Candle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "PanoChart/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance: http %d", resp.StatusCode)
	}

	var raw [][]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	candles := make([]Candle, 0, len(raw))
	for _, r := range raw {
		if len(r) < 5 {
			continue
		}
		var openTime float64
		if err := json.Unmarshal(r[0], &openTime); err != nil {
			continue
		}
		open, err1 := parseFloat(r[1])
		high, err2 := parseFloat(r[2])
		low, err3 := parseFloat(r[3])
		cl, err4 := parseFloat(r[4])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		candles = append(candles, Candle{
			OpenTime: int64(openTime),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    cl,
		})
	}

	return candles, nil
}

func parseFloat(raw json.RawMessage) (float64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(s, 64)
}
