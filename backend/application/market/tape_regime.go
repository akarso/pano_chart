package market

import (
	"math"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/scoring"
)

// TapeRegime is the market regime derived by scoring a merged composite
// candle series with the same calculators used on individual charts (PR-084).
type TapeRegime struct {
	Structure      mkt.Breadth
	State          mkt.State
	Confidence     float64
	Bias           string
	EffectiveTrend float64
	BreakdownRate  float64
	Label          string
	Source         string
}

// ScoreMarketTape classifies a composite OHLCV series the same way a single
// symbol is scored on the rankings page, then picks a dominant regime.
func ScoreMarketTape(series domain.CandleSeries, timeframe, source string) TapeRegime {
	empty := TapeRegime{
		State:      mkt.StateSideways,
		Bias:       "neutral",
		Label:      BuildMarketLabel(0, 0),
		Source:     source,
		Confidence: 0,
	}
	if series.Len() < 2 {
		return empty
	}

	trendCalc := &scoring.TrendPredictabilityScoreCalculator{}
	trendScore, trendBias, err := trendCalc.ScoreWithDirection(series)
	if err != nil {
		trendScore, trendBias = 0, "neutral"
	}
	trend := math.Abs(trendScore)

	sidewaysCalc := &scoring.SidewaysV5ScoreCalculator{
		Config: scoring.NewSidewaysV5ConfigForTimeframe(timeframe),
	}
	sideways, err := sidewaysCalc.Score(series)
	if err != nil {
		sideways = 0
	}

	compressionCalc := &scoring.CompressionScoreCalculator{
		Config: scoring.DefaultCompressionConfig(),
	}
	compression, err := compressionCalc.Score(series)
	if err != nil {
		compression = 0
	}

	breakoutCalc := &scoring.BreakoutScoreCalculator{
		Config:           scoring.DefaultBreakoutConfig(),
		CompressionScore: compression,
	}
	expansion, err := breakoutCalc.Score(series)
	if err != nil {
		expansion = 0
	}

	total := trend + sideways + compression + expansion
	var structure mkt.Breadth
	if total <= 0 {
		structure = mkt.Breadth{Sideways: 1}
	} else {
		structure = mkt.Breadth{
			Trend:       trend / total,
			Sideways:    sideways / total,
			Compression: compression / total,
			Expansion:   expansion / total,
		}
	}

	// Composite-level health (one tape, not diluted across 150 symbols).
	closes := closesFromSeries(series)
	price, hi, lo, atr, recentReturn := sparklineStats(closes)
	stateDir := "uptrend"
	if trendBias == "down" {
		stateDir = "downtrend"
	} else if trendBias != "up" {
		// Flat/neutral trend score — health not meaningful; leave undamped.
		stateDir = ""
	}
	var effectiveTrend, breakdownRate float64
	if stateDir != "" && atr > 0 {
		h := ComputeTrendHealth(stateDir, price, hi, lo, atr, recentReturn)
		effectiveTrend = h
		if h < 0.4 {
			breakdownRate = 1
		}
		structure = DampenTrendByHealth(structure, effectiveTrend, breakdownRate)
	}

	dominant, confidence := dominantFromBreadth(structure)

	// Indecisive when no clear winner on the tape itself.
	first, second := topTwo([]float64{
		structure.Sideways, structure.Compression, structure.Expansion, structure.Trend,
	})
	if first < 0.50 || (first-second) < 0.30 {
		dominant = mkt.StateIndecisive
		confidence = first
	}

	bias := trendBias
	if bias == "" {
		bias = "neutral"
	}
	// Validate bias against net tape move.
	if len(closes) >= 2 {
		net := closes[len(closes)-1] - closes[0]
		if bias == "up" && net < 0 {
			bias = "neutral"
		} else if bias == "down" && net > 0 {
			bias = "neutral"
		}
	}

	return TapeRegime{
		Structure:      structure,
		State:          dominant,
		Confidence:     confidence,
		Bias:           bias,
		EffectiveTrend: effectiveTrend,
		BreakdownRate:  breakdownRate,
		Label:          BuildMarketLabel(structure.Trend, effectiveTrend),
		Source:         source,
	}
}

func dominantFromBreadth(b mkt.Breadth) (mkt.State, float64) {
	dominant := mkt.StateSideways
	maxWeight := b.Sideways
	if b.Trend >= maxWeight {
		dominant = mkt.StateTrend
		maxWeight = b.Trend
	}
	if b.Compression >= maxWeight {
		dominant = mkt.StateCompression
		maxWeight = b.Compression
	}
	if b.Expansion >= maxWeight {
		dominant = mkt.StateExpansion
		maxWeight = b.Expansion
	}
	return dominant, maxWeight
}

func closesFromSeries(series domain.CandleSeries) []float64 {
	n := series.Len()
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		c, err := series.At(i)
		if err != nil {
			continue
		}
		out[i] = c.Close()
	}
	return out
}

func sparklineStats(closes []float64) (price, hi, lo, atr, recentReturn float64) {
	n := len(closes)
	if n == 0 {
		return
	}
	price = closes[n-1]
	hi, lo = closes[0], closes[0]
	var atrSum float64
	for i := 0; i < n; i++ {
		if closes[i] > hi {
			hi = closes[i]
		}
		if closes[i] < lo {
			lo = closes[i]
		}
		if i > 0 {
			atrSum += math.Abs(closes[i] - closes[i-1])
		}
	}
	if n > 1 {
		atr = atrSum / float64(n-1)
		if atr > 0 {
			recentReturn = (closes[n-1] - closes[0]) / atr
		}
	}
	return
}
