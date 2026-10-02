package scoring

// IsPivotHigh reports whether vals[i] is a strict local maximum over the
// inclusive window [i−w, i+w] (strictly greater than every neighbor).
func IsPivotHigh(vals []float64, i, w int) bool {
	if w < 0 || i < w || i+w >= len(vals) {
		return false
	}
	v := vals[i]
	for j := i - w; j <= i+w; j++ {
		if j == i {
			continue
		}
		if vals[j] >= v {
			return false
		}
	}
	return true
}

// IsPivotLow reports whether vals[i] is a strict local minimum over the
// inclusive window [i−w, i+w] (strictly less than every neighbor).
func IsPivotLow(vals []float64, i, w int) bool {
	if w < 0 || i < w || i+w >= len(vals) {
		return false
	}
	v := vals[i]
	for j := i - w; j <= i+w; j++ {
		if j == i {
			continue
		}
		if vals[j] <= v {
			return false
		}
	}
	return true
}
