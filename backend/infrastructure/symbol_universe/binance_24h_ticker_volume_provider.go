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
	if resp.StatusCode != 200 {
		return nil, err
	}
	var tickers []struct {
		Symbol      string  `json:"symbol"`
		QuoteVolume string  `json:"quoteVolume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
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
