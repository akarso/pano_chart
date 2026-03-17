package risk

// fundingExtremeness scores how extreme the current funding rate is.
// Higher absolute funding indicates a crowded side and increased fragility.
// Normalised assuming ~0.01 (1%) is extremely high.
func fundingExtremeness(funding float64) float64 {
	abs := funding
	if abs < 0 {
		abs = -abs
	}
	score := abs / 0.01
	if score > 1 {
		score = 1
	}
	return score
}
