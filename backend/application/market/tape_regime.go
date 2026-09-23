package market

import (
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/scoring"
)

const (
	// tapeTrendGate is the exclusive path to State=trend (PR-115).
	tapeTrendGate = 0.5
	// crashTailBars is the lookback for the adverse-move penalty.
	crashTailBars = 8
)

// TapeRegime is the market regime derived by scoring a merged composite
// candle series. Trend state requires TapeTrend ≥ tapeTrendGate; structure
// remains the measured four-way mix with no health redistribution.
type TapeRegime struct {
	Structure      mkt.Breadth
	State          mkt.State
	Confidence     float64
	Bias           string
	TrendScore     float64 // raw TapeTrend score
	EffectiveTrend float64
	BreakdownRate  float64
	Label          string
	Source         string
	WindowBars     int
}

// ScoreMarketTape classifies a composite OHLCV series.
func ScoreMarketTape(series domain.CandleSeries, timeframe, source string) TapeRegime {
	empty := TapeRegime{
		State:      mkt.StateSideways,
		Bias:       "neutral",
		Label:      BuildMarketLabel(0, 0),
		Source:     source,
		Confidence: 0,
		WindowBars: series.Len(),
	}
	if series.Len() < 2 {
		return empty
	}

	closes := closesFromSeries(series)
	candles := series.All()
	atr14 := scoring.TrueATR(candles, 14)
	trendScore, trendBias := scoring.TapeTrend(closes, atr14)

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

	price := closes[len(closes)-1]
	hi, lo, extremeBars := ohlcWindowExtreme(candles, trendBias == "down")
	recentReturn := tailReturnATR(closes, atr14, crashTailBars)

	var effectiveTrend, breakdownRate float64
	// Health describes a TREND headline only — not a weak TapeTrend that
	// lost the exclusive gate.
	if trendScore >= tapeTrendGate && atr14 > 0 && (trendBias == "up" || trendBias == "down") {
		stateDir := "uptrend"
		if trendBias == "down" {
			stateDir = "downtrend"
		}
		effectiveTrend = ComputeTrendHealthV2(stateDir, price, hi, lo, atr14, recentReturn, extremeBars)
		if effectiveTrend < 0.4 {
			breakdownRate = 1
		}
	}

	tape := classifyTape(trendScore, trendBias, sideways, compression, expansion, effectiveTrend, breakdownRate)
	tape.Source = source
	tape.WindowBars = series.Len()
	return tape
}

// classifyTape turns raw calculator scores into a TapeRegime. Exported for
// tests that need coexistence / weak-grind fixtures without fighting the
// real Sideways/Compression calculators.
func classifyTape(
	trendScore float64,
	trendBias string,
	sideways, compression, expansion float64,
	effectiveTrend, breakdownRate float64,
) TapeRegime {
	trend := trendScore
	total := trend + sideways + compression + expansion
	var structure mkt.Breadth
	if total < 0.05 {
		structure = mkt.Breadth{Sideways: 1}
	} else {
		structure = mkt.Breadth{
			Trend:       trend / total,
			Sideways:    sideways / total,
			Compression: compression / total,
			Expansion:   expansion / total,
		}
	}

	bias := trendBias
	if bias == "" {
		bias = "neutral"
	}

	var dominant mkt.State
	var confidence float64
	if trendScore >= tapeTrendGate {
		dominant = mkt.StateTrend
		confidence = trendScore
	} else {
		dominant, confidence = dominantNonTrend(structure)
		first, second := topTwo([]float64{
			structure.Sideways, structure.Compression, structure.Expansion,
		})
		if first < 0.50 || (first-second) < 0.30 {
			dominant = mkt.StateIndecisive
			confidence = first
		}
	}

	return TapeRegime{
		Structure:      structure,
		State:          dominant,
		Confidence:     confidence,
		Bias:           bias,
		TrendScore:     trendScore,
		EffectiveTrend: effectiveTrend,
		BreakdownRate:  breakdownRate,
		Label:          BuildTapeLabel(dominant, effectiveTrend),
	}
}

// dominantNonTrend picks among sideways / compression / expansion only.
func dominantNonTrend(b mkt.Breadth) (mkt.State, float64) {
	dominant := mkt.StateSideways
	maxWeight := b.Sideways
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

// ohlcWindowExtreme returns the candle high/low extreme and bars since it.
// lookForLow selects the window low (downtrends); otherwise the window high.
func ohlcWindowExtreme(candles []domain.Candle, lookForLow bool) (hi, lo float64, barsSince int) {
	n := len(candles)
	if n == 0 {
		return 0, 0, 0
	}
	hi, lo = candles[0].High(), candles[0].Low()
	extremeIdx := 0
	for i := 1; i < n; i++ {
		if candles[i].High() > hi {
			hi = candles[i].High()
		}
		if candles[i].Low() < lo {
			lo = candles[i].Low()
		}
		if lookForLow {
			if candles[i].Low() <= candles[extremeIdx].Low() {
				extremeIdx = i
			}
		} else if candles[i].High() >= candles[extremeIdx].High() {
			extremeIdx = i
		}
	}
	return hi, lo, n - 1 - extremeIdx
}

// tailReturnATR is (last − first_of_tail) / atr over the last n bars.
func tailReturnATR(closes []float64, atr float64, n int) float64 {
	if atr <= 0 || len(closes) < 2 {
		return 0
	}
	if n < 2 {
		n = 2
	}
	if n > len(closes) {
		n = len(closes)
	}
	start := len(closes) - n
	return (closes[len(closes)-1] - closes[start]) / atr
}
