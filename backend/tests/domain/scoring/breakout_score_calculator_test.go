package scoring

import (
	"math"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"testing"
	"time"
)

// makeBreakoutCandles creates candles with explicit OHLCV data.
func makeBreakoutCandles(data [][5]float64) []domain.Candle {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := make([]domain.Candle, len(data))
	for i, d := range data {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf,
			time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC),
			d[0], d[1], d[2], d[3], d[4],
		)
	}
	return candles
}

func makeBreakoutSeries(data [][5]float64) domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := makeBreakoutCandles(data)
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

func defaultBreakoutCfg() scoring.BreakoutConfig {
	return scoring.BreakoutConfig{
		ATRPeriod:            14,
		VolumeLookback:       20,
		PenetrationNorm:      1.5,
		ATRNorm:              0.005,
		VolumeNorm:           2.0,
		ReentryLookback:      3,
		FailurePenalty:       0.3,
		CompressionThreshold: 0.3,
		SwingLookback:        3,
		CandleCount:          100,
		W1:                   1.0,
		W2:                   1.0,
		W3:                   0.5,
	}
}

func TestBreakout_TooFewCandles(t *testing.T) {
	data := make([][5]float64, 10)
	for i := range data {
		data[i] = [5]float64{100, 101, 99, 100, 100}
	}
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.UpScore != 0 || res.DownScore != 0 {
		t.Errorf("expected zero scores for too few candles, got up=%.4f down=%.4f", res.UpScore, res.DownScore)
	}
}

func cleanUpBreakout(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		mid := 100.0
		h := mid + 5
		l := mid - 5
		o := mid - 1
		c := mid + 1
		vol := 100.0
		if i%7 == 0 {
			h = 110
			c = 109
			o = mid
		}
		if i%7 == 3 {
			l = 90
			c = 91
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, vol}
	}
	data[n-1] = [5]float64{108, 118, 107, 117, 500}
	data[n-2] = [5]float64{104, 112, 103, 111, 300}
	return data
}

func TestBreakout_CleanUpBreakout_PositiveUpScore(t *testing.T) {
	data := cleanUpBreakout(60)
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.UpScore <= 0 {
		t.Errorf("expected positive up score, got %.4f", res.UpScore)
	}
	if res.DownScore > 0 {
		t.Errorf("expected zero down score, got %.4f", res.DownScore)
	}
	if res.BoundaryViolationUp <= 0 {
		t.Errorf("expected positive BVS up, got %.4f", res.BoundaryViolationUp)
	}
	if res.CloseConvictionUp <= 0 {
		t.Errorf("expected positive CCS up, got %.4f", res.CloseConvictionUp)
	}
}

func cleanDownBreakout(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		mid := 100.0
		h := mid + 5
		l := mid - 5
		o := mid + 1
		c := mid - 1
		vol := 100.0
		if i%7 == 0 {
			h = 110
			c = 109
			o = mid
		}
		if i%7 == 3 {
			l = 90
			c = 91
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, vol}
	}
	data[n-1] = [5]float64{92, 93, 82, 83, 500}
	data[n-2] = [5]float64{96, 97, 88, 89, 300}
	return data
}

func TestBreakout_CleanDownBreakout_PositiveDownScore(t *testing.T) {
	data := cleanDownBreakout(60)
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.DownScore <= 0 {
		t.Errorf("expected positive down score, got %.4f", res.DownScore)
	}
	if res.UpScore > 0 {
		t.Errorf("expected zero up score, got %.4f", res.UpScore)
	}
	if res.BoundaryViolationDown <= 0 {
		t.Errorf("expected positive BVS down, got %.4f", res.BoundaryViolationDown)
	}
	if res.CloseConvictionDown <= 0 {
		t.Errorf("expected positive CCS down, got %.4f", res.CloseConvictionDown)
	}
}

func insideRange(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		mid := 100.0
		h := mid + 3
		l := mid - 3
		o := mid - 0.5
		c := mid + 0.5
		if i%7 == 0 {
			h = 110
			c = 108
			o = mid
		}
		if i%7 == 3 {
			l = 90
			c = 92
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}
	return data
}

func TestBreakout_InsideRange_ZeroScores(t *testing.T) {
	data := insideRange(60)
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.UpScore > 0.05 {
		t.Errorf("expected near-zero up score, got %.4f", res.UpScore)
	}
	if res.DownScore > 0.05 {
		t.Errorf("expected near-zero down score, got %.4f", res.DownScore)
	}
}

func wickBreakout(n int) [][5]float64 {
	data := insideRange(n)
	data[n-1] = [5]float64{100, 118, 99, 100.5, 100}
	return data
}

func TestBreakout_WickBreakout_LowConviction(t *testing.T) {
	data := wickBreakout(60)
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.CloseConvictionUp > 0.1 {
		t.Errorf("expected low CCS for wick breakout, got %.4f", res.CloseConvictionUp)
	}
}

func TestBreakout_CompressionBoost(t *testing.T) {
	data := cleanUpBreakout(60)
	candles := makeBreakoutCandles(data)
	cfg := defaultBreakoutCfg()
	resNoBoost := scoring.DetectBreakout(candles, cfg, 0)
	resWithBoost := scoring.DetectBreakout(candles, cfg, 0.8)
	if resWithBoost.UpScore <= resNoBoost.UpScore {
		t.Errorf("expected compression boost to increase score: without=%.4f with=%.4f",
			resNoBoost.UpScore, resWithBoost.UpScore)
	}
	if resWithBoost.CompressionBoost <= 1.0 {
		t.Errorf("expected compression boost > 1.0, got %.4f", resWithBoost.CompressionBoost)
	}
}

func TestBreakout_CompressionBelowThreshold_NoBoost(t *testing.T) {
	data := cleanUpBreakout(60)
	candles := makeBreakoutCandles(data)
	cfg := defaultBreakoutCfg()
	res := scoring.DetectBreakout(candles, cfg, 0.1)
	if res.CompressionBoost != 1.0 {
		t.Errorf("expected no boost when compression below threshold, got %.4f", res.CompressionBoost)
	}
}

func TestBreakout_VolumeExpansion_IncreasesScore(t *testing.T) {
	dataLow := cleanUpBreakout(60)
	dataLow[59] = [5]float64{108, 118, 107, 117, 50}
	candlesLow := makeBreakoutCandles(dataLow)

	dataHigh := cleanUpBreakout(60)
	candlesHigh := makeBreakoutCandles(dataHigh)

	cfg := defaultBreakoutCfg()
	resLow := scoring.DetectBreakout(candlesLow, cfg, 0)
	resHigh := scoring.DetectBreakout(candlesHigh, cfg, 0)
	if resHigh.VolumeScore <= resLow.VolumeScore {
		t.Errorf("expected higher VLS with more volume: low=%.4f high=%.4f",
			resLow.VolumeScore, resHigh.VolumeScore)
	}
}

func TestBreakout_ScoresNormalized(t *testing.T) {
	datasets := [][][5]float64{
		cleanUpBreakout(60),
		cleanDownBreakout(60),
		insideRange(60),
	}
	cfg := defaultBreakoutCfg()
	for i, data := range datasets {
		candles := makeBreakoutCandles(data)
		res := scoring.DetectBreakout(candles, cfg, 0)
		if res.UpScore < 0 || res.UpScore > 1 {
			t.Errorf("dataset %d: UpScore out of [0,1]: %.4f", i, res.UpScore)
		}
		if res.DownScore < 0 || res.DownScore > 1 {
			t.Errorf("dataset %d: DownScore out of [0,1]: %.4f", i, res.DownScore)
		}
		if res.BoundaryViolationUp < 0 || res.BoundaryViolationUp > 1 {
			t.Errorf("dataset %d: BVS up out of [0,1]: %.4f", i, res.BoundaryViolationUp)
		}
		if res.BoundaryViolationDown < 0 || res.BoundaryViolationDown > 1 {
			t.Errorf("dataset %d: BVS down out of [0,1]: %.4f", i, res.BoundaryViolationDown)
		}
		if res.VolatilityExpansion < 0 || res.VolatilityExpansion > 1 {
			t.Errorf("dataset %d: VES out of [0,1]: %.4f", i, res.VolatilityExpansion)
		}
		if res.VolumeScore < 0 || res.VolumeScore > 1 {
			t.Errorf("dataset %d: VLS out of [0,1]: %.4f", i, res.VolumeScore)
		}
	}
}

func reentryBreakout(n int) [][5]float64 {
	data := insideRange(n)
	data[n-3] = [5]float64{108, 118, 107, 115, 300}
	data[n-2] = [5]float64{105, 107, 100, 102, 200}
	data[n-1] = [5]float64{108, 118, 107, 117, 500}
	return data
}

func TestBreakout_ReentryPenalty_ReducesScore(t *testing.T) {
	dataClean := cleanUpBreakout(60)
	dataReentry := reentryBreakout(60)
	cfg := defaultBreakoutCfg()
	canClean := makeBreakoutCandles(dataClean)
	canReentry := makeBreakoutCandles(dataReentry)
	resClean := scoring.DetectBreakout(canClean, cfg, 0)
	resReentry := scoring.DetectBreakout(canReentry, cfg, 0)
	if resReentry.UpScore >= resClean.UpScore && resReentry.UpScore > 0 {
		t.Errorf("expected re-entry penalty to reduce score: clean=%.4f reentry=%.4f",
			resClean.UpScore, resReentry.UpScore)
	}
}

func TestBreakoutScoreCalculator_Name(t *testing.T) {
	calc := &scoring.BreakoutScoreCalculator{Config: defaultBreakoutCfg()}
	if calc.Name() != "Breakout" {
		t.Errorf("expected name 'Breakout', got '%s'", calc.Name())
	}
}

func TestBreakoutScoreCalculator_Score(t *testing.T) {
	data := cleanUpBreakout(60)
	series := makeBreakoutSeries(data)
	calc := &scoring.BreakoutScoreCalculator{Config: defaultBreakoutCfg()}
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0 || score > 1 {
		t.Errorf("score out of [0,1]: %.4f", score)
	}
	if score <= 0 {
		t.Errorf("expected positive score for breakout data, got %.4f", score)
	}
}

func TestBreakoutSubscore_Name(t *testing.T) {
	sub := scoring.BreakoutSubscore{}
	if sub.Name() != "Breakout" {
		t.Errorf("expected name 'Breakout', got '%s'", sub.Name())
	}
}

func TestBreakoutSubscore_Compute(t *testing.T) {
	data := cleanUpBreakout(60)
	candles := makeBreakoutCandles(data)
	sub := scoring.BreakoutSubscore{}
	res := sub.Compute(candles, defaultBreakoutCfg())
	if res.Value < 0 || res.Value > 1 {
		t.Errorf("subscore value out of [0,1]: %.4f", res.Value)
	}
	if res.Value <= 0 {
		t.Errorf("expected positive subscore, got %.4f", res.Value)
	}
	if res.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %.4f", res.Confidence)
	}
	for _, key := range []string{"upScore", "downScore", "bvsUp", "bvsDown", "ccsUp", "ccsDown", "ves", "vls", "compBoost"} {
		if _, ok := res.Meta[key]; !ok {
			t.Errorf("missing meta key: %s", key)
		}
	}
}

func TestBreakoutSubscore_InvalidData(t *testing.T) {
	sub := scoring.BreakoutSubscore{}
	res := sub.Compute("not candles", nil)
	if res.Value != 0 {
		t.Errorf("expected 0 for invalid data, got %.4f", res.Value)
	}
	if res.Meta["error"] != 1 {
		t.Errorf("expected error meta flag")
	}
}

func TestDefaultBreakoutConfig(t *testing.T) {
	cfg := scoring.DefaultBreakoutConfig()
	if cfg.ATRPeriod <= 0 {
		t.Errorf("ATRPeriod should be positive, got %d", cfg.ATRPeriod)
	}
	if cfg.PenetrationNorm <= 0 {
		t.Errorf("PenetrationNorm should be positive, got %.4f", cfg.PenetrationNorm)
	}
	if cfg.CandleCount <= 0 {
		t.Errorf("CandleCount should be positive, got %d", cfg.CandleCount)
	}
}

func TestBreakout_UpAndDown_NotBothHigh(t *testing.T) {
	dataUp := cleanUpBreakout(60)
	canUp := makeBreakoutCandles(dataUp)
	resUp := scoring.DetectBreakout(canUp, defaultBreakoutCfg(), 0)
	if resUp.UpScore > 0 && resUp.DownScore > 0 {
		t.Errorf("both up and down positive on up breakout: up=%.4f down=%.4f",
			resUp.UpScore, resUp.DownScore)
	}

	dataDown := cleanDownBreakout(60)
	canDown := makeBreakoutCandles(dataDown)
	resDown := scoring.DetectBreakout(canDown, defaultBreakoutCfg(), 0)
	if resDown.UpScore > 0 && resDown.DownScore > 0 {
		t.Errorf("both up and down positive on down breakout: up=%.4f down=%.4f",
			resDown.UpScore, resDown.DownScore)
	}
}

func flatATRData(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		data[i] = [5]float64{99, 101, 99, 101, 100}
		if i%7 == 0 {
			data[i] = [5]float64{98, 110, 90, 109, 100}
		}
		if i%7 == 3 {
			data[i] = [5]float64{102, 110, 90, 91, 100}
		}
	}
	data[n-1] = [5]float64{108, 118, 107, 117, 100}
	return data
}

func TestBreakout_FlatATR_LowVES(t *testing.T) {
	data := flatATRData(60)
	candles := makeBreakoutCandles(data)
	res := scoring.DetectBreakout(candles, defaultBreakoutCfg(), 0)
	if res.VolatilityExpansion > 0.3 {
		t.Errorf("expected low VES with flat ATR, got %.4f", res.VolatilityExpansion)
	}
}
