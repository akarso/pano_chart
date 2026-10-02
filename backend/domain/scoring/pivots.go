package scoring

// IsPivotHigh reports whether vals[i] is a strict local maximum over the
// inclusive window [i−w, i+w] (strictly greater than every neighbor).
// Used by trend-strength swing structure (ROADMAP: h_i > h_{i±1..w}).
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

// IsPivotHighAllowEqual is like IsPivotHigh but treats equal neighbor highs as
// still a local max (plateau-tolerant). Matches Sideways V5 extrema, which
// accept equal touches within the pivot window.
func IsPivotHighAllowEqual(vals []float64, i, w int) bool {
	if w < 0 || i < w || i+w >= len(vals) {
		return false
	}
	v := vals[i]
	for j := i - w; j <= i+w; j++ {
		if j == i {
			continue
		}
		if vals[j] > v {
			return false
		}
	}
	return true
}

// IsPivotLowAllowEqual is like IsPivotLow but treats equal neighbor lows as
// still a local min (plateau-tolerant).
func IsPivotLowAllowEqual(vals []float64, i, w int) bool {
	if w < 0 || i < w || i+w >= len(vals) {
		return false
	}
	v := vals[i]
	for j := i - w; j <= i+w; j++ {
		if j == i {
			continue
		}
		if vals[j] < v {
			return false
		}
	}
	return true
}
