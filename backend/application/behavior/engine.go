package behavior

import (
	domainbehavior "pano_chart/backend/domain/behavior"
)

// BehaviorContext holds the input signals needed to compute behavioral
// dimensions.  All values are expected in [0,1] except Regime (string).
type BehaviorContext struct {
	FragilityScore     float64
	FundingExtremeness float64
	OIExpansion        float64
	Imbalance          float64

	Regime string

	VolumeScore float64
	Volatility  float64
}

// Engine computes retail behavioral dimensions from market context.
type Engine struct{}

// NewEngine constructs the behavior engine.
func NewEngine() *Engine { return &Engine{} }

// Evaluate computes all behavioral dimensions and returns the result.
func (e *Engine) Evaluate(ctx BehaviorContext) domainbehavior.RetailBehavior {
	g := greed(ctx)
	f := fear(ctx)
	p := patience(ctx)
	pa := panicScore(ctx)

	// Soft-normalize to avoid nonsense states (e.g. high greed + high fear).
	g, f, p, pa = normalize(g, f, p, pa)

	return domainbehavior.RetailBehavior{
		Greed:    g,
		Fear:     f,
		Patience: p,
		Panic:    pa,
		Summary:  Summarize(g, f, p, pa),
	}
}

// greed is driven by funding extremeness, long/short imbalance, and OI expansion.
func greed(ctx BehaviorContext) float64 {
	score := ctx.FundingExtremeness*0.4 +
		ctx.Imbalance*0.4 +
		ctx.OIExpansion*0.2
	return clamp(score)
}

// fear is driven by fragility and volatility.
func fear(ctx BehaviorContext) float64 {
	score := ctx.FragilityScore*0.6 +
		ctx.Volatility*0.4
	return clamp(score)
}

// patience is high when volatility is low, volume calm, and regime is compression.
func patience(ctx BehaviorContext) float64 {
	compressionBoost := 0.0
	if ctx.Regime == "compression" {
		compressionBoost = 0.3
	}
	score := (1-ctx.Volatility)*0.5 +
		(1-ctx.VolumeScore)*0.2 +
		compressionBoost
	return clamp(score)
}

// panicScore is driven by fragility and volatility spikes.
func panicScore(ctx BehaviorContext) float64 {
	score := ctx.FragilityScore*0.5 +
		ctx.Volatility*0.5
	return clamp(score)
}

// normalize soft-caps the total of all dimensions to 1.5 to keep behavior realistic.
func normalize(g, f, p, pa float64) (float64, float64, float64, float64) {
	total := g + f + p + pa
	if total > 1.5 {
		scale := 1.5 / total
		g *= scale
		f *= scale
		p *= scale
		pa *= scale
	}
	return g, f, p, pa
}

// Summarize produces a human-readable summary from the four dimensions.
func Summarize(g, f, p, pa float64) string {
	if pa > 0.7 {
		return "Panic rising"
	}
	if g > 0.7 {
		return "Greed dominant"
	}
	if p > 0.6 {
		return "Market waiting / coiling"
	}
	if f > 0.6 {
		return "Fear elevated"
	}
	return "Neutral sentiment"
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
