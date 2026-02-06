package candle_repository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pano_chart/backend/domain"
)

// CoinGeckoCandleRepository implements domain.CandleRepositoryPort
// Fetches OHLC data from CoinGecko and maps to CandleSeries
// Only supports 15m, 1h, 4h, 1d timeframes
type CoinGeckoCandleRepository struct {
	httpClient *http.Client
	BaseURL    string
}

func NewCoinGeckoCandleRepository(client *http.Client) *CoinGeckoCandleRepository {
   if client == nil {
	   client = http.DefaultClient
   }
   return &CoinGeckoCandleRepository{
	   httpClient: client,
	   BaseURL:    "https://api.coingecko.com/api/v3",
   }
}

func (r *CoinGeckoCandleRepository) GetSeries(symbol domain.Symbol, timeframe domain.Timeframe, from, to time.Time) (domain.CandleSeries, error) {
	cgID, err := symbolToCoinGeckoID(symbol)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	interval, err := timeframeToMinutes(timeframe)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	// CoinGecko only supports vs_currency=usd, days=1-90, interval in [15,60,240,1440]
	url := fmt.Sprintf("%s/coins/%s/ohlc?vs_currency=usd&days=1&interval=%d", r.BaseURL, cgID, interval)
	resp, err := r.httpClient.Get(url)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			// Optionally log or handle the error, but do not shadow the main error path
			fmt.Printf("warning: error closing response body: %v\n", cerr)
		}
	}()
	if resp.StatusCode != 200 {
		return domain.CandleSeries{}, fmt.Errorf("coingecko: http %d", resp.StatusCode)
	}
	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return domain.CandleSeries{}, err
	}
	return mapCoinGeckoOHLCToCandleSeries(symbol, timeframe, raw)
}
