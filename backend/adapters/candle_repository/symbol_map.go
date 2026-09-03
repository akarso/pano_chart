package candle_repository

import (
	"strings"
	"pano_chart/backend/domain"
)

// symbolToCoinGeckoID maps a domain.Symbol to a CoinGecko ID (lowercase, hyphenated)
func symbolToCoinGeckoID(symbol domain.Symbol) (string, error) {
	s := strings.ToLower(symbol.String())
	switch s {
	case "btc", "btcusdt":
		return "bitcoin", nil
	case "eth", "ethusdt":
		return "ethereum", nil
	// Add more mappings as needed
	default:
		return "", ErrInvalidSymbol
	}
}
