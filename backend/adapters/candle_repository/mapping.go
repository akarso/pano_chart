package candle_repository

import (
	"fmt"
	"time"
	"pano_chart/backend/domain"
)

func mapCoinGeckoOHLCToCandleSeries(symbol domain.Symbol, tf domain.Timeframe, raw [][]interface{}) (domain.CandleSeries, error) {
	candles := make([]domain.Candle, 0, len(raw))
	for _, arr := range raw {
		if len(arr) != 5 {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: expected 5 fields, got %d", len(arr))
		}
		ts, ok := arr[0].(float64)
		if !ok {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: invalid timestamp type")
		}
		open, ok := arr[1].(float64)
		if !ok {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: invalid open type")
		}
		high, ok := arr[2].(float64)
		if !ok {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: invalid high type")
		}
		low, ok := arr[3].(float64)
		if !ok {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: invalid low type")
		}
		closep, ok := arr[4].(float64)
		if !ok {
			return domain.CandleSeries{}, fmt.Errorf("coingecko: invalid close type")
		}
		tm := time.UnixMilli(int64(ts)).UTC()
		c, err := domain.NewCandle(symbol, tf, tm, open, high, low, closep, 0)
		if err != nil {
			return domain.CandleSeries{}, err
		}
		candles = append(candles, c)
	}
	return domain.NewCandleSeries(symbol, tf, candles)
}
