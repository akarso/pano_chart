package setups

import (
	"context"
	"fmt"
	"log"
	"math"

	"pano_chart/backend/application/market"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/risk"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/domain/setup"
)

// MarketProvider returns the current market summary for a timeframe.
type MarketProvider interface {
	Calculate(timeframe string) (mkt.Summary, error)
}

// FragilityProvider returns the crowding / fragility assessment for a symbol.
type FragilityProvider interface {
	Get(ctx context.Context, symbol, timeframe string) (risk.Fragility, error)
}

// SetupService orchestrates candle retrieval, score computation, and setup
// evaluation for a single symbol.
type SetupService struct {
	candleRepo        ports.CandleRepositoryPort
	scorer            usecases.SymbolScorer
	engine            *Engine
	marketProvider    MarketProvider    // optional; nil means no market modifier
	fragilityProvider FragilityProvider // optional; nil means crowding = 0
}

const candleLimit = 200

// NewSetupService constructs the service.
func NewSetupService(repo ports.CandleRepositoryPort, scorer usecases.SymbolScorer, eng *Engine) *SetupService {
	return &SetupService{
		candleRepo: repo,
		scorer:     scorer,
		engine:     eng,
	}
}

// SetMarketProvider injects the market state provider (optional).
func (s *SetupService) SetMarketProvider(mp MarketProvider) {
	s.marketProvider = mp
}

// SetFragilityProvider injects the fragility/crowding provider (optional).
func (s *SetupService) SetFragilityProvider(fp FragilityProvider) {
	s.fragilityProvider = fp
}

// Evaluate fetches candles, computes underlying scores, builds a SetupContext,
// and runs the engine.
func (s *SetupService) Evaluate(_ context.Context, symbol, timeframe string) (setup.SetupScores, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid symbol: %w", err)
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid timeframe: %w", err)
	}

	series, err := s.candleRepo.GetLastNCandles(sym, tf, candleLimit)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("candle fetch: %w", err)
	}

	if series.Len() < 2 {
		return setup.SetupScores{
			Symbol:    symbol,
			Timeframe: timeframe,
			Scores:    map[setup.SetupType]float64{},
		}, nil
	}

	stats, err := s.scorer.Score(series)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("scoring: %w", err)
	}

	ctx := buildContext(symbol, series, stats)
	result := s.engine.Evaluate(ctx)
	result.Timeframe = timeframe

	// Apply market-level modifier when a provider is available.
	if s.marketProvider != nil {
		if summary, err := s.marketProvider.Calculate(timeframe); err == nil {
			result = ApplyMarketModifier(result, summary.EffectiveTrend)
		}
	}

	// Populate confidence inputs and compute unified confidence.
	result.VolatilityFit = VolatilityFit(result.Regime, ctx.Volatility)
	if s.fragilityProvider != nil {
		if frag, err := s.fragilityProvider.Get(context.Background(), symbol, timeframe); err == nil {
			result.Crowding = frag.Score
		}
	}
	result.Confidence = ComputeConfidence(result)

	// Compute confidence-adjusted breakout probabilities.
	result = ApplyBreakoutConfidence(
		result,
		stats.Scores["Breakout Up"],
		stats.Scores["Breakout Down"],
	)

	return result, nil
}

// buildContext converts raw scoring output and candle data into a SetupContext.
func buildContext(symbol string, series domain.CandleSeries, stats usecases.SymbolStats) SetupContext {
	regime, trendHealth := computeRegimeAndHealth(series, stats)
	return SetupContext{
		Symbol:           symbol,
		CompressionScore: stats.Scores["Compression"],
		TrendScore:       stats.Scores["Trend Predictability"],
		RangeScore:       rangeFromSideways(stats.Scores),
		VolumeScore:      volumeScore(series),
		Volatility:       volatilityFromSeries(series),
		TrendHealth:      trendHealth,
		Regime:           regime,
	}
}

// computeRegimeAndHealth determines the dominant regime and computes health.
func computeRegimeAndHealth(series domain.CandleSeries, stats usecases.SymbolStats) (string, float64) {
	n := series.Len()
	if n < 2 {
		return "sideways", 0
	}

	regime := dominantRegime(stats.Scores, series)

	if regime != "uptrend" && regime != "downtrend" {
		return regime, 0
	}

	last, _ := series.At(n - 1)
	price := last.Close()
	atr := simpleATR(series)

	recentHigh, recentLow := recentExtremes(series)
	recentReturn := recentReturnPct(series)

	health := market.ComputeTrendHealth(regime, price, recentHigh, recentLow, atr, recentReturn)
	return regime, health
}

// trendDirectionCalc is the single instance used to recover trend direction
// in dominantRegime — package-level so it's an explicit, visible
// dependency and not reallocated on every call.
var trendDirectionCalc = &scoring.TrendPredictabilityScoreCalculator{}

// scoresAgree reports whether two independently-obtained scores for what
// should be the same computation are close enough to trust — see
// dominantRegime's doc for why this matters. Today both sides call the
// exact same deterministic arithmetic on the exact same series, so they
// match bit-for-bit; epsilon exists only to tolerate future floating-point
// variation (e.g. a different summation order), not to absorb any
// currently-expected divergence — hence a very tight tolerance rather than
// a looser one.
func scoresAgree(a, b float64) bool {
	const epsilon = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// dominantRegime maps the highest-scoring dimension to a regime label.
// series is only consulted when trend is the dominant dimension, to
// recover the actual direction — see
// scoring.TrendPredictabilityScoreCalculator.ScoreWithDirection's doc for
// why this is the canonical direction source, not a magnitude threshold.
//
// scores["Trend Predictability"] and ScoreWithDirection(series) are two
// independent computations that happen to run the identical calculator
// over the identical series today (the generic WeightedSymbolScorer just
// calls each calculator's Score(series) unweighted into the map). That's
// an implicit invariant, not an enforced one — if the scorer is ever
// swapped for a decorator, cache, or resampled series, the two could
// silently diverge. scoresAgree checks it explicitly: if the recomputed
// magnitude doesn't match what the caller already scored, the direction
// can't be trusted either, so fall back to "sideways" rather than report
// a bias that might belong to a different series than the score did.
func dominantRegime(scores map[string]float64, series domain.CandleSeries) string {
	trend := scores["Trend Predictability"]
	compression := scores["Compression"]

	// Consider remaining scores as sideways indicators.
	sideways := 0.0
	for k, v := range scores {
		if k == "Compression" || k == "Trend Predictability" || k == "Gain/Loss" {
			continue
		}
		if v > sideways {
			sideways = v
		}
	}

	if compression > trend && compression > sideways {
		return "compression"
	}
	if trend > sideways {
		recomputed, bias, err := trendDirectionCalc.ScoreWithDirection(series)
		switch {
		case err != nil:
			// Not expected to happen here — computeRegimeAndHealth already
			// guards series.Len() < 2, and a regression over >= 2 distinct
			// indices can't hit ScoreWithDirection's other error path
			// (zero denominator). Logged because a masked error here would
			// otherwise be undiagnosable in the field.
			log.Printf("[setups] dominantRegime: ScoreWithDirection error, falling back to sideways: %v", err)
			return "sideways"
		case bias == "neutral":
			// No reliable direction (flat, clustered, or too little data)
			// despite a nonzero trend score from other calculators —
			// don't guess a direction that isn't there.
			return "sideways"
		case !scoresAgree(recomputed, trend):
			// The recomputed score doesn't match what was already scored —
			// series/scorer diverged somewhere; don't trust the bias. Logged
			// since this is the one branch scoresAgree's doc comment flags
			// as "should never happen today" — if it ever fires, that
			// assumption broke somewhere and needs investigating.
			log.Printf("[setups] dominantRegime: score mismatch (scored=%.6f recomputed=%.6f), falling back to sideways", trend, recomputed)
			return "sideways"
		case bias == "up":
			return "uptrend"
		default:
			return "downtrend"
		}
	}
	return "sideways"
}

// simpleATR computes average true range over the series.
func simpleATR(series domain.CandleSeries) float64 {
	n := series.Len()
	if n < 2 {
		return 0
	}

	var total float64
	for i := 1; i < n; i++ {
		c, _ := series.At(i)
		prev, _ := series.At(i - 1)

		tr := math.Max(c.High()-c.Low(),
			math.Max(math.Abs(c.High()-prev.Close()), math.Abs(c.Low()-prev.Close())))
		total += tr
	}
	return total / float64(n-1)
}

// recentExtremes finds the highest high and lowest low over the last 20 candles.
func recentExtremes(series domain.CandleSeries) (float64, float64) {
	n := series.Len()
	window := 20
	if n < window {
		window = n
	}
	high := 0.0
	low := math.MaxFloat64
	for i := n - window; i < n; i++ {
		c, _ := series.At(i)
		if c.High() > high {
			high = c.High()
		}
		if c.Low() < low {
			low = c.Low()
		}
	}
	return high, low
}

// recentReturnPct returns the percentage change over the last 5 candles.
func recentReturnPct(series domain.CandleSeries) float64 {
	n := series.Len()
	lookback := 5
	if n < lookback+1 {
		lookback = n - 1
	}
	if lookback <= 0 {
		return 0
	}
	old, _ := series.At(n - 1 - lookback)
	cur, _ := series.At(n - 1)
	if old.Close() == 0 {
		return 0
	}
	return (cur.Close() - old.Close()) / old.Close()
}

// rangeFromSideways derives a range score from sideways scores.
// Higher sideways consistency implies better range-reversion opportunity.
func rangeFromSideways(scores map[string]float64) float64 {
	// Use the best available sideways score.
	best := 0.0
	for k, v := range scores {
		if k == "Compression" || k == "Trend Predictability" || k == "Gain/Loss" {
			continue
		}
		if v > best {
			best = v
		}
	}
	return best
}

// volumeScore computes a normalised volume score from the series.
// Compares the most recent volume to the series average.
func volumeScore(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0
	}

	var total float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		total += c.Volume()
	}
	avg := total / float64(n)
	if avg == 0 {
		return 0
	}

	last, _ := series.At(n - 1)
	ratio := last.Volume() / avg
	// Normalise: ratio 0→0, ratio 1→0.5, ratio ≥2→1
	return clamp(ratio / 2.0)
}

// volatilityFromSeries computes a normalised volatility score.
// Uses ATR-like measure: average (high-low)/close, normalised to [0,1].
func volatilityFromSeries(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0
	}

	var total float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		if c.Close() == 0 {
			continue
		}
		total += (c.High() - c.Low()) / c.Close()
	}
	avg := total / float64(n)
	// Typical crypto daily range: 0-10% ≈ 0-0.1.
	// Map 0→0, 0.05→0.5, ≥0.1→1.
	return clamp(avg / 0.1)
}
