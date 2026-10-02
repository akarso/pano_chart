package transition

import (
	"context"
	"fmt"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/replay"
	mkt "pano_chart/backend/domain/market"
	domainsignal "pano_chart/backend/domain/signal"
)

// RegimeProvider abstracts market-state computation so the transition
// service can be tested without the full MarketStateService dependency
// chain. Satisfied directly by *appmarket.MarketStateService's
// CalculateWithCandleMetrics — the transition engine needs
// VolatilityExpansion for its volatility-slope input, which the cheaper
// Calculate doesn't compute.
type RegimeProvider interface {
	CalculateWithCandleMetrics(ctx context.Context, timeframe string) (mkt.Summary, error)
}

// AgeProvider returns the current regime age in candles.
// Satisfied by regimehistory.Service.CurrentAge.
type AgeProvider interface {
	CurrentAge(timeframe string) (int, error)
}

// openPeriodStarter optionally identifies the live open period so a cached
// MergedPrefix is only applied when it belongs to the same period (not a
// same-named regime that started after a flip within the matrix TTL).
type openPeriodStarter interface {
	OpenPeriodStart(timeframe string) (int64, error)
}

// replayAgeProvider supplies regime age at a replay cutoff (PR-112a).
type replayAgeProvider interface {
	AgeAtAsOf(ctx context.Context, timeframe string, asOf time.Time) (int, error)
}

// TransitionService orchestrates regime detection and transition-probability
// calculation.  It is the primary entry point for the HTTP handler.
type TransitionService struct {
	regimeProvider RegimeProvider
	engine         *TransitionEngine
	ageProvider    AgeProvider         // optional — falls back to default when nil
	signalEmitter  ports.SignalEmitter // optional — PR-090
	matrixCache    *MatrixCache        // optional — PR-107 empirical blend
}

// NewTransitionService wires the service.
func NewTransitionService(rp RegimeProvider, eng *TransitionEngine) *TransitionService {
	return &TransitionService{
		regimeProvider: rp,
		engine:         eng,
	}
}

// SetAgeProvider attaches a regime-history-based age provider.
func (s *TransitionService) SetAgeProvider(ap AgeProvider) {
	s.ageProvider = ap
}

// SetSignalEmitter attaches an optional signal logger (PR-090).
func (s *TransitionService) SetSignalEmitter(e ports.SignalEmitter) {
	s.signalEmitter = e
}

// SetMatrixCache attaches an empirical transition matrix cache (PR-107).
func (s *TransitionService) SetMatrixCache(c *MatrixCache) {
	s.matrixCache = c
}

// Calculate fetches the current regime summary and returns transition
// probabilities for the requested timeframe.
func (s *TransitionService) Calculate(ctx context.Context, timeframe string) (mkt.MarketTransition, error) {
	summary, err := s.regimeProvider.CalculateWithCandleMetrics(ctx, timeframe)
	if err != nil {
		return mkt.MarketTransition{}, fmt.Errorf("transition: regime error: %w", err)
	}

	volSlope := summary.VolatilityExpansion - 1.0
	regimeAge := s.regimeAge(ctx, timeframe)
	currentRegime := mkt.Regime(summary.State)
	view, regimeAge := s.matrixAndAge(ctx, timeframe, currentRegime, regimeAge)

	heuristic := s.engine.Calculate(
		currentRegime,
		summary.Breadth.Compression,
		volSlope,
		regimeAge,
	)

	probs, source, empiricalWeight, sampleSize, pooled := s.blendEmpirical(view, currentRegime, regimeAge, heuristic)

	s.emitTransitionSignals(ctx, summary.Timeframe, probs)

	return mkt.MarketTransition{
		Timeframe:       summary.Timeframe,
		CurrentRegime:   currentRegime,
		Probabilities:   probs,
		Horizon:         formatHorizon(summary.Timeframe, regimeAge),
		Source:          source,
		EmpiricalWeight: empiricalWeight,
		SampleSize:      sampleSize,
		Pooled:          pooled,
	}, nil
}

func (s *TransitionService) liveAge(timeframe string) int {
	age := 12
	if s.ageProvider != nil {
		if a, err := s.ageProvider.CurrentAge(timeframe); err == nil && a > 0 {
			age = a
		}
	}
	return age
}

func (s *TransitionService) regimeAge(ctx context.Context, timeframe string) int {
	if asOf, ok := replay.AsOf(ctx); ok {
		// Never invent the live default age under replay (PR-112a).
		if rap, ok := s.ageProvider.(replayAgeProvider); ok {
			if a, err := rap.AgeAtAsOf(ctx, timeframe, asOf); err == nil && a > 0 {
				return a
			}
		}
		return 0
	}
	return s.liveAge(timeframe)
}

func (s *TransitionService) matrixAndAge(
	ctx context.Context,
	timeframe string,
	current mkt.Regime,
	liveAge int,
) (MatrixView, int) {
	// Replay: skip live matrix cache (keyed without asOf) — heuristic only (PR-112a).
	if _, ok := replay.AsOf(ctx); ok {
		return MatrixView{}, liveAge
	}
	if s.matrixCache == nil {
		return MatrixView{}, liveAge
	}
	view := s.matrixCache.Matrix(ctx, timeframe)
	return view, applyMergedPrefix(s.ageProvider, timeframe, current, liveAge, view)
}

// applyMergedPrefix adds cached closed predecessors only when the live open
// period is the same period the matrix snapshot saw (matching start) and the
// core regimes align (silent/indecisive ≡ sideways).
func applyMergedPrefix(
	ages AgeProvider,
	timeframe string,
	current mkt.Regime,
	liveAge int,
	view MatrixView,
) int {
	if view.Stale || view.MergedPrefix <= 0 || view.OpenStart == 0 {
		return liveAge
	}
	core, ok := coreRegime(current)
	if !ok || view.MergedRegime != core {
		return liveAge
	}
	starter, ok := ages.(openPeriodStarter)
	if !ok {
		return liveAge
	}
	start, err := starter.OpenPeriodStart(timeframe)
	if err != nil || start == 0 || start != view.OpenStart {
		return liveAge
	}
	return view.MergedPrefix + liveAge
}

func (s *TransitionService) blendEmpirical(
	view MatrixView,
	current mkt.Regime,
	regimeAge int,
	heuristic mkt.TransitionProbabilities,
) (probs mkt.TransitionProbabilities, source string, weight float64, sampleSize int, pooled bool) {
	probs = heuristic
	source = "heuristic"
	if s.matrixCache == nil || view.Stale {
		return probs, source, 0, 0, false
	}
	look, ok := view.Matrix.Lookup(current, regimeAge)
	if !ok {
		return probs, source, 0, 0, false
	}
	sampleSize = look.SampleSize
	pooled = look.Pooled
	if look.SampleSize < minBlendSamples {
		return probs, source, 0, sampleSize, pooled
	}
	weight = WeightFromSamples(look.SampleSize)
	return Blend(look.Probabilities, heuristic, weight), "blend", weight, sampleSize, pooled
}

func formatHorizon(timeframe string, regimeAge int) string {
	horizon := fmt.Sprintf("%d candles", regimeAge)
	if h := HumanDuration(timeframe, regimeAge); h != "" {
		horizon = fmt.Sprintf("%d candles (~%s)", regimeAge, h)
	}
	return horizon
}

func (s *TransitionService) emitTransitionSignals(ctx context.Context, timeframe string, probs mkt.TransitionProbabilities) {
	if s.signalEmitter == nil {
		return
	}
	if _, ok := replay.AsOf(ctx); ok {
		return
	}
	targets := []struct {
		label string
		p     float64
	}{
		{"transition:trend", probs.Trend},
		{"transition:sideways", probs.Sideways},
		{"transition:compression", probs.Compression},
		{"transition:expansion", probs.Expansion},
	}
	for _, t := range targets {
		if t.p < 0.5 {
			continue
		}
		s.signalEmitter.Emit(ctx, domainsignal.Signal{
			Kind:      domainsignal.KindTransition,
			Timeframe: timeframe,
			Label:     t.label,
			Score:     t.p,
			Context: map[string]float64{
				"trend":       probs.Trend,
				"sideways":    probs.Sideways,
				"compression": probs.Compression,
				"expansion":   probs.Expansion,
			},
		})
	}
}
