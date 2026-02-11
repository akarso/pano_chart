package symbol_universe

import (
	"context"
	"sort"
	"pano_chart/backend/domain"
)

// StaticBinanceTop15Universe provides a deterministic, hardcoded list of top 15 Binance USDT pairs.
type StaticBinanceTop15Universe struct{}

var binanceTop15 = []string{
	"BTCUSDT",
	"ETHUSDT",
	"BNBUSDT",
	"SOLUSDT",
	"XRPUSDT",
	"ADAUSDT",
	"DOGEUSDT",
	"AVAXUSDT",
	"LINKUSDT",
	"DOTUSDT",
	"MATICUSDT",
	"TRXUSDT",
	"LTCUSDT",
	"ATOMUSDT",
	"UNIUSDT",
}

func NewStaticBinanceTop15Universe() *StaticBinanceTop15Universe {
	return &StaticBinanceTop15Universe{}
}

func (s *StaticBinanceTop15Universe) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	syms := make([]domain.Symbol, 0, len(binanceTop15))
	for _, str := range binanceTop15 {
		sym, err := domain.NewSymbol(str)
		if err != nil {
			return nil, err
		}
		syms = append(syms, sym)
	}
	// Deterministic ordering
	sort.SliceStable(syms, func(i, j int) bool { return syms[i].String() < syms[j].String() })
	return syms, nil
}
