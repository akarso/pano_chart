package usecases

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// FakeRanker is a test double for the RankSymbols use case.
type FakeRanker struct {
	rankedSymbols []usecases.RankedSymbol
	err           error
}

func NewFakeRanker(symbols []usecases.RankedSymbol, err error) *FakeRanker {
	return &FakeRanker{
		rankedSymbols: symbols,
		err:           err,
	}
}

func (f *FakeRanker) Rank(series map[domain.Symbol]domain.CandleSeries) ([]usecases.RankedSymbol, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rankedSymbols, nil
}

// FakeCandleRepository is a test double for the CandleRepositoryPort.
type FakeCandleRepository struct {
	candlesPerSymbol map[domain.Symbol]domain.CandleSeries
	err              error
}

func NewFakeCandleRepository(candlesPerSymbol map[domain.Symbol]domain.CandleSeries, err error) *FakeCandleRepository {
	return &FakeCandleRepository{
		candlesPerSymbol: candlesPerSymbol,
		err:              err,
	}
}

func (f *FakeCandleRepository) GetSeries(symbol domain.Symbol, timeframe domain.Timeframe, from, to time.Time) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	if cs, ok := f.candlesPerSymbol[symbol]; ok {
		return cs, nil
	}
	return domain.NewCandleSeries(symbol, timeframe, []domain.Candle{})
}

func (f *FakeCandleRepository) GetLastNCandles(symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	if cs, ok := f.candlesPerSymbol[symbol]; ok {
		// Return last N candles
		if cs.Len() <= n {
			return cs, nil
		}
		// Extract last N
		candles := make([]domain.Candle, 0, n)
		for i := cs.Len() - n; i < cs.Len(); i++ {
			if c, err := cs.At(i); err == nil {
				candles = append(candles, c)
			}
		}
		return domain.NewCandleSeries(symbol, timeframe, candles)
	}
	return domain.NewCandleSeries(symbol, timeframe, []domain.Candle{})
}

var _ ports.CandleRepositoryPort = (*FakeCandleRepository)(nil)

// Test: Returns sparklines for ranked symbols
func TestGetOverviewReturnsSparklines(t *testing.T) {
	// Setup: Create fake candles
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")

	baseTime := time.Unix(int64(1000000), 0).UTC().Truncate(time.Hour)

	btcCandles, _ := domain.NewCandleSeries(btc, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandleAt(btc, domain.NewTimeframeUnsafe("1h"), baseTime.Add(0*time.Hour), 42000.0),
		mustNewCandleAt(btc, domain.NewTimeframeUnsafe("1h"), baseTime.Add(1*time.Hour), 42100.0),
		mustNewCandleAt(btc, domain.NewTimeframeUnsafe("1h"), baseTime.Add(2*time.Hour), 42050.0),
	})

	ethCandles, _ := domain.NewCandleSeries(eth, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandleAt(eth, domain.NewTimeframeUnsafe("1h"), baseTime.Add(0*time.Hour), 2000.0),
		mustNewCandleAt(eth, domain.NewTimeframeUnsafe("1h"), baseTime.Add(1*time.Hour), 2050.0),
		mustNewCandleAt(eth, domain.NewTimeframeUnsafe("1h"), baseTime.Add(2*time.Hour), 2025.0),
	})

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
		eth: ethCandles,
	}, nil)

	ranker := NewFakeRanker([]usecases.RankedSymbol{
		{Symbol: btc, TotalScore: 0.9, Scores: map[string]float64{"score1": 0.5, "score2": 0.4}},
		{Symbol: eth, TotalScore: 0.8, Scores: map[string]float64{"score1": 0.4, "score2": 0.4}},
	}, nil)

	getOverview := usecases.NewGetOverview(ranker, candleRepo, 30, 2)

	// Execute
	req := usecases.GetOverviewRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Limit:     10,
	}
	results, err := getOverview.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Check BTC
	if results[0].Symbol != btc {
		t.Fatalf("expected symbol BTC, got %s", results[0].Symbol.String())
	}
	if len(results[0].Sparkline) != 3 {
		t.Fatalf("expected 3 close prices, got %d", len(results[0].Sparkline))
	}
	if results[0].Sparkline[0] != 42000.0 || results[0].Sparkline[1] != 42100.0 {
		t.Fatalf("unexpected sparkline values: %v", results[0].Sparkline)
	}

	// Check ETH
	if results[1].Symbol != eth {
		t.Fatalf("expected symbol ETH, got %s", results[1].Symbol.String())
	}
	if len(results[1].Sparkline) != 3 {
		t.Fatalf("expected 3 close prices, got %d", len(results[1].Sparkline))
	}
}

// Test: Respects limit
func TestGetOverviewRespectsLimit(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")
	bnb := domain.NewSymbolUnsafe("BNBUSDT")

	btcCandles, _ := domain.NewCandleSeries(btc, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandle(btc, domain.NewTimeframeUnsafe("1h"), 42000.0),
	})
	ethCandles, _ := domain.NewCandleSeries(eth, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandle(eth, domain.NewTimeframeUnsafe("1h"), 2000.0),
	})
	bnbCandles, _ := domain.NewCandleSeries(bnb, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandle(bnb, domain.NewTimeframeUnsafe("1h"), 500.0),
	})

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
		eth: ethCandles,
		bnb: bnbCandles,
	}, nil)

	ranker := NewFakeRanker([]usecases.RankedSymbol{
		{Symbol: btc, TotalScore: 0.9, Scores: map[string]float64{}},
		{Symbol: eth, TotalScore: 0.8, Scores: map[string]float64{}},
		{Symbol: bnb, TotalScore: 0.7, Scores: map[string]float64{}},
	}, nil)

	getOverview := usecases.NewGetOverview(ranker, candleRepo, 30, 2)

	// Execute with limit=2
	req := usecases.GetOverviewRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Limit:     2,
	}
	results, err := getOverview.Execute(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (limit), got %d", len(results))
	}
}

// Test: Skips symbols with no candles
func TestGetOverviewSkipsSymbolsWithNoCandles(t *testing.T) {
	btc := domain.NewSymbolUnsafe("BTCUSDT")
	eth := domain.NewSymbolUnsafe("ETHUSDT")

	btcCandles, _ := domain.NewCandleSeries(btc, domain.NewTimeframeUnsafe("1h"), []domain.Candle{
		mustNewCandle(btc, domain.NewTimeframeUnsafe("1h"), 42000.0),
	})

	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{
		btc: btcCandles,
		// ETH has no candles
	}, nil)

	ranker := NewFakeRanker([]usecases.RankedSymbol{
		{Symbol: btc, TotalScore: 0.9, Scores: map[string]float64{}},
		{Symbol: eth, TotalScore: 0.8, Scores: map[string]float64{}}, // will have empty candles
	}, nil)

	getOverview := usecases.NewGetOverview(ranker, candleRepo, 30, 2)

	// Execute
	req := usecases.GetOverviewRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Limit:     10,
	}
	results, err := getOverview.Execute(context.Background(), req)

	// Assert: Should only have BTC
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (BTC only), got %d", len(results))
	}

	if results[0].Symbol != btc {
		t.Fatalf("expected BTC, got %s", results[0].Symbol.String())
	}
}

// Test: Returns error when ranking fails
func TestGetOverviewErrorOnRankingFailure(t *testing.T) {
	candleRepo := NewFakeCandleRepository(map[domain.Symbol]domain.CandleSeries{}, nil)
	ranker := NewFakeRanker(nil, fmt.Errorf("test ranking error"))

	getOverview := usecases.NewGetOverview(ranker, candleRepo, 30, 2)

	req := usecases.GetOverviewRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Limit:     10,
	}
	_, err := getOverview.Execute(context.Background(), req)

	if err == nil {
		t.Fatalf("expected error from ranking failure, got nil")
	}
}

// Helper to create test candles
func mustNewCandle(symbol domain.Symbol, tf domain.Timeframe, close float64) domain.Candle {
	// Use a properly aligned time: hour boundary (00:00:00 UTC)
	baseTime := time.Unix(int64(1000000), 0).UTC()
	// Truncate to hour boundary
	baseTime = baseTime.Truncate(time.Hour)
	// Offset by close price as hours to create different timestamps
	ts := baseTime.Add(time.Duration(int(close)) * time.Hour)
	c, err := domain.NewCandle(symbol, tf, ts, close-10, close+10, close-20, close, 1000)
	if err != nil {
		panic(err)
	}
	return c
}

// Helper to create test candles at specific time
func mustNewCandleAt(symbol domain.Symbol, tf domain.Timeframe, ts time.Time, close float64) domain.Candle {
	c, err := domain.NewCandle(symbol, tf, ts, close-10, close+10, close-20, close, 1000)
	if err != nil {
		panic(err)
	}
	return c
}
