package setups_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/setups"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	domainrisk "pano_chart/backend/domain/risk"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/domain/setup"
	domainsignal "pano_chart/backend/domain/signal"
)

// --- Fakes ---

type fakeCandleRepo struct {
	series domain.CandleSeries
	err    error
}

func (f *fakeCandleRepo) GetSeries(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _, _ time.Time) (domain.CandleSeries, error) {
	return f.series, f.err
}

func (f *fakeCandleRepo) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

// fakeFragilityProvider lets a test observe the ctx it's given and react —
// e.g. simulate the caller cancelling mid-lookup — by running onGet before
// returning the configured result.
type fakeFragilityProvider struct {
	frag  domainrisk.Fragility
	err   error
	onGet func()
}

func (f *fakeFragilityProvider) Get(_ context.Context, _, _ string) (domainrisk.Fragility, error) {
	if f.onGet != nil {
		f.onGet()
	}
	if f.err != nil {
		return domainrisk.Fragility{}, f.err
	}
	return f.frag, nil
}

// fakeSeasonalityProvider lets a test control CurrentSpikeProbability's
// result/error — see the SetSeasonalityProvider tests below (PR-082).
type fakeSeasonalityProvider struct {
	spikeProb float64
	err       error
}

func (f *fakeSeasonalityProvider) CurrentSpikeProbability(_ context.Context, _ string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.spikeProb, nil
}

type fakeScorer struct {
	stats usecases.SymbolStats
	err   error
}

func (f *fakeScorer) Score(_ domain.CandleSeries) (usecases.SymbolStats, error) {
	if f.err != nil {
		return usecases.SymbolStats{}, f.err
	}
	return f.stats, nil
}

// --- Helpers ---

func makeSeries(n int) domain.CandleSeries {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("4h")
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			100, 110, 90, 105, float64(1000+i*10),
		)
	}
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

// makeDirectionalSeries builds a monotonically rising or falling close-price
// series, for tests that need dominantRegime to see a real direction rather
// than makeSeries' flat 105-close candles.
func makeDirectionalSeries(n int, rising bool) domain.CandleSeries {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("4h")
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		price := 100.0 + float64(i)
		if !rising {
			price = 100.0 + float64(n-i)
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			price, price+2, price-2, price, 1000,
		)
	}
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

// makeSeriesWithTimeframeAndRange builds a flat-close series (no directional
// bias, so dominantRegime falls back to "sideways" — see
// TestDominantRegime_DirectionMatchesPriceAction's identical flat-series
// case) at the given timeframe, with every candle's (high-low)/close ratio
// set to exactly rangeFrac — used to drive volatilityFromSeries with a known
// input for the PR-080 regression tests below.
func makeSeriesWithTimeframeAndRange(t *testing.T, n int, tfStr string, rangeFrac float64) domain.CandleSeries {
	t.Helper()
	sym, err := domain.NewSymbol("BTCUSDT")
	if err != nil {
		t.Fatalf("NewSymbol: %v", err)
	}
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		t.Fatalf("NewTimeframe(%q): %v", tfStr, err)
	}
	const close = 100.0
	half := close * rangeFrac / 2
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*tf.Duration()),
			close, close+half, close-half, close, 1000,
		)
	}
	s, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("NewCandleSeries: %v", err)
	}
	return s
}

// sidewaysFallbackStats returns SymbolStats scored so dominantRegime falls
// back to "sideways" for a flat-close series (Compression/Trend both low,
// and the flat series' recomputed trend bias is "neutral" — see
// dominantRegime's flat-series fallback path).
func sidewaysFallbackStats() usecases.SymbolStats {
	return usecases.SymbolStats{
		Scores: map[string]float64{
			"Compression":          0.1,
			"Trend Predictability": 0.1,
		},
	}
}

// --- Tests ---

func TestSetupService_HappyPath(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 2.5,
		Scores: map[string]float64{
			"Compression":          0.8,
			"Trend Predictability": 0.6,
			"Sideways":             0.3,
		},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", result.Symbol)
	}
	if result.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", result.Timeframe)
	}
	if len(result.Scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(result.Scores))
	}
	if result.BestSetup == "" {
		t.Error("expected a non-empty best setup")
	}
	if result.Score <= 0 {
		t.Errorf("expected positive best score, got %f", result.Score)
	}
}

// trendDominantStatsFor builds fake SymbolStats whose "Trend Predictability"
// entry matches what the real calculator computes for series — dominantRegime
// now cross-checks the two (scoresAgree) and falls back to "sideways" if a
// fake fixture's magnitude doesn't match the recomputed one, so tests must
// use a real, series-consistent score rather than an arbitrary constant.
func trendDominantStatsFor(t *testing.T, series domain.CandleSeries) usecases.SymbolStats {
	t.Helper()
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	score, _, err := calc.ScoreWithDirection(series)
	if err != nil {
		t.Fatalf("unexpected error scoring fixture series: %v", err)
	}
	return usecases.SymbolStats{
		Scores: map[string]float64{
			"Compression":          0.1,
			"Trend Predictability": score,
			"Sideways":             0.1,
		},
	}
}

func TestDominantRegime_DirectionMatchesPriceAction(t *testing.T) {
	// Regression test for the PR-072 bug: dominantRegime used to guess
	// "uptrend" whenever the trend score exceeded 0.5, regardless of
	// whether the price actually rose or fell.

	t.Run("rising series classifies as uptrend", func(t *testing.T) {
		series := makeDirectionalSeries(50, true)
		repo := &fakeCandleRepo{series: series}
		scorer := &fakeScorer{stats: trendDominantStatsFor(t, series)}
		svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

		result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Regime != "uptrend" {
			t.Errorf("expected uptrend for a rising series, got %q", result.Regime)
		}
	})

	t.Run("falling series classifies as downtrend, not uptrend", func(t *testing.T) {
		series := makeDirectionalSeries(50, false)
		repo := &fakeCandleRepo{series: series}
		scorer := &fakeScorer{stats: trendDominantStatsFor(t, series)}
		svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

		result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Regime != "downtrend" {
			t.Errorf("expected downtrend for a falling series, got %q", result.Regime)
		}
	})

	t.Run("flat series has no direction to claim, falls back to sideways", func(t *testing.T) {
		// makeSeries' candles all close at 105 — the real calculator scores
		// this as 0 (flat line), so the fake stats naturally agree with it;
		// there's no real price direction to report.
		series := makeSeries(50)
		repo := &fakeCandleRepo{series: series}
		scorer := &fakeScorer{stats: trendDominantStatsFor(t, series)}
		svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

		result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Regime == "uptrend" {
			t.Errorf("expected no uptrend claim for a flat/undirected series, got %q", result.Regime)
		}
	})
}

func TestDominantRegime_ScoreMismatchFallsBackToSideways(t *testing.T) {
	// Regression test for the scoresAgree safety net: if the fake/stale
	// "Trend Predictability" score doesn't match what ScoreWithDirection
	// actually recomputes for the series, dominantRegime must not trust the
	// recomputed bias and should fall back to "sideways" instead of
	// reporting a possibly-mismatched direction.
	series := makeDirectionalSeries(50, true) // real recomputed score saturates at 1.0, not 0.42
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		Scores: map[string]float64{
			"Compression":          0.1,
			"Trend Predictability": 0.42, // deliberately mismatched vs. the real recomputed score
			"Sideways":             0.1,
		},
	}}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Regime != "sideways" {
		t.Errorf("expected sideways on score mismatch, got %q", result.Regime)
	}
}

func TestSetupService_InvalidSymbol(t *testing.T) {
	eng := setups.NewEngine()
	svc := setups.NewSetupService(&fakeCandleRepo{}, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for empty symbol")
	}
}

func TestSetupService_InvalidTimeframe(t *testing.T) {
	eng := setups.NewEngine()
	svc := setups.NewSetupService(&fakeCandleRepo{}, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestSetupService_CandleFetchError(t *testing.T) {
	repo := &fakeCandleRepo{err: errors.New("network failure")}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when candle fetch fails")
	}
}

func TestSetupService_ScorerError(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{err: errors.New("scoring failed")}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when scorer fails")
	}
}

func TestSetupService_EmptySeriesReturnsZeroScores(t *testing.T) {
	series := makeSeries(1) // Less than 2 candles
	repo := &fakeCandleRepo{series: series}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, &fakeScorer{}, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Scores) != 0 {
		t.Errorf("expected empty scores for short series, got %d", len(result.Scores))
	}
	if result.BestSetup != "" {
		t.Errorf("expected empty best setup, got %s", result.BestSetup)
	}
}

func TestSetupService_HighCompressionSelectsCompressionBreakout(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 1.0,
		Scores: map[string]float64{
			"Compression":          0.95,
			"Trend Predictability": 0.1,
		},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BestSetup != setup.CompressionBreakout {
		t.Errorf("expected compression_breakout, got %s", result.BestSetup)
	}
}

// TestSetupService_FragilityCancellation_AbortsInsteadOfDefaultingCrowding
// is the regression test for the CR finding that a caller-cancelled ctx
// during the fragility lookup was silently swallowed, leaving Crowding at
// its zero default (the maximum-confidence value) and returning a
// successful, confidently-computed result built on data that was never
// actually obtained. Evaluate must propagate ctx.Err() instead.
func TestSetupService_FragilityCancellation_AbortsInsteadOfDefaultingCrowding(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 1.0,
		Scores:     map[string]float64{"Compression": 0.5, "Trend Predictability": 0.5},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	ctx, cancel := context.WithCancel(context.Background())
	// onGet simulates the caller giving up while the fragility lookup is
	// in flight: the provider fails, and by the time Evaluate checks
	// ctx.Err(), the parent context is already done.
	svc.SetFragilityProvider(&fakeFragilityProvider{
		err:   errors.New("lookup interrupted"),
		onGet: cancel,
	})

	result, err := svc.Evaluate(ctx, "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when ctx is cancelled during fragility lookup")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error wrapping context.Canceled, got %v", err)
	}
	if result.Symbol != "" || result.Scores != nil {
		t.Errorf("expected zero-value result on cancellation, got %+v", result)
	}
}

// TestSetupService_FragilityError_DegradesGracefullyWhenNotCancelled is the
// companion case: a fragility-provider failure unrelated to cancellation
// must still degrade gracefully (Crowding defaults to 0, Evaluate
// succeeds) — the fix must not turn every fragility error into a hard
// failure, only ones caused by the caller's own context being done.
func TestSetupService_FragilityError_DegradesGracefullyWhenNotCancelled(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 1.0,
		Scores:     map[string]float64{"Compression": 0.5, "Trend Predictability": 0.5},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)
	svc.SetFragilityProvider(&fakeFragilityProvider{err: errors.New("provider down")})

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Crowding != 0 {
		t.Errorf("expected Crowding to default to 0 on a non-cancellation error, got %f", result.Crowding)
	}
}

// TestVolatilityFromSeries_PR080_FifteenMinuteRangeNoLongerPinnedNearFloor is
// the regression test for PR-080: volatilityFromSeries used to normalize
// every timeframe against a divisor (0.1) calibrated for 1d candles. A
// realistic, genuinely-tradeable 15m average range (~0.5% of price) used to
// map to ~0.05 — nowhere near VolatilityFit's 0.5 "ideal" input for the
// sideways regime — silently capping VolatilityFit near its floor (~0.1)
// regardless of how good the actual conditions were. With the fix, the same
// 0.5% range should land close to the sideways regime's ideal point.
func TestVolatilityFromSeries_PR080_FifteenMinuteRangeNoLongerPinnedNearFloor(t *testing.T) {
	const typical15mRange = 0.005 // 0.5% average (high-low)/close
	series := makeSeriesWithTimeframeAndRange(t, 50, "15m", typical15mRange)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: sidewaysFallbackStats()}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Regime != "sideways" {
		t.Fatalf("test fixture must classify as sideways to exercise the sideways VolatilityFit curve, got %q", result.Regime)
	}

	// Pre-fix, the same input produced VolatilityFit ≈ 0.1 (clamp(1-2*|0.05-0.5|)).
	// Post-fix it should land close to the ideal 0.5 input, i.e. a high fit.
	if result.VolatilityFit < 0.8 {
		t.Errorf("expected VolatilityFit >= 0.8 for a typical 15m range post-fix, got %f (pre-fix this would have been ≈0.1)", result.VolatilityFit)
	}
}

// TestVolatilityFromSeries_PR080_DailyBehaviorUnchanged pins the 1d divisor
// at exactly the pre-fix value (0.1) — the fix must not change scoring for
// the timeframe the original constant was actually calibrated for.
func TestVolatilityFromSeries_PR080_DailyBehaviorUnchanged(t *testing.T) {
	const midScaleDailyRange = 0.05 // the original comment's own "0.05→0.5" reference point
	series := makeSeriesWithTimeframeAndRange(t, 50, "1d", midScaleDailyRange)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: sidewaysFallbackStats()}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "1d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Regime != "sideways" {
		t.Fatalf("test fixture must classify as sideways to exercise the sideways VolatilityFit curve, got %q", result.Regime)
	}

	// 0.05 / 0.1 == 0.5 == the sideways regime's ideal input -> VolatilityFit == 1.0.
	if result.VolatilityFit < 0.99 {
		t.Errorf("expected VolatilityFit ≈ 1.0 for the daily divisor's own reference point, got %f", result.VolatilityFit)
	}
}

// TestVolatilityFromSeries_PR080_AllTimeframesScaleBySqrtOfTime is the CR
// follow-up for PR-080: the two tests above only exercised 15m and 1d,
// leaving 1m/5m/1h/4h uncovered — a sign or exponent slip in
// volatilityDivisorForTimeframe for any of those would have gone
// undetected. This computes each timeframe's expected divisor
// independently (dailyDivisor * sqrt(minutes/1440), duplicating the
// production formula deliberately — volatilityDivisorForTimeframe is
// unexported, so this is the only way to pin it from the external
// setups_test package) and feeds a range set to exactly that divisor's
// ideal midpoint, so every timeframe must land at VolatilityFit ≈ 1.0 for
// the fix to be correct across the board, not just at the two points
// already tested above.
func TestVolatilityFromSeries_PR080_AllTimeframesScaleBySqrtOfTime(t *testing.T) {
	const dailyDivisor = 0.1 // must match dailyVolatilityDivisor in service.go
	dailyMinutes := 24.0 * 60.0

	for _, tfStr := range []string{"1m", "5m", "15m", "1h", "4h", "1d"} {
		t.Run(tfStr, func(t *testing.T) {
			tf, err := domain.NewTimeframe(tfStr)
			if err != nil {
				t.Fatalf("NewTimeframe(%q): %v", tfStr, err)
			}
			expectedDivisor := dailyDivisor * math.Sqrt(tf.Duration().Minutes()/dailyMinutes)
			rangeFrac := expectedDivisor * 0.5 // the sideways regime's ideal VolatilityFit input

			series := makeSeriesWithTimeframeAndRange(t, 50, tfStr, rangeFrac)
			repo := &fakeCandleRepo{series: series}
			scorer := &fakeScorer{stats: sidewaysFallbackStats()}
			svc := setups.NewSetupService(repo, scorer, setups.NewEngine())

			result, err := svc.Evaluate(context.Background(), "BTCUSDT", tfStr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Regime != "sideways" {
				t.Fatalf("test fixture must classify as sideways, got %q", result.Regime)
			}
			if result.VolatilityFit < 0.99 {
				t.Errorf("expected VolatilityFit ≈ 1.0 at %s's own ideal range, got %f — possible sign/exponent slip in volatilityDivisorForTimeframe", tfStr, result.VolatilityFit)
			}
		})
	}
}

// --- SeasonalityProvider wiring (PR-082) ---

func TestSetupService_NoSeasonalityProvider_DefaultsToNeutralFit(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		Scores: map[string]float64{"Compression": 0.5, "Trend Predictability": 0.5},
	}}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	// No SetSeasonalityProvider call — nil provider is the default.

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SeasonalityFit != 0.5 {
		t.Errorf("expected neutral SeasonalityFit 0.5 with no provider, got %f", result.SeasonalityFit)
	}
}

func TestSetupService_SeasonalityProvider_ComputesFitFromSpikeProbability(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		Scores: map[string]float64{"Compression": 0.5, "Trend Predictability": 0.5},
	}}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetSeasonalityProvider(&fakeSeasonalityProvider{spikeProb: 0.0}) // historically calm right now

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SeasonalityFit != 1.0 {
		t.Errorf("expected SeasonalityFit 1.0 for zero spike probability, got %f", result.SeasonalityFit)
	}
}

func TestSetupService_SeasonalityProviderError_DegradesGracefullyToNeutral(t *testing.T) {
	// Unlike the fragility provider's ctx.Err() escalation (Crowding is
	// weighted directly into the score, and a zero default silently
	// maximizes it), seasonality is explicitly a supplementary metric —
	// see service.go's doc on this. A failure here must never fail
	// Evaluate as a whole.
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		Scores: map[string]float64{"Compression": 0.5, "Trend Predictability": 0.5},
	}}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetSeasonalityProvider(&fakeSeasonalityProvider{err: errors.New("volatility data not loaded")})

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("expected Evaluate to succeed despite the seasonality provider failing, got: %v", err)
	}
	if result.SeasonalityFit != 0.5 {
		t.Errorf("expected neutral SeasonalityFit 0.5 on provider error, got %f", result.SeasonalityFit)
	}
}

// --- Evaluation store (PR-089b) ---

type fakeEvalStore struct {
	snap    domain.EvaluationSnapshot
	at      time.Time
	getErr  error
	calls   int
	lastSym string
	lastTF  string
}

func (f *fakeEvalStore) Put(context.Context, string, []domain.EvaluationSnapshot, time.Time) error {
	return nil
}

func (f *fakeEvalStore) Get(context.Context, string) ([]domain.EvaluationSnapshot, time.Time, error) {
	return nil, time.Time{}, ports.ErrEvaluationNotFound
}

func (f *fakeEvalStore) GetSymbol(_ context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error) {
	f.calls++
	f.lastTF = tf
	f.lastSym = symbol
	if f.getErr != nil {
		return domain.EvaluationSnapshot{}, time.Time{}, f.getErr
	}
	if symbol != f.snap.Symbol {
		return domain.EvaluationSnapshot{}, time.Time{}, ports.ErrEvaluationNotFound
	}
	return f.snap, f.at, nil
}

type countingScorer struct {
	fakeScorer
	calls int
}

func (c *countingScorer) Score(series domain.CandleSeries) (usecases.SymbolStats, error) {
	c.calls++
	return c.fakeScorer.Score(series)
}

func storeSnapForSeries(t *testing.T, series domain.CandleSeries) domain.EvaluationSnapshot {
	t.Helper()
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	trend, _, err := calc.ScoreWithDirection(series)
	if err != nil {
		t.Fatalf("ScoreWithDirection: %v", err)
	}
	return domain.EvaluationSnapshot{
		Symbol:            "BTCUSDT",
		TrendScore:        trend,
		CompressionScore:  0.1,
		SidewaysScore:     0.1,
		BreakoutUpScore:   0.55,
		BreakoutDownScore: 0.12,
		AlgoVersion:       domain.AlgoVersion,
		// Deliberately wrong sparkline Bias — must not drive Regime.
		Bias: "down",
	}
}

func TestSetupService_FreshStoreSkipsScorer(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeDirectionalSeries(50, true)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: storeSnapForSeries(t, series),
		at:   now.Add(-time.Minute),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 0 {
		t.Fatalf("expected scorer not called on fresh store, got %d", scorer.calls)
	}
	if store.calls != 1 {
		t.Fatalf("expected one GetSymbol, got %d", store.calls)
	}
	if result.Regime != "uptrend" {
		t.Errorf("expected uptrend from live ScoreWithDirection, got %q", result.Regime)
	}
	if result.BreakoutUp <= 0 {
		t.Errorf("expected store BreakoutUp to drive breakout confidence, got %f", result.BreakoutUp)
	}
}

func TestSetupService_MixedCaseSymbolHitsStore(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeDirectionalSeries(50, true)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: storeSnapForSeries(t, series),
		at:   now.Add(-time.Minute),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "btcusdt", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if store.lastSym != "BTCUSDT" {
		t.Fatalf("GetSymbol must use uppercase Symbol.String(), got %q", store.lastSym)
	}
	if store.lastTF != "4h" {
		t.Fatalf("GetSymbol timeframe=%q", store.lastTF)
	}
	if scorer.calls != 0 {
		t.Fatalf("lowercase path must hit store, scorer calls=%d", scorer.calls)
	}
}

func TestSetupService_StoreWarmColdRegimeAgree(t *testing.T) {
	// Same candle window: store-sourced scores with live trend overlay must
	// match cold scorer path Regime.
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeDirectionalSeries(50, false) // downtrend
	live := trendDominantStatsFor(t, series)
	snap := storeSnapForSeries(t, series)
	snap.TrendScore = live.Scores["Trend Predictability"]

	warmRepo := &fakeCandleRepo{series: series}
	warmScorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	warmStore := &fakeEvalStore{snap: snap, at: now.Add(-time.Minute)}
	warm := setups.NewSetupService(warmRepo, warmScorer, setups.NewEngine())
	warm.SetEvaluationStore(warmStore)
	warm.SetNow(func() time.Time { return now })

	coldRepo := &fakeCandleRepo{series: series}
	cold := setups.NewSetupService(coldRepo, &fakeScorer{stats: live}, setups.NewEngine())

	warmRes, err := warm.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("warm: %v", err)
	}
	coldRes, err := cold.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("cold: %v", err)
	}
	if warmRes.Regime != coldRes.Regime {
		t.Fatalf("warm Regime %q != cold Regime %q", warmRes.Regime, coldRes.Regime)
	}
	if warmRes.Regime != "downtrend" {
		t.Fatalf("expected downtrend for falling series, got %q", warmRes.Regime)
	}
	if warmScorer.calls != 0 {
		t.Fatalf("warm path must skip scorer, got %d", warmScorer.calls)
	}
}

func TestSetupService_StoreHighTrendLiveLowNotTrendDominant(t *testing.T) {
	// Stale store TrendScore must not keep Regime in uptrend when live
	// ScoreWithDirection on flat candles is near zero.
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeSeries(50) // flat closes → live trend ≈ 0
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol:            "BTCUSDT",
			TrendScore:        0.95,
			CompressionScore:  0.1,
			SidewaysScore:     0.1,
			BreakoutUpScore:   0.1,
			BreakoutDownScore: 0.1,
			AlgoVersion:       domain.AlgoVersion,
		},
		at: now.Add(-time.Minute),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 0 {
		t.Fatalf("expected store hit, scorer calls=%d", scorer.calls)
	}
	if result.Regime == "uptrend" || result.Regime == "downtrend" {
		t.Fatalf("expected non-trend regime after live overlay, got %q", result.Regime)
	}
}

func TestSetupService_ExactStaleBoundaryIsFresh(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tf := domain.NewTimeframeUnsafe("4h")
	staleAfter := domain.EvaluationStaleAfter(tf)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol: "BTCUSDT", TrendScore: 0.2, CompressionScore: 0.8,
			AlgoVersion: domain.AlgoVersion,
		},
		at: now.Add(-staleAfter),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 0 {
		t.Fatalf("age==StaleAfter must be fresh, scorer calls=%d", scorer.calls)
	}
}

func TestSetupService_StaleStoreFallsBackToScorer(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tf := domain.NewTimeframeUnsafe("4h")
	staleAfter := domain.EvaluationStaleAfter(tf)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: domain.AlgoVersion,
		},
		at: now.Add(-(staleAfter + time.Second)),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("expected scorer fallback on stale store, got %d calls", scorer.calls)
	}
}

func TestSetupService_AlgoVersionMismatchFallsBack(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: "stale-algo",
		},
		at: now.Add(-time.Second),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("algo mismatch must fall back, scorer calls=%d", scorer.calls)
	}
}

func TestSetupService_EmptyAlgoVersionFallsBack(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: "",
		},
		at: now.Add(-time.Second),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("empty AlgoVersion must fall back, scorer calls=%d", scorer.calls)
	}
}

func TestSetupService_StoreMissFallsBackToScorer(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{getErr: ports.ErrEvaluationNotFound}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("expected scorer on miss, got %d", scorer.calls)
	}
}

func TestSetupService_StoreTransportErrorFallsBackToScorer(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{getErr: errors.New("redis down")}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate should degrade on store transport error: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("expected scorer fallback, got %d", scorer.calls)
	}
}

func TestSetupService_StoreContextCanceledDoesNotScore(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{getErr: context.Canceled}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if scorer.calls != 0 {
		t.Fatalf("cancel must not fall through to scorer, got %d calls", scorer.calls)
	}
}

func TestSetupService_FutureAtFallsBackToScorer(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol: "BTCUSDT", TrendScore: 0.9, AlgoVersion: domain.AlgoVersion,
		},
		at: now.Add(time.Hour),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 1 {
		t.Fatalf("future at must fall back, scorer calls=%d", scorer.calls)
	}
}

func TestSetupService_OverlayUsesRankingsSizedWindow(t *testing.T) {
	// 200-bar series: early half flat (would be non-trend), late 110 rising.
	// Overlay must use the trailing 110 (rankings window), not the full 200.
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("4h")
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 200)
	for i := 0; i < 200; i++ {
		price := 100.0
		if i >= 90 {
			price = 100.0 + float64(i-90) // rising over last 110
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			price, price+2, price-2, price, 1000,
		)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("NewCandleSeries: %v", err)
	}
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol:           "BTCUSDT",
			TrendScore:       0.1, // deliberately low store magnitude
			CompressionScore: 0.1,
			SidewaysScore:    0.1,
			AlgoVersion:      domain.AlgoVersion,
		},
		at: now.Add(-time.Minute),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if scorer.calls != 0 {
		t.Fatalf("expected store hit, scorer calls=%d", scorer.calls)
	}
	if result.Regime != "uptrend" {
		t.Fatalf("trailing rankings window should yield uptrend, got %q", result.Regime)
	}
}

func TestSetupService_StoreScoresDriveCompressionSetup(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &countingScorer{fakeScorer: fakeScorer{stats: sidewaysFallbackStats()}}
	store := &fakeEvalStore{
		snap: domain.EvaluationSnapshot{
			Symbol:           "BTCUSDT",
			CompressionScore: 0.95,
			TrendScore:       0.1,
			SidewaysScore:    0.1,
			AlgoVersion:      domain.AlgoVersion,
		},
		at: now.Add(-time.Minute),
	}
	svc := setups.NewSetupService(repo, scorer, setups.NewEngine())
	svc.SetEvaluationStore(store)
	svc.SetNow(func() time.Time { return now })

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Regime != "compression" {
		t.Errorf("expected compression regime from store scores, got %q", result.Regime)
	}
	if scorer.calls != 0 {
		t.Fatalf("expected store hit, scorer calls=%d", scorer.calls)
	}
}

// --- Signal writers (PR-090) ---

type capturingSetupEmitter struct {
	mu   sync.Mutex
	sigs []domainsignal.Signal
}

func (c *capturingSetupEmitter) Emit(_ context.Context, s domainsignal.Signal) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sigs = append(c.sigs, s)
	return true
}

func TestSetupService_NilEmitterNoop(t *testing.T) {
	svc := setups.NewSetupService(
		&fakeCandleRepo{series: makeSeries(50)},
		&fakeScorer{stats: highCompressionStats()},
		setups.NewEngine(),
	)
	if _, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h"); err != nil {
		t.Fatal(err)
	}
}

func TestSetupService_EmitsMappedLabelWhenConfident(t *testing.T) {
	cap := &capturingSetupEmitter{}
	svc := setups.NewSetupService(
		&fakeCandleRepo{series: tightCompressionSeries(50)},
		&fakeScorer{stats: highCompressionStats()},
		setups.NewEngine(),
	)
	svc.SetSignalEmitter(cap)
	svc.SetMarketProvider(highEffectiveMarket{})
	svc.SetFragilityProvider(&fakeFragilityProvider{frag: domainrisk.Fragility{Score: 0.05}})
	svc.SetSeasonalityProvider(&fakeSeasonalityProvider{spikeProb: 0.05})

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatal(err)
	}
	if result.Confidence < 0.5 {
		t.Fatalf("fixture must reach Confidence≥0.5, got %.3f (regime=%s TH=%.2f ME=%.2f VF=%.2f SF=%.2f C=%.2f)",
			result.Confidence, result.Regime, result.TrendHealth, result.MarketEffective,
			result.VolatilityFit, result.SeasonalityFit, result.Crowding)
	}
	if len(cap.sigs) != 1 {
		t.Fatalf("expected 1 setup signal, got %d (confidence=%.2f best=%s)", len(cap.sigs), result.Confidence, result.BestSetup)
	}
	s := cap.sigs[0]
	if s.Kind != domainsignal.KindSetup {
		t.Fatalf("kind=%s", s.Kind)
	}
	switch s.Label {
	case "compression", "breakout_up", "breakout_down", "range", "trend_up", "trend_down":
		// PR-091 vocabulary
	default:
		t.Fatalf("unexpected setup label %q (raw BestSetup=%s)", s.Label, result.BestSetup)
	}
	if s.Price <= 0 || s.ATR <= 0 {
		t.Fatalf("price/atr must be positive: price=%v atr=%v", s.Price, s.ATR)
	}
}

type highEffectiveMarket struct{}

func (highEffectiveMarket) Calculate(context.Context, string) (mkt.Summary, error) {
	return mkt.Summary{EffectiveTrend: 0.55, State: mkt.StateCompression}, nil
}

// tightCompressionSeries builds a low-range series so VolatilityFit stays high
// for compression (needed to clear the Confidence ≥ 0.5 emit gate).
func tightCompressionSeries(n int) domain.CandleSeries {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("4h")
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			100, 100.2, 99.8, 100, 1000,
		)
	}
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

func highCompressionStats() usecases.SymbolStats {
	return usecases.SymbolStats{
		TotalScore: 2.5,
		Scores: map[string]float64{
			"Compression":          0.9,
			"Trend Predictability": 0.2,
			"Sideways":             0.2,
		},
	}
}
