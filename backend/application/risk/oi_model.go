package risk

// oiExpansion detects rapid open interest build-up over the last 10 data points.
// Fast positioning accumulation increases fragility risk.
func oiExpansion(oiSeries []float64) float64 {
	if len(oiSeries) < 10 {
		return 0
	}
	start := oiSeries[len(oiSeries)-10]
	end := oiSeries[len(oiSeries)-1]
	if start == 0 {
		return 0
	}
	growth := (end - start) / start
	if growth < 0 {
		return 0
	}
	if growth > 1 {
		return 1
	}
	return growth
}
