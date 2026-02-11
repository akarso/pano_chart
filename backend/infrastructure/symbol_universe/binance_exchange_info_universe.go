package symbol_universe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"pano_chart/backend/domain"
)

type BinanceExchangeInfoUniverse struct {
	client  *http.Client
	baseURL string
	limit   int
}

func NewBinanceExchangeInfoUniverse(client *http.Client, baseURL string, limit int) *BinanceExchangeInfoUniverse {
	return &BinanceExchangeInfoUniverse{
		client:  client,
		baseURL: baseURL,
		limit:   limit,
	}
}

type exchangeInfoResponse struct {
	Symbols []struct {
		Symbol               string `json:"symbol"`
		Status               string `json:"status"`
		QuoteAsset           string `json:"quoteAsset"`
		IsSpotTradingAllowed bool   `json:"isSpotTradingAllowed"`
	} `json:"symbols"`
}

func (b *BinanceExchangeInfoUniverse) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	url := b.baseURL
	if url == "" {
		url = "https://api.binance.com/api/v3/exchangeInfo"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		return nil, fmt.Errorf("binance: http %d", resp.StatusCode)
	}
	var info exchangeInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	var syms []domain.Symbol
	for _, s := range info.Symbols {
		if s.QuoteAsset == "USDT" && s.Status == "TRADING" && s.IsSpotTradingAllowed {
			dsym, err := domain.NewSymbol(s.Symbol)
			if err != nil {
				continue // skip invalid
			}
			syms = append(syms, dsym)
		}
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i].String() < syms[j].String() })
	if b.limit > 0 && len(syms) > b.limit {
		syms = syms[:b.limit]
	}
	return syms, nil
}
