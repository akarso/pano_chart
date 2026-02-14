package symbol_universe

import "strings"

const (
	DefaultBinanceAPIBaseURL       = "https://api.binance.com/api/v3"
	DefaultBinanceExchangeInfoPath = "/exchangeInfo"
	DefaultBinanceTickerPath       = "/ticker/24hr"
)

// BuildBinanceURLs builds exchangeInfo and ticker URLs from a base URL.
// If base is empty, DefaultBinanceAPIBaseURL is used.
func BuildBinanceURLs(base string) (exchangeInfoURL string, tickerURL string) {
	if base == "" {
		base = DefaultBinanceAPIBaseURL
	}
	base = strings.TrimRight(base, "/")
	return base + DefaultBinanceExchangeInfoPath, base + DefaultBinanceTickerPath
}
