package scoring

import (
	"math"
	"sort"

	"pano_chart/backend/domain"
)

const (
	compressionPercentileWindow  = 500
	compressionPercentileMinBars = 200
	compressionBWLookback        = 20
	compressionATRPeriod         = 14
	compressionSqueezeCap        = 20
)

// CompressionPercentileScoreCalculator scores compression relative to the
// symbol's own history (PR-105). Name is "Compression Pct"; rankings still
// store the value under the "Compression" key when this algo is selected.
type CompressionPercentileScoreCalculator struct {
	// Legacy is used when fewer than compressionPercentileMinBars are available.
	Legacy CompressionConfig
}

func (c *CompressionPercentileScoreCalculator) Name() string {
	return "Compression Pct"
}

// WindowHint requests a longer candle fetch for percentile history.
func (c *CompressionPercentileScoreCalculator) WindowHint() int {
	return compressionPercentileWindow
}

func (c *CompressionPercentileScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	n := series.Len()
	if n < compressionPercentileMinBars {
		legacy := c.Legacy
		if legacy.CandleCount == 0 {
			legacy = DefaultCompressionConfig()
		}
		abs := &CompressionScoreCalculator{Config: legacy}
		return abs.Score(series)
	}

	start := 0
	length := n
	if length > compressionPercentileWindow {
		start = length - compressionPercentileWindow
		length = compressionPercentileWindow
	}
	candles := make([]domain.Candle, length)
	for i := 0; i < length; i++ {
		cd, err := series.At(start + i)
		if err != nil {
			return 0, err
		}
		candles[i] = cd
	}
	return DetectCompressionPercentile(candles), nil
}

// DetectCompressionPercentile computes the PR-105 percentile compression score
// on the given window (caller trims to ≤500). Returns 0 when inputs are unusable.
func DetectCompressionPercentile(candles []domain.Candle) float64 {
	n := len(candles)
	if n < compressionPercentileMinBars {
		return 0
	}

	bw := make([]float64, n)
	bwOK := make([]bool, n)
	for i := compressionBWLookback - 1; i < n; i++ {
		maxH := candles[i-compressionBWLookback+1].High()
		minL := candles[i-compressionBWLookback+1].Low()
		for j := i - compressionBWLookback + 2; j <= i; j++ {
			if candles[j].High() > maxH {
				maxH = candles[j].High()
			}
			if candles[j].Low() < minL {
				minL = candles[j].Low()
			}
		}
		close := candles[i].Close()
		if !finitePositive(close) {
			continue
		}
		w := (maxH - minL) / close
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			continue
		}
		bw[i] = w
		bwOK[i] = true
	}

	atrAligned := make([]float64, n)
	atrOK := make([]bool, n)
	atrSeries := rollingATR(candles, compressionATRPeriod)
	for i, v := range atrSeries {
		idx := i + compressionATRPeriod
		if idx >= n || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		atrAligned[idx] = v
		atrOK[idx] = true
	}

	last := n - 1
	if !bwOK[last] {
		return 0
	}

	bwHist := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if bwOK[i] {
			bwHist = append(bwHist, bw[i])
		}
	}
	atrHist := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if atrOK[i] {
			atrHist = append(atrHist, atrAligned[i])
		}
	}

	pBW := percentileRank(bwHist, bw[last])
	pATR := 0.5
	if atrOK[last] && len(atrHist) > 0 {
		pATR = percentileRank(atrHist, atrAligned[last])
	}

	p20 := empiricalPercentile(bwHist, 0.20)
	squeeze := 0
	for i := last; i >= 0 && squeeze < compressionSqueezeCap; i-- {
		if !bwOK[i] || bw[i] >= p20 {
			break
		}
		squeeze++
	}
	squeezeNorm := float64(squeeze) / float64(compressionSqueezeCap)

	score := (1-pBW)*0.5 + (1-pATR)*0.3 + squeezeNorm*0.2
	return clamp01(score)
}

// percentileRank is the fraction of hist values strictly below x (0 = tightest).
func percentileRank(hist []float64, x float64) float64 {
	if len(hist) == 0 {
		return 0.5
	}
	var below int
	for _, v := range hist {
		if v < x {
			below++
		}
	}
	return float64(below) / float64(len(hist))
}

// empiricalPercentile returns the p-quantile of hist (p in [0,1]) via sorted linear
// interpolation. Empty hist → +Inf so squeeze comparisons never match.
func empiricalPercentile(hist []float64, p float64) float64 {
	if len(hist) == 0 {
		return math.Inf(1)
	}
	if p <= 0 {
		min := hist[0]
		for _, v := range hist[1:] {
			if v < min {
				min = v
			}
		}
		return min
	}
	if p >= 1 {
		max := hist[0]
		for _, v := range hist[1:] {
			if v > max {
				max = v
			}
		}
		return max
	}
	sorted := append([]float64(nil), hist...)
	sort.Float64s(sorted)
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	w := pos - float64(lo)
	return sorted[lo]*(1-w) + sorted[hi]*w
}
