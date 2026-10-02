package setups

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"pano_chart/backend/application/market"
	"pano_chart/backend/application/ports"
	appsignal "pano_chart/backend/application/signal"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/risk"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/domain/setup"
	domainsignal "pano_chart/backend/domain/signal"
)

// MarketProvider returns the current market summary for a timeframe.
type MarketProvider interface {
	Calculate(ctx context.Context, timeframe string) (mkt.Summary, error)
}

// FragilityProvider returns the crowding / fragility assessment for a symbol.
type FragilityProvider interface {
	Get(ctx context.Context, symbol, timeframe string) (risk.Fragility, error)
}

// SeasonalityProvider returns the current moment's historical spike
// probability — a forward-looking risk read from time-of-day/day-of-week
// seasonality, as opposed to VolatilityFit's backward-looking read of
// realized volatility over the last N candles — see PR-082. timeframe is
// accepted for symmetry with MarketProvider/FragilityProvider and future
// use, but the current implementation (VolatilitySeasonalityProvider in
// adapters/http) always answers from 1-minute-of-day granularity
// regardless of it: infrastructure/volatility's coarser derived timeframes
// (5m/15m/1h/4h) group 1-minute buckets by array position, not by an
// explicit time range each bucket covers, so matching "the bucket
// containing right now" is unambiguous only at 1-minute granularity.
//
// CurrentSpikeProbabilityFor (PR-108) prefers a per-sector profile when
// available and falls back to the market-wide curve when the sector file
// is missing or sector is empty/"other".
type SeasonalityProvider interface {
	CurrentSpikeProbability(ctx context.Context, timeframe string) (float64, error)
	CurrentSpikeProbabilityFor(ctx context.Context, sector, timeframe string) (float64, error)
}

// SectorResolver maps a symbol to a sector id (PR-098 catalog).
// Optional on SetupService; nil → seasonality uses the market-wide curve.
type SectorResolver interface {
	ForSymbol(sym string) string
}

// SetupService orchestrates candle retrieval, score computation, and setup
// evaluation for a single symbol.
type SetupService struct {
	candleRepo          ports.CandleRepositoryPort
	scorer              usecases.SymbolScorer
	engine              *Engine
	marketProvider      MarketProvider        // optional; nil means no market modifier
	fragilityProvider   FragilityProvider     // optional; nil means crowding = 0
	seasonalityProvider SeasonalityProvider   // optional; nil means SeasonalityFit = neutral 0.5
	sectorResolver      SectorResolver        // optional; nil → market-wide seasonality only
	evalStore           ports.EvaluationStore // optional; nil → always re-score
	signalEmitter       ports.SignalEmitter   // optional; nil = no signal log (PR-090)
	// trendDir recovers trend magnitude+bias for overlay / dominantRegime.
	// Must match the rankings / WeightedSymbolScorer trend engine (PR-103).
	trendDir        scoring.DirectedScoreCalculator
	trendAlgo       string // expected EvaluationSnapshot.TrendAlgo for store hits
	compressionAlgo string // expected EvaluationSnapshot.CompressionAlgo for store hits
	now             func() time.Time
}

const candleLimit = 200

// storeTrendOverlayBars is the candle window used when overlaying live trend
// magnitude onto store scores. Must match GetRankings default precision /
// sparkline length so warm dominance compares like-for-like with store
// Compression/Sideways/Breakout (not the full setup 200-bar series).
const storeTrendOverlayBars = 110

// NewSetupService constructs the service.
func NewSetupService(repo ports.CandleRepositoryPort, scorer usecases.SymbolScorer, eng *Engine) *SetupService {
	return &SetupService{
		candleRepo:      repo,
		scorer:          scorer,
		engine:          eng,
		trendDir:        &scoring.TrendPredictabilityScoreCalculator{},
		trendAlgo:       domain.DefaultTrendAlgo,
		compressionAlgo: domain.DefaultCompressionAlgo,
		now:             time.Now,
	}
}

// SetTrendDirectionCalc sets the engine used for store-trend overlay and
// dominantRegime direction (PR-103). Must match the WeightedSymbolScorer
// trend calculator; default is Trend Predictability.
func (s *SetupService) SetTrendDirectionCalc(c scoring.DirectedScoreCalculator) {
	if c == nil {
		return
	}
	s.trendDir = c
}

// SetTrendAlgo sets the expected EvaluationSnapshot.TrendAlgo for store hits
// (PR-103). Independent of SetTrendDirectionCalc (live overlay engine).
func (s *SetupService) SetTrendAlgo(algo string) {
	mode, _ := usecases.ParseTrendAlgo(algo)
	s.trendAlgo = string(mode)
}

// SetCompressionAlgo sets the expected EvaluationSnapshot.CompressionAlgo for
// store hits (PR-105).
func (s *SetupService) SetCompressionAlgo(algo string) {
	mode, _ := usecases.ParseCompressionAlgo(algo)
	s.compressionAlgo = string(mode)
}

// SetMarketProvider injects the market state provider (optional).
func (s *SetupService) SetMarketProvider(mp MarketProvider) {
	s.marketProvider = mp
}

// SetFragilityProvider injects the fragility/crowding provider (optional).
func (s *SetupService) SetFragilityProvider(fp FragilityProvider) {
	s.fragilityProvider = fp
}

// SetSeasonalityProvider injects the volatility-seasonality provider
// (optional).
func (s *SetupService) SetSeasonalityProvider(sp SeasonalityProvider) {
	s.seasonalityProvider = sp
}

// SetSectorResolver injects the symbol→sector map used for per-sector
// seasonality (PR-108). Optional; without it, setups always use the
// market-wide spike curve.
func (s *SetupService) SetSectorResolver(r SectorResolver) {
	s.sectorResolver = r
}

// SetEvaluationStore injects the shared evaluation store (optional, PR-089b).
// When present and fresh, scores come from GetSymbol instead of SymbolScorer.
func (s *SetupService) SetEvaluationStore(store ports.EvaluationStore) {
	s.evalStore = store
}

// SetSignalEmitter attaches an optional signal logger (PR-090).
func (s *SetupService) SetSignalEmitter(e ports.SignalEmitter) {
	s.signalEmitter = e
}

// SetNow overrides the clock used for store freshness (tests).
func (s *SetupService) SetNow(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}

// Evaluate fetches candles, resolves scores (store when fresh, else scorer),
// builds a SetupContext, and runs the engine.
func (s *SetupService) Evaluate(ctx context.Context, symbol, timeframe string) (setup.SetupScores, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid symbol: %w", err)
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid timeframe: %w", err)
	}

	series, err := s.candleRepo.GetLastNCandles(ctx, sym, tf, candleLimit)
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

	stats, err := s.resolveScores(ctx, sym, tf, series)
	if err != nil {
		return setup.SetupScores{}, err
	}

	setupCtx := buildContext(sym.String(), series, stats, s.trendDir)
	result := s.engine.Evaluate(setupCtx)
	result.Timeframe = timeframe

	// Apply market-level modifier when a provider is available.
	if s.marketProvider != nil {
		if summary, err := s.marketProvider.Calculate(ctx, timeframe); err == nil {
			result = ApplyMarketModifier(result, summary.EffectiveTrend)
		}
	}

	// Populate confidence inputs and compute unified confidence.
	result.VolatilityFit = VolatilityFit(result.Regime, setupCtx.Volatility)

	// Forward-looking seasonality fit — supplementary, like
	// VolatilityExpansion/Dispersion in market.MarketStateService: a
	// failure here must not fail Evaluate as a whole, so SeasonalityFit
	// simply stays at its neutral default (matching what a nil provider
	// already produces) rather than escalating the way the fragility
	// ctx.Err() check below does for Crowding — see PR-082.
	result.SeasonalityFit = 0.5
	if s.seasonalityProvider != nil {
		sector := ""
		if s.sectorResolver != nil {
			sector = s.sectorResolver.ForSymbol(symbol)
		}
		if spikeProb, serr := s.seasonalityProvider.CurrentSpikeProbabilityFor(ctx, sector, timeframe); serr == nil {
			result.SeasonalityFit = SeasonalityFit(spikeProb)
		}
	}

	if s.fragilityProvider != nil {
		frag, err := s.fragilityProvider.Get(ctx, symbol, timeframe)
		switch {
		case err == nil:
			result.Crowding = frag.Score
		case ctx.Err() != nil:
			// The caller gave up, not the fragility provider — Crowding's
			// zero default would otherwise silently maximize the crowding
			// contribution to ComputeConfidence below, returning a
			// confidently-computed result built on data we never actually
			// obtained. Abort instead of masking cancellation as success.
			return setup.SetupScores{}, ctx.Err()
		}
		// else: fragility provider failed for a reason unrelated to
		// cancellation (network hiccup, bad data) — degrade gracefully,
		// Crowding stays at its zero default.
	}
	result.Confidence = ComputeConfidence(result)

	// Compute confidence-adjusted breakout probabilities.
	result = ApplyBreakoutConfidence(
		result,
		stats.Scores["Breakout Up"],
		stats.Scores["Breakout Down"],
	)

	s.emitSetupSignal(ctx, result, series, stats)
	return result, nil
}

func (s *SetupService) emitSetupSignal(ctx context.Context, result setup.SetupScores, series domain.CandleSeries, stats usecases.SymbolStats) {
	if s.signalEmitter == nil || result.Confidence < 0.5 {
		return
	}
	price, atr := seriesPriceATR(series)
	label := appsignal.SetupLabel(string(result.BestSetup), result.Regime, result.BreakoutUp, result.BreakoutDown)
	ctxNums := map[string]float64{
		"setup_score":      result.Score,
		"trend_health":     result.TrendHealth,
		"breakout_up":      result.BreakoutUp,
		"breakout_down":    result.BreakoutDown,
		"crowding":         result.Crowding,
		"volatility_fit":   result.VolatilityFit,
		"seasonality_fit":  result.SeasonalityFit,
		"market_effective": result.MarketEffective,
	}
	// Same feature builder as dominantRegimeLearned (PR-109 train/serve parity).
	feats := setupRegimeFeatures(stats.Scores, series)
	for k, v := range feats {
		ctxNums[k] = v
	}
	// Structure-training label from four-way raw-score argmax (includes
	// expansion). Distinct from result.Regime, which folds expansion→sideways.
	ctxNums["regime_code"] = structureRegimeCode(
		feats["trend"], feats["sideways"], feats["compression"], feats["expansion"],
	)
	if label == "range" || label == "compression" {
		hi, lo := recentExtremes(series)
		ctxNums["range_low"] = lo
		ctxNums["range_high"] = hi
	}
	s.signalEmitter.Emit(ctx, domainsignal.Signal{
		Kind:      domainsignal.KindSetup,
		Symbol:    result.Symbol,
		Timeframe: result.Timeframe,
		Label:     label,
		Score:     result.Confidence,
		Price:     price,
		ATR:       atr,
		Context:   ctxNums,
	})
}

// setupRegimeFeatures builds the PR-109 feature map shared by emit and
// dominantRegimeLearned. Uses raw score keys (not confidence-adjusted
// breakouts) and Wilder TrueATR(14) so CSV rows match live classification.
func setupRegimeFeatures(scores map[string]float64, series domain.CandleSeries) map[string]float64 {
	trend := scores["Trend Predictability"]
	compression := scores["Compression"]
	sideways := scores["Sideways Consistency"]
	expansion := scores["Breakout Up"]
	if down := scores["Breakout Down"]; down > expansion {
		expansion = down
	}
	candles := series.All()
	closes := scoring.ClosesFromCandles(candles)
	atr := scoring.TrueATR(candles, 14)
	price := 0.0
	if n := len(closes); n > 0 {
		price = closes[n-1]
	}
	return scoring.BuildRegimeFeatures(trend, sideways, compression, expansion, closes, atr, price)
}

// structureRegimeCode encodes the four-way argmax of raw scores for export
// training (0=sideways, 1=trend, 2=compression, 3=expansion). Independent of
// result.Regime, which folds expansion into sideways for setup labels.
func structureRegimeCode(trend, sideways, compression, expansion float64) float64 {
	code := 0.0
	best := sideways
	if trend >= best {
		best = trend
		code = 1
	}
	if compression >= best {
		best = compression
		code = 2
	}
	if expansion >= best {
		code = 3
	}
	return code
}

func seriesPriceATR(series domain.CandleSeries) (price, atr float64) {
	n := series.Len()
	if n == 0 {
		return 0, 0
	}
	last, err := series.At(n - 1)
	if err != nil {
		return 0, 0
	}
	price = last.Close()
	// Prefer Wilder TrueATR(14) when the series is long enough; fall back to
	// SimpleATR so short series (2–14 bars) still grade with a positive ATR.
	atr = scoring.TrueATR(series.All(), 14)
	if atr <= 0 {
		atr = usecases.SimpleATR(series, 14)
	}
	return price, atr
}

// resolveScores prefers a fresh EvaluationStore snapshot; otherwise scores
// the candle series. Candles remain required for volume/volatility in
// buildContext regardless of the score source. Store hits overlay live
// Trend Predictability from ScoreWithDirection so dominance / Regime track
// the candles Evaluate just fetched (compression/sideways/breakout stay
// from the store). Overlay failure is treated as a miss → live scorer.
func (s *SetupService) resolveScores(ctx context.Context, sym domain.Symbol, tf domain.Timeframe, series domain.CandleSeries) (usecases.SymbolStats, error) {
	if stats, ok, err := s.scoresFromStore(ctx, sym, tf); err != nil {
		return usecases.SymbolStats{}, err
	} else if ok {
		if overlaid, ok := s.overlayLiveTrend(stats, series); ok {
			return overlaid, nil
		}
		log.Printf("[eval] setup reason=overlay symbol=%s tf=%s", sym.String(), tf.String())
	}
	stats, err := s.scorer.Score(series)
	if err != nil {
		return usecases.SymbolStats{}, fmt.Errorf("scoring: %w", err)
	}
	return stats, nil
}

// scoresFromStore returns (stats, true, nil) on a fresh hit; (zero, false, nil)
// on miss/stale/algo/transport; and a non-nil error for context cancel/deadline
// so Evaluate does not fall through to live scoring after the caller gave up.
func (s *SetupService) scoresFromStore(ctx context.Context, sym domain.Symbol, tf domain.Timeframe) (usecases.SymbolStats, bool, error) {
	if s.evalStore == nil {
		return usecases.SymbolStats{}, false, nil
	}
	symbol := sym.String()
	timeframe := tf.String()
	snap, at, err := s.evalStore.GetSymbol(ctx, timeframe, symbol)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return usecases.SymbolStats{}, false, err
		}
		if errors.Is(err, ports.ErrEvaluationNotFound) {
			log.Printf("[eval] setup reason=miss symbol=%s tf=%s", symbol, timeframe)
		} else {
			log.Printf("[eval] setup reason=transport symbol=%s tf=%s err=%v", symbol, timeframe, err)
		}
		return usecases.SymbolStats{}, false, nil
	}
	if !domain.EvaluationIdentityOK(snap.AlgoVersion, snap.TrendAlgo, s.trendAlgo, snap.CompressionAlgo, s.compressionAlgo) {
		log.Printf("[eval] setup reason=algo symbol=%s tf=%s got=%q/%q/%q want=%q/%q/%q",
			symbol, timeframe, snap.AlgoVersion, snap.TrendAlgo, snap.CompressionAlgo,
			domain.AlgoVersion, s.trendAlgo, s.compressionAlgo)
		return usecases.SymbolStats{}, false, nil
	}
	now := s.now
	if now == nil {
		now = time.Now
	}
	age := now().Sub(at)
	if !domain.EvaluationStoreFresh(at, now(), tf) {
		log.Printf("[eval] setup reason=stale symbol=%s tf=%s at=%s age=%s", symbol, timeframe, at.UTC().Format(time.RFC3339), age)
		return usecases.SymbolStats{}, false, nil
	}
	// Hits silent at info — see provider readStore.
	return statsFromSnapshot(snap), true, nil
}

func statsFromSnapshot(snap domain.EvaluationSnapshot) usecases.SymbolStats {
	return usecases.SymbolStats{
		Scores: map[string]float64{
			"Compression":          snap.CompressionScore,
			"Trend Predictability": snap.TrendScore,
			"Sideways Consistency": snap.SidewaysScore,
			"Breakout Up":          snap.BreakoutUpScore,
			"Breakout Down":        snap.BreakoutDownScore,
		},
	}
}

// overlayLiveTrend replaces store Trend Predictability with a live
// ScoreWithDirection magnitude+bias on the rankings-sized trailing window
// (storeTrendOverlayBars), so warm-path dominance uses the same bar count as
// the store scores. Full setup series still drives volume/volatility.
// ok=false → miss.
func (s *SetupService) overlayLiveTrend(stats usecases.SymbolStats, series domain.CandleSeries) (usecases.SymbolStats, bool) {
	window, err := trailingWindow(series, storeTrendOverlayBars)
	if err != nil {
		return usecases.SymbolStats{}, false
	}
	trendDir := s.trendDir
	if trendDir == nil {
		trendDir = &scoring.TrendPredictabilityScoreCalculator{}
	}
	recomputed, bias, err := trendDir.ScoreWithDirection(window)
	if err != nil {
		return usecases.SymbolStats{}, false
	}
	scores := make(map[string]float64, len(stats.Scores))
	for k, v := range stats.Scores {
		scores[k] = v
	}
	scores["Trend Predictability"] = recomputed
	stats.Scores = scores
	stats.DirectionBias = bias
	return stats, true
}

// trailingWindow returns the last n candles as a new series (or the whole
// series when shorter). Used so store-trend overlay matches rankings precision.
func trailingWindow(series domain.CandleSeries, n int) (domain.CandleSeries, error) {
	if n <= 0 || series.Len() <= n {
		return series, nil
	}
	all := series.All()
	tail := all[len(all)-n:]
	first, err := series.At(0)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	return domain.NewCandleSeries(first.Symbol(), series.Timeframe(), tail)
}

// buildContext converts raw scoring output and candle data into a SetupContext.
// MeanReversionScore is left at 0: no setup evaluator reads it today, and
// production Sideways MRS weight is 0. Call meanReversionFromSeries when a
// consumer (PR-110) needs it — avoid per-symbol allocations on notify scans.
func buildContext(symbol string, series domain.CandleSeries, stats usecases.SymbolStats, trendDir scoring.DirectedScoreCalculator) SetupContext {
	regime, trendHealth := computeRegimeAndHealth(series, stats, trendDir)
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

// meanReversionFromSeries returns Lo–MacKinlay MRS on the same trailing
// window Sideways V5 uses (sideways candle_count, default 110).
func meanReversionFromSeries(series domain.CandleSeries) float64 {
	window, err := trailingWindow(series, sidewaysCandleCount())
	if err != nil || window.Len() < 2 {
		return 0
	}
	closes := make([]float64, window.Len())
	for i := 0; i < window.Len(); i++ {
		c, err := window.At(i)
		if err != nil {
			return 0
		}
		closes[i] = c.Close()
	}
	return scoring.MeanReversionScore(closes)
}

// sidewaysCandleCount is Sideways V5's CandleCount without building a full
// SidewaysV5Config (MRS window must match DetectSidewaysV5).
func sidewaysCandleCount() int {
	app := scoring.GetConfig()
	if app == nil {
		app = scoring.DefaultAppConfig()
	}
	if app.Sideways.CandleCount > 0 {
		return app.Sideways.CandleCount
	}
	return 110
}

// computeRegimeAndHealth determines the dominant regime and computes health.
func computeRegimeAndHealth(series domain.CandleSeries, stats usecases.SymbolStats, trendDir scoring.DirectedScoreCalculator) (string, float64) {
	n := series.Len()
	if n < 2 {
		return "sideways", 0
	}

	regime := dominantRegime(stats.Scores, series, stats.DirectionBias, trendDir)

	if regime != "uptrend" && regime != "downtrend" {
		return regime, 0
	}

	last, _ := series.At(n - 1)
	price := last.Close()
	atr := simpleATR(series)

	recentHigh, recentLow := recentExtremes(series)
	recentReturn := recentReturnATR(series, atr)

	health := market.ComputeTrendHealth(regime, price, recentHigh, recentLow, atr, recentReturn)
	return regime, health
}

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
// When directionBias is non-empty (warm store overlay already ran
// ScoreWithDirection), that bias is used for direction — Trend Predictability
// in scores is the matching live magnitude, so scoresAgree is not re-checked.
// On the cold path directionBias is empty and series is scored via
// ScoreWithDirection + scoresAgree — see that method's doc for why this is
// the canonical direction source.
//
// trendDir must be the same engine that produced scores["Trend Predictability"]
// (PR-103: predictability or strength).
//
// EvaluationSnapshot.Bias stays the sparkline first/last signal for Market
// Pulse and is not used here.
func dominantRegime(scores map[string]float64, series domain.CandleSeries, directionBias string, trendDir scoring.DirectedScoreCalculator) string {
	if label, ok := dominantRegimeLearned(scores, series); ok {
		if label != "trend" {
			return label
		}
		return resolveTrendDirection(scores["Trend Predictability"], series, directionBias, trendDir)
	}

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
		return resolveTrendDirection(trend, series, directionBias, trendDir)
	}
	return "sideways"
}

// resolveTrendDirection maps a trend-dominant call to uptrend/downtrend/sideways.
func resolveTrendDirection(trend float64, series domain.CandleSeries, directionBias string, trendDir scoring.DirectedScoreCalculator) string {
	if directionBias != "" {
		return regimeFromDirectionBias(directionBias)
	}
	if trendDir == nil {
		trendDir = &scoring.TrendPredictabilityScoreCalculator{}
	}
	recomputed, bias, err := trendDir.ScoreWithDirection(series)
	switch {
	case err != nil:
		log.Printf("[setups] dominantRegime: ScoreWithDirection error, falling back to sideways: %v", err)
		return "sideways"
	case bias == "neutral":
		return "sideways"
	case !scoresAgree(recomputed, trend):
		log.Printf("[setups] dominantRegime: score mismatch (scored=%.6f recomputed=%.6f), falling back to sideways", trend, recomputed)
		return "sideways"
	case bias == "up":
		return "uptrend"
	default:
		return "downtrend"
	}
}

// dominantRegimeLearned maps a one-vs-rest model to setup regime labels.
// Expansion is folded into sideways (setups have no expansion regime string).
// ok is false when no model is installed or classification fails.
func dominantRegimeLearned(scores map[string]float64, series domain.CandleSeries) (string, bool) {
	model := scoring.ActiveRegimeModel()
	if model == nil {
		return "", false
	}
	features := setupRegimeFeatures(scores, series)
	_, _, _, _, dominant, ok := scoring.ClassifyStructure(features, *model)
	if !ok {
		return "", false
	}
	switch dominant {
	case scoring.ClassTrend:
		return "trend", true // caller resolves up/down
	case scoring.ClassCompression:
		return "compression", true
	case scoring.ClassExpansion, scoring.ClassSideways:
		return "sideways", true
	default:
		return "sideways", true
	}
}

func regimeFromDirectionBias(bias string) string {
	switch bias {
	case "up":
		return "uptrend"
	case "down":
		return "downtrend"
	default:
		return "sideways"
	}
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

// recentReturnATR returns the last-5-candle price change in ATR units, the
// unit ComputeTrendHealth expects for its adverse-move thresholds.
func recentReturnATR(series domain.CandleSeries, atr float64) float64 {
	if atr <= 0 {
		return 0
	}
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
	return (cur.Close() - old.Close()) / atr
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

// dailyVolatilityDivisor is the volatilityFromSeries normalization divisor
// calibrated for 1d candles: typical crypto daily range 0-10% ≈ 0-0.1.
const dailyVolatilityDivisor = 0.1

// volatilityFromSeries computes a normalised volatility score.
// Uses ATR-like measure: average (high-low)/close, normalised to [0,1]
// against a divisor scaled for the series' own timeframe (see
// volatilityDivisorForTimeframe) — PR-080.
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
	divisor := volatilityDivisorForTimeframe(series.Timeframe())
	if divisor <= 0 {
		// Not a real runtime path for any of the six canonical Timeframe
		// values (each has a fixed positive Duration()) — only reachable via
		// a NewTimeframeUnsafe zero/garbage value from tests or misuse.
		return 0
	}
	return clamp(avg / divisor)
}

// dailyMinutes is domain.Timeframe1d's duration in minutes, hoisted to
// package scope so volatilityDivisorForTimeframe doesn't recompute it on
// every call (CR follow-up, PR-080).
var dailyMinutes = domain.Timeframe1d.Duration().Minutes()

// volatilityDivisorForTimeframe scales dailyVolatilityDivisor down for
// sub-daily timeframes using sqrt(time) scaling — the standard random-walk
// assumption that volatility scales with the square root of elapsed time
// (vol(Δt) ≈ vol(1d) * sqrt(Δt/1d)). Without this, volatilityFromSeries
// applied dailyVolatilityDivisor unconditionally at every timeframe: a 15m
// candle's average (high-low)/close is typically ~0.1-0.5%, nowhere close to
// the ~10% this constant assumes, so the normalized result was silently
// pinned near 0 for every sub-daily timeframe — see PR-080.
//
// This is a scaling heuristic, not an empirically fitted constant (neither
// was the single fixed divisor it replaces) — provisional pending real
// per-timeframe score-distribution telemetry, not a calibrated result. Real
// crypto intraday ranges don't necessarily follow clean sqrt(time) scaling
// (fee/tick-size floors, session effects), so treat 1m/5m/1h/4h as
// plausible starting points, not validated — see PR-080's doc.
func volatilityDivisorForTimeframe(tf domain.Timeframe) float64 {
	if dailyMinutes <= 0 {
		return 0
	}
	scale := math.Sqrt(tf.Duration().Minutes() / dailyMinutes)
	return dailyVolatilityDivisor * scale
}
