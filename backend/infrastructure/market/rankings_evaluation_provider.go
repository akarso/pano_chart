package market

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/sync/singleflight"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RankingsEvaluationProvider adapts RankingsUseCase to EvaluationProvider,
// preferring the evaluation store when fresh (PR-089b).
type RankingsEvaluationProvider struct {
	rankings usecases.RankingsUseCase
	store    ports.EvaluationStore // optional; nil → always compute
	now      func() time.Time
	sf       singleflight.Group
}

// NewRankingsEvaluationProvider constructs the adapter.
func NewRankingsEvaluationProvider(r usecases.RankingsUseCase) *RankingsEvaluationProvider {
	return &RankingsEvaluationProvider{
		rankings: r,
		now:      time.Now,
	}
}

// SetStore attaches the evaluation store (optional).
func (p *RankingsEvaluationProvider) SetStore(store ports.EvaluationStore) {
	p.store = store
}

// SetNow overrides the clock (tests).
func (p *RankingsEvaluationProvider) SetNow(fn func() time.Time) {
	if fn != nil {
		p.now = fn
	}
}

// GetLatestEvaluations implements market.EvaluationProvider.
// Fresh non-empty store hit (matching AlgoVersion) → return store data.
// Miss / stale / empty / algo mismatch → rankings fallback (singleflight).
// Redis transport errors fail closed as ports.ErrEvaluationStoreUnavailable.
func (p *RankingsEvaluationProvider) GetLatestEvaluations(ctx context.Context, timeframe string) ([]domain.EvaluationSnapshot, error) {
	if p == nil || p.rankings == nil {
		return nil, errors.New("rankings evaluation provider not configured")
	}
	nowFn := p.now
	if nowFn == nil {
		nowFn = time.Now
	}

	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return nil, err
	}
	tfKey := tf.String()
	if p.store != nil {
		evals, hit, err := p.readStore(ctx, tf, tfKey, nowFn)
		if err != nil {
			return nil, err
		}
		if hit {
			return evals, nil
		}
	}
	return p.computeFromRankings(ctx, tf, tfKey)
}

// readStore returns (evals, true, nil) on a fresh usable hit; (nil, false, nil)
// on miss/stale/empty/algo; and a wrapped ErrEvaluationStoreUnavailable on
// Redis transport failure.
func (p *RankingsEvaluationProvider) readStore(ctx context.Context, tf domain.Timeframe, timeframe string, nowFn func() time.Time) ([]domain.EvaluationSnapshot, bool, error) {
	evals, at, err := p.store.Get(ctx, timeframe)
	if err != nil {
		if errors.Is(err, ports.ErrEvaluationNotFound) {
			log.Printf("[eval] provider reason=miss tf=%s", timeframe)
			return nil, false, nil
		}
		log.Printf("[eval] provider reason=transport tf=%s err=%v", timeframe, err)
		return nil, false, fmt.Errorf("%w: %v", ports.ErrEvaluationStoreUnavailable, err)
	}
	if len(evals) == 0 {
		// Empty fresh Put must not poison Market Pulse into
		// DataQualityUnavailable — treat as miss (reader invariant).
		log.Printf("[eval] provider reason=empty tf=%s", timeframe)
		return nil, false, nil
	}
	if !algoVersionOK(evals) {
		log.Printf("[eval] provider reason=algo tf=%s", timeframe)
		return nil, false, nil
	}
	age := nowFn().Sub(at)
	if age > domain.EvaluationStaleAfter(tf) {
		log.Printf("[eval] provider reason=stale tf=%s age=%s", timeframe, age)
		return nil, false, nil
	}
	// Hits are silent at info level — notification ticks / Market Pulse
	// would otherwise spam stdout in steady state (miss/stale/… stay loud).
	return evals, true, nil
}

func algoVersionOK(evals []domain.EvaluationSnapshot) bool {
	for _, e := range evals {
		if e.AlgoVersion != domain.AlgoVersion {
			return false
		}
	}
	return true
}

func (p *RankingsEvaluationProvider) computeFromRankings(ctx context.Context, tf domain.Timeframe, timeframe string) ([]domain.EvaluationSnapshot, error) {
	// Coalesce concurrent miss/stale fallbacks for the same TF so a cliff
	// at EvaluationStaleAfter (or cold store) does not stampede rankings.
	// DoChan + select: cancelled callers return immediately while the shared
	// flight continues (same pattern as RedisCachedComposite.CalculateTape).
	ch := p.sf.DoChan(timeframe, func() (interface{}, error) {
		workCtx := context.WithoutCancel(ctx)
		results, err := p.rankings.Execute(workCtx, usecases.GetRankingsRequest{
			Timeframe: tf,
			Sort:      usecases.SortByTotal,
		})
		if err != nil {
			return nil, err
		}
		return appmarket.SnapshotsFromRankings(results, timeframe, time.Time{}), nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		evals, _ := res.Val.([]domain.EvaluationSnapshot)
		// Deep-copy structs + Sparkline backing arrays so waiters do not
		// share mutable slice memory with the shared flight result.
		return cloneEvaluationSnapshots(evals), nil
	}
}

func cloneEvaluationSnapshots(evals []domain.EvaluationSnapshot) []domain.EvaluationSnapshot {
	out := make([]domain.EvaluationSnapshot, len(evals))
	for i, e := range evals {
		out[i] = e
		if e.Sparkline != nil {
			out[i].Sparkline = append([]float64(nil), e.Sparkline...)
		}
	}
	return out
}
