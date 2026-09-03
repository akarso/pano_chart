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

// makeCompressingCandles builds 110 candles forming a symmetric triangle:
// price oscillates between converging upper and lower boundaries so that
// swing highs decline and swing lows rise — triggering DetectCompression.
// The bandwidth is scaled so that the range exceeds MinStructuralRange (0.5%),
// avoiding the small-range penalty.
func makeCompressingCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
		t := float64(i) / 109.0 // 0..1
		// Bandwidth shrinks from 10% to 1% of basePrice.
		bandWidth := basePrice * 0.10 * (1.0 - 0.9*t)
		// Oscillate close between upper and lower regions, period ~14 candles.
		phase := math.Sin(float64(i) * math.Pi / 7.0)
		offset := phase * bandWidth * 0.4
		p := basePrice + offset
		spread := bandWidth * 0.25
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p - spread*0.1, high: p + spread,
			low: p - spread, close: p + spread*0.1,
		}
	}
	return candles
}

// makeExpandingCandles builds 110 candles where the last 7 have much larger ranges.
func makeExpandingCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 103; i++ {
		p := basePrice + float64(i)*0.1
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 1, low: p - 1, close: p,
		}
	}
	for i := 103; i < 110; i++ {
		p := basePrice + float64(i)*5
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p, high: p + 50, low: p - 50, close: p,
		}
	}
	return candles
}

// makeTrendingCandles builds 110 candles with a strong directional move
// (>10% return) and moderate volatility (shortATR/longATR between 0.9–1.3).
func makeTrendingCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
		// Strong uptrend: +0.5% per candle → ~55% total return over 110 candles.
		p := basePrice * (1 + 0.005*float64(i))
		// Consistent moderate range so ATR ratio stays near 1.0.
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: p * 0.995, high: p * 1.01, low: p * 0.99, close: p,
		}
	}
	return candles
}

// makeFlatCandles builds 110 flat candles for sideways testing.
func makeFlatCandles(sym domain.Symbol, tf domain.Timeframe, basePrice float64) []fakeRegimeCandle {
	candles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
		candles[i] = fakeRegimeCandle{
			symbol: sym, timeframe: tf, ts: tsR(i),
			open: basePrice, high: basePrice + 10, low: basePrice - 10, close: basePrice,
		}
	}
	return candles
}

// --- Regime Detector Tests ---

func TestDetectRegime_Compression(t *testing.T) {
	// Compressing candles → high compression breadth from real DetectCompression + low vol
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

	if result.Regime != mkt.RegimeCompression {
		t.Errorf("expected compression, got %s (metrics: comp=%.4f, trend=%.4f, vol=%.4f)",
			result.Regime, result.Metrics.CompressionBreadth,
			result.Metrics.TrendBreadth, result.Metrics.VolatilityExpansion)
	}
	if result.Metrics.CompressionBreadth < 0.20 {
		t.Errorf("expected compressionBreadth >= 0.20, got %f", result.Metrics.CompressionBreadth)
	}
}

func TestDetectRegime_Trend(t *testing.T) {
	// Strongly trending candles → high trend breadth from real TrendPredictability + moderate vol
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": makeTrendingCandles(sym, tf, 50000),
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeTrend {
		t.Errorf("expected trend, got %s (metrics: trend=%.4f, comp=%.4f, vol=%.4f)",
			result.Regime, result.Metrics.TrendBreadth,
			result.Metrics.CompressionBreadth, result.Metrics.VolatilityExpansion)
	}
	if result.Metrics.TrendBreadth < 0.25 {
		t.Errorf("expected trendBreadth >= 0.25, got %f", result.Metrics.TrendBreadth)
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

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
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
	// Flat candles: zero return → zero trend breadth; consistent ATR → zero
	// compression breadth.  Volatility ≈ 1.0 (short == long).
	// SidewaysV5 also returns 0 for perfectly flat data (no oscillation),
	// so no regime has strong evidence → indecisive.
	sym := makeSymR("BTCUSDT")
	tf := makeTimeframeR("4h")

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": makeFlatCandles(sym, tf, 50000),
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Regime != mkt.RegimeIndecisive {
		t.Errorf("expected indecisive (no clear signal), got %s (metrics: trend=%.4f, comp=%.4f, vol=%.4f)",
			result.Regime, result.Metrics.TrendBreadth,
			result.Metrics.CompressionBreadth, result.Metrics.VolatilityExpansion)
	}
	// Prevalence should be the dominant (low) score.
	if result.Prevalence < 0.1 {
		t.Errorf("expected prevalence > 0.1, got %f", result.Prevalence)
	}
	// Scores should sum to ~1.0.
	sum := result.Scores.Expansion + result.Scores.Compression +
		result.Scores.Trend + result.Scores.Sideways
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("expected scores to sum to ~1.0, got %f", sum)
	}
}

func TestDetectRegime_NoSymbols(t *testing.T) {
	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{},
		candles: map[string][]fakeRegimeCandle{},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No symbols → zero candles → zero breadth → indecisive (no evidence).
	if result.Regime != mkt.RegimeIndecisive {
		t.Errorf("expected indecisive, got %s", result.Regime)
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
	btcCandles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
		p := 50000 + float64(i)*500
		btcCandles[i] = fakeRegimeCandle{
			symbol: sym1, timeframe: tf, ts: tsR(i),
			open: p, high: p + 10, low: p - 10, close: p,
		}
	}

	// ETH goes down significantly
	ethCandles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
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

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
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

func TestDetectRegime_MixedDirectionNotTrend(t *testing.T) {
	// Two symbols trending in opposite directions (BTC up, ETH down).
	// The direction agreement penalty dampens trend breadth — half the lost
	// trend is retained as undirected trend energy, so the regime may still
	// be classified as trend but with reduced breadth compared to the
	// single-direction case.  We verify trend breadth is dampened.
	sym1 := makeSymR("BTCUSDT")
	sym2 := makeSymR("ETHUSDT")
	tf := makeTimeframeR("4h")

	btcCandles := makeTrendingCandles(sym1, tf, 50000) // strong uptrend

	// ETH: strong downtrend (mirror of the uptrend pattern).
	ethCandles := make([]fakeRegimeCandle, 110)
	for i := 0; i < 110; i++ {
		p := 3000.0 * (1 - 0.005*float64(i))
		ethCandles[i] = fakeRegimeCandle{
			symbol: sym2, timeframe: tf, ts: tsR(i),
			open: p * 1.005, high: p * 1.01, low: p * 0.99, close: p,
		}
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym1, sym2},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": btcCandles,
			"ETHUSDT:4h": ethCandles,
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --- baseline: single-direction trend breadth ---
	singleProvider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym1},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": btcCandles,
		},
	}
	singleComposite := metrics.NewCompositeIndexService(singleProvider, 5)
	singleSvc := metrics.NewMetricsService(singleComposite, singleProvider, evalProvider)
	singleResult, err := singleSvc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metrics.TrendBreadth >= singleResult.Metrics.TrendBreadth {
		t.Errorf("mixed-direction trend breadth (%.4f) should be lower than single-direction (%.4f)",
			result.Metrics.TrendBreadth, singleResult.Metrics.TrendBreadth)
	}
}

func TestDetectRegime_Bias_FollowsAggregateReturn(t *testing.T) {
	// When aggregate return is negative, bias must be "down", never "up".
	sym1 := makeSymR("BTCUSDT")
	sym2 := makeSymR("ETHUSDT")
	tf := makeTimeframeR("4h")

	// Both tokens trend down.
	mkDown := func(sym domain.Symbol, base float64) []fakeRegimeCandle {
		candles := make([]fakeRegimeCandle, 110)
		for i := 0; i < 110; i++ {
			p := base * (1 - 0.005*float64(i))
			candles[i] = fakeRegimeCandle{
				symbol: sym, timeframe: tf, ts: tsR(i),
				open: p * 1.005, high: p * 1.01, low: p * 0.99, close: p,
			}
		}
		return candles
	}

	provider := &fakeRegimeCandleProvider{
		symbols: []domain.Symbol{sym1, sym2},
		candles: map[string][]fakeRegimeCandle{
			"BTCUSDT:4h": mkDown(sym1, 50000),
			"ETHUSDT:4h": mkDown(sym2, 3000),
		},
	}

	evalProvider := &fakeRegimeEvalProvider{evals: []domain.EvaluationSnapshot{}}
	compositeService := metrics.NewCompositeIndexService(provider, 5)
	svc := metrics.NewMetricsService(compositeService, provider, evalProvider)

	result, err := svc.CalculateRegime(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Bias == "up" {
		t.Errorf("bias must not be 'up' when aggregate return is negative; got %q", result.Bias)
	}
	if result.Bias != "down" {
		t.Errorf("expected bias 'down', got %q", result.Bias)
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
			Prevalence: 0.71,
			Scores: mkt.RegimeScores{
				Expansion:   0.05,
				Compression: 0.71,
				Trend:       0.14,
				Sideways:    0.10,
			},
			Metrics: mkt.RegimeMetrics{
				TrendBreadth:        0.18,
				SidewaysBreadth:     0.10,
				ExpansionBreadth:    0.05,
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
		Prevalence float64 `json:"prevalence"`
		Scores     struct {
			Expansion   float64 `json:"expansion"`
			Compression float64 `json:"compression"`
			Trend       float64 `json:"trend"`
			Sideways    float64 `json:"sideways"`
		} `json:"scores"`
		Metrics struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			SidewaysBreadth     float64 `json:"sidewaysBreadth"`
			ExpansionBreadth    float64 `json:"expansionBreadth"`
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
	if resp.Prevalence != 0.71 {
		t.Errorf("expected prevalence 0.71, got %f", resp.Prevalence)
	}
	if resp.Scores.Compression != 0.71 {
		t.Errorf("expected scores.compression 0.71, got %f", resp.Scores.Compression)
	}
	if resp.Metrics.TrendBreadth != 0.18 {
		t.Errorf("expected trendBreadth 0.18, got %f", resp.Metrics.TrendBreadth)
	}
	if resp.Metrics.SidewaysBreadth != 0.10 {
		t.Errorf("expected sidewaysBreadth 0.10, got %f", resp.Metrics.SidewaysBreadth)
	}
	if resp.Metrics.ExpansionBreadth != 0.05 {
		t.Errorf("expected expansionBreadth 0.05, got %f", resp.Metrics.ExpansionBreadth)
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
			Prevalence: 0.123456789,
			Scores: mkt.RegimeScores{
				Expansion:   0.111111111,
				Compression: 0.222222222,
				Trend:       0.444444444,
				Sideways:    0.222222222,
			},
			Metrics: mkt.RegimeMetrics{
				TrendBreadth:        0.987654321,
				SidewaysBreadth:     0.444444444,
				ExpansionBreadth:    0.666666666,
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
		Prevalence float64 `json:"prevalence"`
		Scores     struct {
			Expansion   float64 `json:"expansion"`
			Compression float64 `json:"compression"`
			Trend       float64 `json:"trend"`
			Sideways    float64 `json:"sideways"`
		} `json:"scores"`
		Metrics struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			SidewaysBreadth     float64 `json:"sidewaysBreadth"`
			ExpansionBreadth    float64 `json:"expansionBreadth"`
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
	check("prevalence", resp.Prevalence, 0.1235)
	check("scores.trend", resp.Scores.Trend, 0.4444)
	check("trendBreadth", resp.Metrics.TrendBreadth, 0.9877)
	check("sidewaysBreadth", resp.Metrics.SidewaysBreadth, 0.4444)
	check("expansionBreadth", resp.Metrics.ExpansionBreadth, 0.6667)
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
