package transition

import (
	"context"
	"fmt"

	"pano_chart/backend/application/ports"
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

// TransitionService orchestrates regime detection and transition-probability
// calculation.  It is the primary entry point for the HTTP handler.
type TransitionService struct {
	regimeProvider RegimeProvider
	engine         *TransitionEngine
	ageProvider    AgeProvider         // optional — falls back to default when nil
	signalEmitter  ports.SignalEmitter // optional — PR-090
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

// Calculate fetches the current regime summary and returns transition
// probabilities for the requested timeframe.
func (s *TransitionService) Calculate(ctx context.Context, timeframe string) (mkt.MarketTransition, error) {
	summary, err := s.regimeProvider.CalculateWithCandleMetrics(ctx, timeframe)
	if err != nil {
		return mkt.MarketTransition{}, fmt.Errorf("transition: regime error: %w", err)
	}

	// Derive volatility slope from the single-point VolatilityExpansion metric.
	// A value of 1.0 is neutral; >1 means expansion, <1 compression.
	volSlope := summary.VolatilityExpansion - 1.0

	// Derive regime age from history; fall back to 12 if unavailable.
	regimeAge := 12
	if s.ageProvider != nil {
		if age, err := s.ageProvider.CurrentAge(timeframe); err == nil && age > 0 {
			regimeAge = age
		}
	}

	currentRegime := mkt.Regime(summary.State)
	probs := s.engine.Calculate(
		currentRegime,
		summary.Breadth.Compression,
		volSlope,
		regimeAge,
	)

	s.emitTransitionSignals(ctx, summary.Timeframe, probs)

	horizon := fmt.Sprintf("%d candles", regimeAge)
	if h := HumanDuration(summary.Timeframe, regimeAge); h != "" {
		horizon = fmt.Sprintf("%d candles (~%s)", regimeAge, h)
	}

	return mkt.MarketTransition{
		Timeframe:     summary.Timeframe,
		CurrentRegime: currentRegime,
		Probabilities: probs,
		Horizon:       horizon,
	}, nil
}

func (s *TransitionService) emitTransitionSignals(ctx context.Context, timeframe string, probs mkt.TransitionProbabilities) {
	if s.signalEmitter == nil {
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
