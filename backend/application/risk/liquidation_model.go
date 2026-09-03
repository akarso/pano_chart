package risk

// liquidationProximity scores how close the current price is to the nearest
// liquidation cluster. Closer clusters → higher fragility.
func liquidationProximity(price, nearestCluster float64) float64 {
	if price == 0 {
		return 0
	}
	distance := nearestCluster - price
	if distance < 0 {
		distance = -distance
	}
	ratio := distance / price
	score := 1 - ratio*10
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
