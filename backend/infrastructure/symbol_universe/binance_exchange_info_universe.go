package symbol_universe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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
	// 1. Fetch exchangeInfo
	infoURL := b.baseURL
	if infoURL == "" {
		infoURL = "https://api.binance.com/api/v3/exchangeInfo"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", infoURL, nil)
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

	// 2. Filter for USDT spot trading pairs, status=TRADING
	filtered := make(map[string]struct{})
	for _, s := range info.Symbols {
		if s.QuoteAsset == "USDT" && s.Status == "TRADING" && s.IsSpotTradingAllowed {
			filtered[s.Symbol] = struct{}{}
		}
	}
	if len(filtered) == 0 {
		return []domain.Symbol{}, nil
	}

	// 3. Fetch 24h ticker stats
	tickerURL := b.baseURL
	//nolint:staticcheck // QF1003: tagged switch not appropriate here, see PR-020 rationale
	if tickerURL == "" {
		tickerURL = "https://api.binance.com/api/v3/ticker/24hr"
	} else {
		// Always append /ticker/24hr to the base URL root (for tests and prod)
		if tickerURL[len(tickerURL)-1] == '/' {
			tickerURL += "ticker/24hr"
		} else {
			tickerURL += "/ticker/24hr"
		}
	}
	treq, err := http.NewRequestWithContext(ctx, "GET", tickerURL, nil)
	if err != nil {
		return nil, err
	}
	tresp, err := b.client.Do(treq)
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := tresp.Body.Close()
		if cerr != nil {
			fmt.Printf("warning: error closing ticker response body: %v\n", cerr)
		}
	}()
	if tresp.StatusCode != 200 {
		return nil, fmt.Errorf("binance: ticker http %d", tresp.StatusCode)
	}
	var tickers []struct {
		Symbol      string `json:"symbol"`
		QuoteVolume string `json:"quoteVolume"`
	}
	if err := json.NewDecoder(tresp.Body).Decode(&tickers); err != nil {
		return nil, err
	}

	// 4. Build list of filtered symbols with their quoteVolume
	type symVol struct {
		sym  string
		vol  float64
	}
	var svs []symVol
	for _, t := range tickers {
		if _, ok := filtered[t.Symbol]; ok {
			v, err := strconv.ParseFloat(t.QuoteVolume, 64)
			if err != nil {
				v = 0
			}
			svs = append(svs, symVol{sym: t.Symbol, vol: v})
		}
	}
	// There may be symbols in filtered not present in tickers (shouldn't happen, but be robust)
	for s := range filtered {
		found := false
		for _, sv := range svs {
			if sv.sym == s {
				found = true
				break
			}
		}
		if !found {
			svs = append(svs, symVol{sym: s, vol: 0})
		}
	}

	// 5. Sort by descending volume, then alphabetically
	sort.Slice(svs, func(i, j int) bool {
		if svs[i].vol == svs[j].vol {
			return svs[i].sym < svs[j].sym
		}
		return svs[i].vol > svs[j].vol
	})

	// 6. Apply limit after sorting
	if b.limit > 0 && len(svs) > b.limit {
		svs = svs[:b.limit]
	}

	// 7. Convert to domain.Symbol
	out := make([]domain.Symbol, 0, len(svs))
	for _, sv := range svs {
		dsym, err := domain.NewSymbol(sv.sym)
		if err != nil {
			continue // skip invalid
		}
		out = append(out, dsym)
	}
	return out, nil
}

//lint:file-ignore QF1003 false positive: tagged switch not used, see PR discussion
