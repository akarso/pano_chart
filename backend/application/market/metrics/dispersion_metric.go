package metrics

// dispersion computes the mean absolute deviation of individual asset returns
// from the market return. Low dispersion means assets move together; high
// dispersion indicates rotation or divergence.
func dispersion(assetReturns []float64, marketReturn float64) float64 {
	if len(assetReturns) == 0 {
		return 0
	}

	sum := 0.0
	for _, r := range assetReturns {
		diff := r - marketReturn
		if diff < 0 {
			diff = -diff
		}
		sum += diff
	}

	return sum / float64(len(assetReturns))
}
