package market_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake implementations ---

type fakeRegimeCandle struct {
	symbol    domain.Symbol
	timeframe domain.Timeframe
	ts        time.Time
	open      float64
	high      float64
	low       float64
	close     float64
}

type fakeRegimeCandleProvider struct {
	symbols []domain.Symbol
	candles map[string][]fakeRegimeCandle
	err     error
}

func (f *fakeRegimeCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}

func (f *fakeRegimeCandleProvider) GetLastNCandles(sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	fcs, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("no data for %s", key)
	}
	candles := make([]domain.Candle, 0, len(fcs))
	for _, fc := range fcs {
		c := domain.NewCandleUnsafe(fc.symbol, fc.timeframe, fc.ts, fc.open, fc.high, fc.low, fc.close, 1000)
		candles = append(candles, c)
	}
	if n < len(candles) {
		candles = candles[len(candles)-n:]
	}
	return domain.NewCandleSeries(sym, tf, candles)
}

type fakeRegimeEvalProvider struct {
	evals []domain.EvaluationSnapshot
	err   error
}

func (f *fakeRegimeEvalProvider) GetLatestEvaluations(_ string) ([]domain.EvaluationSnapshot, error) {
	return f.evals, f.err
}

func makeSymR(s string) domain.Symbol {
	sym, _ := domain.NewSymbol(s)
	return sym
}

func makeTimeframeR(s string) domain.Timeframe {
	tf, _ := domain.NewTimeframe(s)
	return tf
}

func tsR(idx int) time.Time {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(idx) * 4 * time.Hour)
}

// makeCompressingCandles builds 30 candles where initial candles have wider ranges
// and the last 7 have very tight ranges, resulting in shortATR/longATR < 0.9.
func makeCompressingCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 23; i++ {
		p := basePrice + float64(i)*0.1
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 20, low: p - 20, close: p,
		}
	}
	// Last 7 candles: very tight range
	for i := 23; i < 30; i++ {
		p := basePrice + float64(i)*0.01
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 1, low: p - 1, close: p,
		}
	}
	return candles
}

// makeExpandingCandles builds 30 candles where the last 7 have much larger ranges.
func makeExpandingCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 23; i++ {
		p := basePrice + float64(i)*0.1
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 1, low: p - 1, close: p,
		}
	}
	for i := 23; i < 30; i++ {
		p := basePrice + float64(i)*5
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 50, low: p - 50, close: p,
		}
	}
	return candles
}

// --- Regime Detector Tests ---

func TestDetectRegime_Compression(t *testing.T) {
	// High compression breadth + low volatility → compression
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": makeCompressingCandles(sym, tf, 50000),
		},
	}

	// 40% compression breadth
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := 0; i < 4; i++ {
		evals[i] = domain.EvaluationSnapshot{CompressionScore: 0.8}
	}
	for i := 4; i < 10; i++ {
		evals[i] = domain.EvaluationSnapshot{SidewaysScore: 0.9}
	}

	evalProvider := &fakeRegimeEvalProvider{evals: evals}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeCompression {
		t.Errorf("expected compression, got %s", result.Regime)
	}
	if result.Metrics.CompressionBreadth < 0.30 {
		t.Errorf("expected compressionBreadth >= 0.30, got %f", result.Metrics.CompressionBreadth)
	}
}

func TestDetectRegime_Trend(t *testing.T) {
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	// Candles where recent volatility is moderately higher than long-term
	// so shortATR/longATR > 1.0 but not > 1.3 (which would be expansion).
	candles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 23; i++ {
		p := 50000 + float64(i)*10
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 20, low: p - 20, close: p,
		}
	}
	// Last 7: moderately wider ranges (3x the early ones)
	for i := 23; i < 30; i++ {
		p := 50000 + float64(i)*10
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 60, low: p - 60, close: p,
		}
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": candles,
		},
	}

	// 50% trend breadth
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := 0; i < 5; i++ {
		evals[i] = domain.EvaluationSnapshot{TrendScore: 0.8}
	}
	for i := 5; i < 10; i++ {
		evals[i] = domain.EvaluationSnapshot{SidewaysScore: 0.9}
	}

	evalProvider := &fakeRegimeEvalProvider{evals: evals}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeTrend {
		t.Errorf("expected trend, got %s", result.Regime)
	}
	if result.Metrics.TrendBreadth < 0.40 {
		t.Errorf("expected trendBreadth >= 0.40, got %f", result.Metrics.TrendBreadth)
	}
}

func TestDetectRegime_Expansion(t *testing.T) {
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": makeExpandingCandles(sym, tf, 50000),
		},
	}

	// Low breadth → no trend or compression
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := range evals {
		evals[i] = domain.EvaluationSnapshot{SidewaysScore: 0.9}
	}

	evalProvider := &fakeRegimeEvalProvider{evals: evals}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeExpansion {
		t.Errorf("expected expansion, got %s", result.Regime)
	}
	if result.Metrics.VolatilityExpansion <= 1.3 {
		t.Errorf("expected volatilityExpansion > 1.3, got %f", result.Metrics.VolatilityExpansion)
	}
}

func TestDetectRegime_Sideways(t *testing.T) {
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	// Moderate candles — not expanding, not compressed
	candles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 30; i++ {
		p := 50000.0
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 10, low: p - 10, close: p,
		}
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": candles,
		},
	}

	evals := make([]domain.EvaluationSnapshot, 10)
	for i := range evals {
		evals[i] = domain.EvaluationSnapshot{SidewaysScore: 0.9}
	}

	evalProvider := &fakeRegimeEvalProvider{evals: evals}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeSideways {
		t.Errorf("expected sideways, got %s", result.Regime)
	}
	if result.Confidence != 0.5 {
		t.Errorf("expected confidence 0.5, got %f", result.Confidence)
	}
}

func TestDetectRegime_EmptyEvaluations(t *testing.T) {
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": makeCompressingCandles(sym, tf, 50000),
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No evaluations → zero breadth → sideways
	if result.Regime != mkt.RegimeSideways {
		t.Errorf("expected sideways, got %s", result.Regime)
	}
}

func TestDetectRegime_InvalidTimeframe(t *testing.T) {
	provider := &fakeRegimeCandleProvider{}
	evalProvider := &fakeRegimeEvalProvider{}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	_, err := svc.CalculateRegime(context.Background(), "invalid")
	if err == nil {
		t.Error("expected error for invalid timeframe")
	}
}

func TestDetectRegime_Dispersion(t *testing.T) {
	// Two symbols with divergent returns → higher dispersion
	sym1 := makeSymR("BTCUSDT")
	sym2 := makeSymR("ETHUSDT")
	tf := makeTimeframeR("4h")

	// BTC goes up significantly
	btcCandles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 30; i++ {
		p := 50000 + float64(i)*500
		btcCandles[i] = fakeRegimeCandle{
			symbol: sym1, timeframe: tf, ts: tsR(i),
			open: p, high: p + 10, low: p - 10, close: p,
		}
	}

	// ETH goes down significantly
	ethCandles := make([]fakeRegimeCandle, 30)
	for i := 0; i < 30; i++ {
		p := 3000 - float64(i)*50
		ethCandles[i] = fakeRegimeCandle{
			symbol: sym2, timeframe: tf, ts: tsR(i),
			open: p, high: p + 10, low: p - 10, close: p,
		}
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym1, sym2},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": btcCandles,
			"ETHUSDT:4h": ethCandles,
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{
		{SidewaysScore: 0.9},
		{SidewaysScore: 0.9},
	}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metrics.Dispersion <= 0 {
		t.Errorf("expected positive dispersion for divergent assets, got %f", result.Metrics.Dispersion)
	}
}

// --- Handler Tests ---

type fakeRegimeCalc struct {
	summary mkt.RegimeSummary
	err     error
}

func (f *fakeRegimeCalc) CalculateRegime(_ context.Context, _ string) (mkt.RegimeSummary, error) {
	return f.summary, f.err
}

func TestRegimeHandler_DefaultParams(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.RegimeSummary{
			Timeframe:  "4h",
			Regime:     mkt.RegimeCompression,
			Confidence: 0.71,
			Metrics: mkt.RegimeMetrics{
				TrendBreadth:        0.18,
				CompressionBreadth:  0.34,
				VolatilityExpansion: 0.82,
				Dispersion:          0.21,
			},
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Timeframe  string  `json:"timeframe"`
		Regime     string  `json:"regime"`
		Confidence float64 `json:"confidence"`
		Metrics    struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			CompressionBreadth  float64 `json:"compressionBreadth"`
			VolatilityExpansion float64 `json:"volatilityExpansion"`
			Dispersion          float64 `json:"dispersion"`
		} `json:"metrics"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", resp.Timeframe)
	}
	if resp.Regime != "compression" {
		t.Errorf("expected regime compression, got %s", resp.Regime)
	}
	if resp.Confidence != 0.71 {
		t.Errorf("expected confidence 0.71, got %f", resp.Confidence)
	}
	if resp.Metrics.TrendBreadth != 0.18 {
		t.Errorf("expected trendBreadth 0.18, got %f", resp.Metrics.TrendBreadth)
	}
	if resp.Metrics.CompressionBreadth != 0.34 {
		t.Errorf("expected compressionBreadth 0.34, got %f", resp.Metrics.CompressionBreadth)
	}
	if resp.Metrics.VolatilityExpansion != 0.82 {
		t.Errorf("expected volatilityExpansion 0.82, got %f", resp.Metrics.VolatilityExpansion)
	}
	if resp.Metrics.Dispersion != 0.21 {
		t.Errorf("expected dispersion 0.21, got %f", resp.Metrics.Dispersion)
	}
}

func TestRegimeHandler_CustomTimeframe(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.RegimeSummary{
			Timeframe: "1h",
			Regime:    mkt.RegimeSideways,
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Timeframe string `json:"timeframe"`
		Regime    string `json:"regime"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", resp.Timeframe)
	}
}

func TestRegimeHandler_Error(t *testing.T) {
	calc := &fakeRegimeCalc{err: fmt.Errorf("something went wrong")}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestRegimeHandler_RoundsValues(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.RegimeSummary{
			Timeframe:  "4h",
			Regime:     mkt.RegimeTrend,
			Confidence: 0.123456789,
			Metrics: mkt.RegimeMetrics{
				TrendBreadth:        0.987654321,
				CompressionBreadth:  0.111111111,
				VolatilityExpansion: 1.555555555,
				Dispersion:          0.333333333,
			},
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Confidence float64 `json:"confidence"`
		Metrics    struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			CompressionBreadth  float64 `json:"compressionBreadth"`
			VolatilityExpansion float64 `json:"volatilityExpansion"`
			Dispersion          float64 `json:"dispersion"`
		} `json:"metrics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// All values rounded to 4 decimal places
	check := func(name string, got, want float64) {
		if math.Abs(got-want) > 0.00001 {
			t.Errorf("%s: expected %f, got %f", name, want, got)
		}
	}
	check("confidence", resp.Confidence, 0.1235)
	check("trendBreadth", resp.Metrics.TrendBreadth, 0.9877)
	check("compressionBreadth", resp.Metrics.CompressionBreadth, 0.1111)
	check("volatilityExpansion", resp.Metrics.VolatilityExpansion, 1.5556)
	check("dispersion", resp.Metrics.Dispersion, 0.3333)
}

// --- Unit tests for individual metric functions ---
// These test the pure functions via the MetricsService integration.

func TestMetrics_VolatilityExpansion_InsufficientData(t *testing.T) {
	// < 30 candles → volatility defaults to 1.0
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	candles := make([]fakeRegimeCandle, 5)
	for i := 0; i < 5; i++ {
		p := 50000.0
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 10, low: p - 10, close: p,
		}
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{"BTCUSDT:4h": candles},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With < 30 candles, volatility defaults to 1.0
	if result.Metrics.VolatilityExpansion != 1.0 {
		t.Errorf("expected volatilityExpansion 1.0 for insufficient data, got %f", result.Metrics.VolatilityExpansion)
	}
}
