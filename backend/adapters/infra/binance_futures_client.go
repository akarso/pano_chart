package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultBinanceFuturesBaseURL is Binance's public USD-M Futures API host.
const DefaultBinanceFuturesBaseURL = "https://fapi.binance.com"

// BinanceFuturesClient implements ports.FuturesDataPort against Binance's
// public Futures REST API. All three endpoints used here are public
// (no API key/signature required) — see PR-081.
type BinanceFuturesClient struct {
	baseURL string
	client  *http.Client
}

// NewBinanceFuturesClient constructs the adapter. If base is empty,
// DefaultBinanceFuturesBaseURL is used. If client is nil, http.DefaultClient
// is used.
func NewBinanceFuturesClient(base string, client *http.Client) *BinanceFuturesClient {
	if base == "" {
		base = DefaultBinanceFuturesBaseURL
	}
	base = strings.TrimRight(base, "/")
	if client == nil {
		client = http.DefaultClient
	}
	return &BinanceFuturesClient{baseURL: base, client: client}
}

// binanceAPIError is the error body shape Binance returns for a bad
// request (e.g. an unknown symbol) — {"code":-1121,"msg":"Invalid symbol."}.
type binanceAPIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// premiumIndexResponse is the subset of GET /fapi/v1/premiumIndex this
// adapter reads.
type premiumIndexResponse struct {
	Symbol          string `json:"symbol"`
	LastFundingRate string `json:"lastFundingRate"`
}

// FundingRate implements ports.FuturesDataPort.
func (c *BinanceFuturesClient) FundingRate(ctx context.Context, symbol string) (float64, error) {
	u := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", c.baseURL, url.QueryEscape(symbol))
	var resp premiumIndexResponse
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return 0, fmt.Errorf("premiumIndex: %w", err)
	}
	rate, err := strconv.ParseFloat(resp.LastFundingRate, 64)
	if err != nil {
		return 0, fmt.Errorf("premiumIndex: parsing lastFundingRate %q: %w", resp.LastFundingRate, err)
	}
	return rate, nil
}

// openInterestHistEntry is one entry of GET /futures/data/openInterestHist.
type openInterestHistEntry struct {
	SumOpenInterest string `json:"sumOpenInterest"`
	Timestamp       int64  `json:"timestamp"`
}

// openInterestHistLimit is how many recent points to request — matches
// oiExpansion's own minimum of 10 data points (application/risk/oi_model.go)
// with headroom, at the 5m period (the shortest Binance offers for this
// endpoint besides 5m itself).
const openInterestHistLimit = 12

// OpenInterestHistory implements ports.FuturesDataPort.
func (c *BinanceFuturesClient) OpenInterestHistory(ctx context.Context, symbol string) ([]float64, error) {
	u := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=5m&limit=%d",
		c.baseURL, url.QueryEscape(symbol), openInterestHistLimit)
	var entries []openInterestHistEntry
	if err := c.getJSON(ctx, u, &entries); err != nil {
		return nil, fmt.Errorf("openInterestHist: %w", err)
	}
	// Binance returns HTTP 200 with an empty array for a symbol with no
	// futures market or no data in range — that's not distinguishable from
	// a genuinely quiet market at the HTTP layer, but the caller (this
	// symbol has no futures market at all) needs to know either way rather
	// than silently proceeding with an empty series — see PR-081 §5.
	if len(entries) == 0 {
		return nil, fmt.Errorf("openInterestHist: no data for symbol %q", symbol)
	}
	// Binance returns entries oldest-to-newest already (ascending
	// timestamp) — no reordering needed, matches oiExpansion's expectation.
	oi := make([]float64, len(entries))
	for i, e := range entries {
		v, err := strconv.ParseFloat(e.SumOpenInterest, 64)
		if err != nil {
			return nil, fmt.Errorf("openInterestHist: parsing sumOpenInterest %q: %w", e.SumOpenInterest, err)
		}
		oi[i] = v
	}
	return oi, nil
}

// longShortRatioEntry is one entry of GET /futures/data/globalLongShortAccountRatio.
type longShortRatioEntry struct {
	LongAccount string `json:"longAccount"`
}

// LongShortRatio implements ports.FuturesDataPort.
func (c *BinanceFuturesClient) LongShortRatio(ctx context.Context, symbol string) (float64, error) {
	u := fmt.Sprintf("%s/futures/data/globalLongShortAccountRatio?symbol=%s&period=5m&limit=1",
		c.baseURL, url.QueryEscape(symbol))
	var entries []longShortRatioEntry
	if err := c.getJSON(ctx, u, &entries); err != nil {
		return 0, fmt.Errorf("globalLongShortAccountRatio: %w", err)
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("globalLongShortAccountRatio: no data for symbol %q", symbol)
	}
	ratio, err := strconv.ParseFloat(entries[0].LongAccount, 64)
	if err != nil {
		return 0, fmt.Errorf("globalLongShortAccountRatio: parsing longAccount %q: %w", entries[0].LongAccount, err)
	}
	return ratio, nil
}

// maxErrorBodyBytes bounds how much of a non-2xx response body getJSON will
// read for its error message — enough to see what a proxy/WAF/rate-limit
// page actually said, without risking an unbounded read on a pathological
// response.
const maxErrorBodyBytes = 512

// getJSON performs a GET request and decodes a successful JSON response
// into out. A non-2xx status is reported with Binance's own error message
// when the body parses as one (binanceAPIError); otherwise the raw body
// (truncated to maxErrorBodyBytes) is included verbatim — a proxy/WAF error
// page or a 429/418 rate-limit body won't match binanceAPIError's shape, and
// "binance http 429" alone isn't enough to diagnose that in production.
func (c *BinanceFuturesClient) getJSON(ctx context.Context, reqURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		var apiErr binanceAPIError
		if jerr := json.Unmarshal(body, &apiErr); jerr == nil && apiErr.Msg != "" {
			return fmt.Errorf("binance http %d: %s (code %d)", resp.StatusCode, apiErr.Msg, apiErr.Code)
		}
		if len(body) == 0 {
			return fmt.Errorf("binance http %d", resp.StatusCode)
		}
		return fmt.Errorf("binance http %d: %s", resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
