package risk

// imbalance scores how skewed the long/short ratio is.
// longRatio = 0.5 is neutral; deviation in either direction increases fragility.
func imbalance(longRatio float64) float64 {
	diff := longRatio - 0.5
	if diff < 0 {
		diff = -diff
	}
	score := diff * 2 // normalise to 0–1
	if score > 1 {
		return 1
	}
	return score
}
