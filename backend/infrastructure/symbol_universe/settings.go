package symbol_universe

import "strings"

const (
	DefaultBinanceAPIBaseURL       = "https://api.binance.com/api/v3"
	DefaultBinanceExchangeInfoPath = "/exchangeInfo"
	DefaultBinanceTickerPath       = "/ticker/24hr"
)

// ExcludedSymbols contains symbols that should be filtered out of the
// trading universe. Stablecoins produce misleading sparklines when
// normalized and are not meaningful for trading analysis.
var ExcludedSymbols = map[string]struct{}{
	"USDCUSDT":  {},
	"USD1USDT":  {},
	"FDUSDUSDT": {},
	"DAIUSDT":   {},
	"TUSDUSDT":  {},
	"BUSDUSDT":  {},
	"USDPUSDT":  {},
	"EURUSDT":   {},
	"GBPUSDT":   {},
	"AEURUSDT":  {},
	"USTCUSDT":  {},
	"PYUSDUSDT": {},
}

// BuildBinanceURLs builds exchangeInfo and ticker URLs from a base URL.
// If base is empty, DefaultBinanceAPIBaseURL is used.
func BuildBinanceURLs(base string) (exchangeInfoURL string, tickerURL string) {
	if base == "" {
		base = DefaultBinanceAPIBaseURL
	}
	base = strings.TrimRight(base, "/")
	return base + DefaultBinanceExchangeInfoPath, base + DefaultBinanceTickerPath
}
