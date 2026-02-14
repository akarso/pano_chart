package symbol_universe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type Binance24hTickerVolumeProvider struct {
	client  *http.Client
	url     string
}

func NewBinance24hTickerVolumeProvider(client *http.Client, url string) *Binance24hTickerVolumeProvider {
	return &Binance24hTickerVolumeProvider{client: client, url: url}
}

func (b *Binance24hTickerVolumeProvider) Volumes(ctx context.Context) (map[string]float64, error) {
	fmt.Printf("[binance_24h_ticker_volume_provider] Requesting ticker URL: %s\n", b.url)
	req, err := http.NewRequestWithContext(ctx, "GET", b.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			fmt.Printf("warning: error closing response body: %v\n", cerr)
		}
	}()
	fmt.Printf("[binance_24h_ticker_volume_provider] HTTP status: %d\n", resp.StatusCode)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance: ticker http %d", resp.StatusCode)
	}
	var tickers []struct {
		Symbol      string  `json:"symbol"`
		QuoteVolume string  `json:"quoteVolume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
	}
	fmt.Printf("[binance_24h_ticker_volume_provider] Parsed %d tickers\n", len(tickers))
	if len(tickers) > 0 {
		fmt.Printf("[binance_24h_ticker_volume_provider] Sample ticker: %+v\n", tickers[0])
	}
	vols := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		v, err := strconv.ParseFloat(t.QuoteVolume, 64)
		if err == nil {
			vols[t.Symbol] = v
		}
	}
	return vols, nil
}
