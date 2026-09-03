package scoring

import "math"

// DirectionAgreement returns how unanimously a set of directional
// observations agree. Returns 1.0 when all point the same way,
// 0.0 at a perfect 50/50 split. Flat observations (neither up nor
// down) are excluded from the count.
func DirectionAgreement(up, down int) float64 {
	total := up + down
	if total == 0 {
		return 0
	}
	return math.Abs(float64(up)-float64(down)) / float64(total)
}

// SeriesDirectionAgreement splits a close-price slice into equal
// segments and measures how consistently they trend in the same
// direction. Returns 1.0 when all segments agree, 0.0 at 50/50.
//
// With fewer than segments*2 data points the series is too short
// for meaningful sub-division and 1.0 (no penalty) is returned.
func SeriesDirectionAgreement(closes []float64, segments int) float64 {
	n := len(closes)
	if n < segments*2 || segments < 2 {
		return 1.0
	}
	segLen := n / segments
	var up, down int
	for i := 0; i < segments; i++ {
		start := i * segLen
		end := start + segLen - 1
		if i == segments-1 {
			end = n - 1 // last segment absorbs remainder
		}
		if closes[end] > closes[start] {
			up++
		} else if closes[end] < closes[start] {
			down++
		}
	}
	return DirectionAgreement(up, down)
}
