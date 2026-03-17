package setups

import "pano_chart/backend/domain/setup"

// SetupEvaluator scores a single setup strategy.
type SetupEvaluator interface {
	Type() setup.SetupType
	Score(ctx SetupContext) float64
}
