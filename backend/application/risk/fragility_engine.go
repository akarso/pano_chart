package risk

import domainrisk "pano_chart/backend/domain/risk"

// Engine computes fragility components and combines them into a final score.
type Engine struct{}

// NewEngine constructs the engine.
func NewEngine() *Engine { return &Engine{} }

// Calculate computes the individual fragility components from raw market data.
func (e *Engine) Calculate(
	funding float64,
	oiSeries []float64,
	longRatio float64,
	price float64,
	nearestCluster float64,
) domainrisk.FragilityComponents {
	return domainrisk.FragilityComponents{
		FundingExtremeness:   fundingExtremeness(funding),
		OIExpansion:          oiExpansion(oiSeries),
		LongShortImbalance:   imbalance(longRatio),
		LiquidationProximity: liquidationProximity(price, nearestCluster),
	}
}

// FinalScore computes the weighted composite score from components.
// Weights reflect reliability: OI (positioning) > funding/liquidation (sentiment/trigger) > imbalance (skew).
func FinalScore(c domainrisk.FragilityComponents) float64 {
	score := c.FundingExtremeness*0.25 +
		c.OIExpansion*0.30 +
		c.LongShortImbalance*0.20 +
		c.LiquidationProximity*0.25
	if score > 1 {
		score = 1
	}
	return score
}

// RiskLevel maps a numeric score to a human-readable risk category.
func RiskLevel(score float64) string {
	switch {
	case score > 0.7:
		return "high"
	case score > 0.4:
		return "medium"
	default:
		return "low"
	}
}

// DominantSide determines which side of the market is crowded.
// Positive funding + high long ratio → "long"; negative funding + low long ratio → "short".
func DominantSide(funding, longRatio float64) string {
	if funding > 0 && longRatio > 0.6 {
		return "long"
	}
	if funding < 0 && longRatio < 0.4 {
		return "short"
	}
	return "neutral"
}

// SqueezeRisk derives the likely squeeze direction from the dominant side.
// A crowded long market faces long_squeeze risk; crowded short faces short_squeeze.
func SqueezeRisk(dominantSide string) string {
	switch dominantSide {
	case "long":
		return "long_squeeze"
	case "short":
		return "short_squeeze"
	default:
		return "none"
	}
}
