package notifications

import (
	"context"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// SetEvaluationStore attaches the shared evaluation snapshot store used to
// build PR-102 alert context (sparkline, total score, RS).
func (s *Scheduler) SetEvaluationStore(store ports.EvaluationStore) {
	s.evalStore = store
}

// alertBuildCache is tick-local: one MarketProvider / EvaluationStore /
// RegimeStackProvider hit per distinct key, not per subscriber.
type alertBuildCache struct {
	tapes    map[string]*mkt.Summary
	contexts map[string]AlertContext
}

func newAlertBuildCache() *alertBuildCache {
	return &alertBuildCache{
		tapes:    make(map[string]*mkt.Summary),
		contexts: make(map[string]AlertContext),
	}
}

func contextCacheKey(timeframe, symbol string) string {
	return timeframe + "\x00" + symbol
}

// buildAlertContext assembles tape + optional symbol context. Failures to
// read the store or compute alignment never block the caller — missing
// fields are simply left unset. Stale or wrong-algo snapshots omit symbol
// fields (true fail-open). When knownAlignment is non-nil it is used
// instead of calling RegimeStackProvider (watchlist already has the stack).
// cache, when non-nil, dedupes work within a scheduler tick.
func (s *Scheduler) buildAlertContext(
	ctx context.Context,
	timeframe, symbol string,
	tape *mkt.Summary,
	knownAlignment *float64,
	cache *alertBuildCache,
) AlertContext {
	key := contextCacheKey(timeframe, symbol)
	if cache != nil {
		if cached, ok := cache.contexts[key]; ok {
			return cached
		}
	}

	out := AlertContext{}
	resolved := s.resolveTape(ctx, timeframe, tape, cache)
	if resolved != nil {
		out.TapeRegime = string(resolved.State)
		out.TapeBias = resolved.Bias
		out.TapeConfidence = floatPtr(resolved.Confidence)
	}

	if symbol == "" {
		if cache != nil {
			cache.contexts[key] = out
		}
		return out
	}

	if s.evalStore != nil && timeframe != "" {
		snap, at, err := s.evalStore.GetSymbol(ctx, timeframe, symbol)
		if err == nil {
			applySnapshotToContext(&out, snap, at, s.now(), timeframe)
		}
	}

	if knownAlignment != nil {
		out.Alignment = floatPtr(*knownAlignment)
	} else if s.regimes != nil {
		if stack, err := s.regimes.Calculate(ctx, symbol); err == nil {
			out.Alignment = floatPtr(stack.Alignment)
		}
	}

	if cache != nil {
		cache.contexts[key] = out
	}
	return out
}

func (s *Scheduler) resolveTape(
	ctx context.Context,
	timeframe string,
	tape *mkt.Summary,
	cache *alertBuildCache,
) *mkt.Summary {
	if tape != nil {
		return tape
	}
	if timeframe == "" || s.market == nil {
		return nil
	}
	if cache != nil {
		if cached, ok := cache.tapes[timeframe]; ok {
			return cached
		}
	}
	sum, err := s.market.Calculate(ctx, timeframe)
	if err != nil {
		return nil
	}
	resolved := &sum
	if cache != nil {
		cache.tapes[timeframe] = resolved
	}
	return resolved
}

// applySnapshotToContext copies score/RS/sparkline only when the snap is
// current algo and fresh for the timeframe. Otherwise leaves symbol fields
// unset so a dead Put is never presented as live context.
func applySnapshotToContext(
	out *AlertContext,
	snap domain.EvaluationSnapshot,
	at time.Time,
	now time.Time,
	timeframe string,
) {
	if snap.AlgoVersion != domain.AlgoVersion {
		return
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return
	}
	if !domain.EvaluationStoreFresh(at, now, tf) {
		return
	}
	if snap.TotalScore != nil {
		out.SymbolScore = floatPtr(*snap.TotalScore)
	}
	if snap.RelativeStrength != nil {
		out.RS = floatPtr(*snap.RelativeStrength)
	}
	if len(snap.Sparkline) > 0 {
		out.Sparkline = DownsampleSparkline(snap.Sparkline, AlertSparklinePoints)
	}
}
