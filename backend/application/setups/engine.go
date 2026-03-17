package setups

import "pano_chart/backend/domain/setup"

// Engine evaluates all registered setup strategies and picks the best one.
type Engine struct {
	evaluators []SetupEvaluator
}

// NewEngine constructs an engine with the default set of evaluators.
func NewEngine() *Engine {
	return &Engine{
		evaluators: []SetupEvaluator{
			CompressionSetup{},
			TrendSetup{},
			RangeSetup{},
		},
	}
}

// Evaluate scores every registered setup and returns the result including the
// best setup by highest score.
func (e *Engine) Evaluate(ctx SetupContext) setup.SetupScores {
	scores := make(map[setup.SetupType]float64, len(e.evaluators))

	bestScore := 0.0
	bestSetup := setup.SetupType("")

	for _, ev := range e.evaluators {
		score := ev.Score(ctx)
		scores[ev.Type()] = score

		if score > bestScore {
			bestScore = score
			bestSetup = ev.Type()
		}
	}

	return setup.SetupScores{
		Symbol:    ctx.Symbol,
		BestSetup: bestSetup,
		Score:     bestScore,
		Scores:    scores,
	}
}
