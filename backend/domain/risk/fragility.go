package risk

// FragilityComponents holds the individual risk sub-scores.
type FragilityComponents struct {
	FundingExtremeness   float64
	OIExpansion          float64
	LongShortImbalance   float64
	LiquidationProximity float64
}

// Fragility represents the overall position crowding / fragility assessment.
type Fragility struct {
	Symbol       string
	Timeframe    string
	Score        float64
	RiskLevel    string
	DominantSide string
	SqueezeRisk  string
	Components   FragilityComponents
}
