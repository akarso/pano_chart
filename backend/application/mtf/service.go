// Package mtf builds a symbol's multi-timeframe regime stack (PR-099) from
// the shared evaluation store — no candle fetch or rescoring, so it is cheap
// enough to call per rankings row.
package mtf

import (
	"context"
	"errors"
	"time"

	appeval "pano_chart/backend/application/evaluation"
	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// alignmentFloor is the minimum share of agreeing frames for AlignedState to
// report anything other than indecisive.
const alignmentFloor = 0.75

// structureStates fixes the iteration order used to break ties
// deterministically when picking the most-common dominant state (map
// iteration order in Go is randomized).
var structureStates = []mkt.State{
	mkt.StateTrend, mkt.StateSideways, mkt.StateCompression, mkt.StateExpansion,
}

// TFRegime is one timeframe's four-way structure and dominant regime.
type TFRegime struct {
	Timeframe string
	Structure mkt.Breadth
	Dominant  mkt.State
	Bias      string
	Score     float64
}

// Stack is a symbol's regime reading across appeval.DefaultTimeframes, plus
// how well those frames agree.
type Stack struct {
	Symbol       string
	Frames       []TFRegime
	Alignment    float64
	AlignedState mkt.State
}

// Service builds a Stack from the shared evaluation store.
type Service struct {
	store ports.EvaluationStore
	now   func() time.Time
}

// NewService constructs the service.
func NewService(store ports.EvaluationStore) *Service {
	return &Service{store: store, now: time.Now}
}

// Calculate reads every timeframe's stored evaluation for symbol and derives
// the alignment stack. A frame whose store entry is missing, stale, or from
// a different scoring version is simply omitted — Alignment/AlignedState are
// computed over whatever frames remain (0/indecisive when none do). Only a
// context cancellation/deadline from the store aborts the whole call; a
// per-frame miss or transport error just skips that frame.
func (s *Service) Calculate(ctx context.Context, symbol string) (Stack, error) {
	now := s.now
	if now == nil {
		now = time.Now
	}

	frames := make([]TFRegime, 0, len(appeval.DefaultTimeframes))
	for _, tfStr := range appeval.DefaultTimeframes {
		tf, err := domain.NewTimeframe(tfStr)
		if err != nil {
			continue // DefaultTimeframes is a fixed valid literal; defensive only.
		}

		snap, at, err := s.store.GetSymbol(ctx, tfStr, symbol)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return Stack{}, err
			}
			continue // miss / transport error: skip this frame.
		}
		if !domain.EvaluationIdentityOK(snap.AlgoVersion, snap.TrendAlgo) {
			continue
		}
		if !domain.EvaluationStoreFresh(at, now(), tf) {
			continue
		}

		structure := appmarket.ScoreWeights(snap)
		dominant, score := mkt.DominantOf(structure)
		frames = append(frames, TFRegime{
			Timeframe: tfStr,
			Structure: structure,
			Dominant:  dominant,
			Bias:      snap.Bias,
			Score:     score,
		})
	}

	alignment, aligned := alignmentOf(frames)
	return Stack{
		Symbol:       symbol,
		Frames:       frames,
		Alignment:    alignment,
		AlignedState: aligned,
	}, nil
}

// alignmentOf returns the share of frames agreeing with the most common
// dominant state, and that state when it clears alignmentFloor (else
// indecisive).
//
// structureStates fixes a *different* priority order (trend first) than
// mkt.DominantOf's per-frame tie-break (sideways < trend <= compression <=
// expansion) — intentionally: this is breaking ties among *counts* of
// already-computed dominant states across frames, not among Breadth weights
// within one frame, so there is no shared rule to reuse. Either order is
// deterministic; only the alignment ratio is part of the contract, and it
// is identical regardless of which tied state "wins" the label.
func alignmentOf(frames []TFRegime) (float64, mkt.State) {
	if len(frames) == 0 {
		return 0, mkt.StateIndecisive
	}
	counts := make(map[mkt.State]int, len(structureStates))
	for _, f := range frames {
		counts[f.Dominant]++
	}
	var most mkt.State
	maxCount := 0
	for _, st := range structureStates {
		if counts[st] > maxCount {
			maxCount = counts[st]
			most = st
		}
	}
	alignment := float64(maxCount) / float64(len(frames))
	aligned := mkt.StateIndecisive
	if alignment >= alignmentFloor {
		aligned = most
	}
	return alignment, aligned
}
