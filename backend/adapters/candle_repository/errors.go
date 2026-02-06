package candle_repository

import "errors"

var (
	ErrUnsupportedTimeframe = errors.New("unsupported timeframe for CoinGecko")
	ErrInvalidSymbol        = errors.New("invalid symbol for CoinGecko")
)
