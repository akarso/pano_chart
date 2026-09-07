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

// ErrSymbolDataUnavailable marks a CONFIRMED, symbol-specific "no data"
// signal from Binance — either a structured API error (binanceAPIError,
// e.g. "Invalid symbol") or an HTTP 200 with an empty/insufficient result
// set for a symbol with no futures market or too little history. This is
// deliberately distinct from a transient/infrastructure failure (network
// error, an unparsed non-2xx like 429/5xx, a malformed response) — see
// RedisCachedFuturesData.cachedFetch, which only caches this kind of error
// as symbol-level unavailability. Caching a transient failure the same way
// would keep reporting "unavailable" long after Binance itself recovered.
type ErrSymbolDataUnavailable struct {
	// Reason is exported so tests (in this package and others, e.g. the
	// RedisCachedFuturesData caching tests) can construct one directly via
	// a struct literal rather than needing a dedicated constructor.
	Reason string
}

func (e *ErrSymbolDataUnavailable) Error() string { return e.Reason }

func errSymbolDataUnavailable(format string, args ...interface{}) *ErrSymbolDataUnavailable {
	return &ErrSymbolDataUnavailable{Reason: fmt.Sprintf(format, args...)}
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
// oiExpansionMinPoints with headroom, at the 5m period (the shortest
// Binance offers for this endpoint besides 5m itself).
const openInterestHistLimit = 12

// oiExpansionMinPoints mirrors application/risk/oi_model.go's oiExpansion,
// which silently returns 0 (no expansion) for a series shorter than this —
// kept as a separate constant here (oiExpansion's own 10 is unexported and
// in a different package) rather than an enforced cross-package link, so
// keep the two in sync if either changes. Without this check, a newly-
// listed or thin futures market returning 1-9 points used to pass straight
// through as a normal-looking series and silently score as "no OI
// expansion" via oiExpansion's own threshold — understating crowding risk
// for exactly the kind of market where thin data makes that risk harder to
// see, rather than surfacing it as the insufficient-data case it actually
// is (CR follow-up).
const oiExpansionMinPoints = 10

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
		return nil, errSymbolDataUnavailable("openInterestHist: no data for symbol %q", symbol)
	}
	// Fewer points than oiExpansion actually needs — not empty, but not
	// enough to compute a real expansion reading either. See
	// oiExpansionMinPoints's doc.
	if len(entries) < oiExpansionMinPoints {
		return nil, errSymbolDataUnavailable("openInterestHist: insufficient history for symbol %q (%d points, need >= %d)",
			symbol, len(entries), oiExpansionMinPoints)
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
		return 0, errSymbolDataUnavailable("globalLongShortAccountRatio: no data for symbol %q", symbol)
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
// into out.
//
// A non-2xx status whose body parses as binanceAPIError (Binance's own
// structured error shape) is a CONFIRMED, symbol-specific rejection —
// returned as *ErrSymbolDataUnavailable so RedisCachedFuturesData knows
// it's safe to cache. Anything else — a network-level failure from
// client.Do, a non-2xx status that doesn't match that shape (a 429/5xx rate
// limit, a proxy/WAF page), or a JSON decode failure on an otherwise-2xx
// response — says nothing conclusive about the symbol itself, so it stays
// a plain error: not cached as symbol-level unavailability, since that
// would keep reporting "unavailable" long after a transient issue clears
// (CR follow-up). The raw body (truncated to maxErrorBodyBytes) is still
// included verbatim in the plain-error case for production triage, even
// though it isn't Binance's structured shape.
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
			return errSymbolDataUnavailable("binance http %d: %s (code %d)", resp.StatusCode, apiErr.Msg, apiErr.Code)
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
