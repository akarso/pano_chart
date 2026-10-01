package scoring_test

import (
	"math"
	"math/rand"
	"testing"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	fixtures "pano_chart/backend/tests/fixtures/candles"
)

func TestVarianceRatio_AR1MeanReverting(t *testing.T) {
	closes := ar1Closes(t, -0.5, 200, 42)
	vr := scoring.VarianceRatio(closes, 4)
	if vr >= 0.8 {
		t.Fatalf("AR(1) φ=-0.5 VR(4)=%g want < 0.8", vr)
	}
}

func TestVarianceRatio_RandomWalkNearOne(t *testing.T) {
	closes := ar1Closes(t, 0, 300, 7)
	vr := scoring.VarianceRatio(closes, 4)
	if vr < 0.85 || vr > 1.15 {
		t.Fatalf("random-walk VR(4)=%g want [0.85, 1.15]", vr)
	}
}

func TestVarianceRatio_MomentumAboveOne(t *testing.T) {
	// Positive AR(1) on returns = momentum → VR > 1.
	// Constant drift+noise yields VR≈1 under Lo–MacKinlay; ROADMAP's
	// "trend+noise > 1.2" is not what the estimator measures.
	closes := ar1Closes(t, 0.5, 200, 99)
	vr := scoring.VarianceRatio(closes, 4)
	if vr <= 1.2 {
		t.Fatalf("momentum AR(1) φ=+0.5 VR(4)=%g want > 1.2", vr)
	}
}

func TestMeanReversionScore_AR1High(t *testing.T) {
	closes := ar1Closes(t, -0.5, 200, 42)
	mrs := scoring.MeanReversionScore(closes)
	if mrs < 0.5 {
		t.Fatalf("AR(1) φ=-0.5 MeanReversionScore=%g want ≥ 0.5", mrs)
	}
}

func TestMeanReversionScore_TightRangeIsLow(t *testing.T) {
	// MRS is Lo–MacKinlay return autocorrelation, not channel quality.
	// tight_range is a smooth channel (VR≫1) → MRS≈0.
	series := fixtures.Load(t, "tight_range")
	closes := closesOf(t, series)
	mrs := scoring.MeanReversionScore(closes)
	if mrs > 0.1 {
		t.Fatalf("tight_range MeanReversionScore=%g want ≈0 (not a channel-quality metric)", mrs)
	}
}

func TestVarianceRatio_ShortSeriesNeutral(t *testing.T) {
	if got := scoring.VarianceRatio([]float64{100}, 4); got != 1 {
		t.Fatalf("short series: VR=%g want 1", got)
	}
	if got := scoring.VarianceRatio([]float64{100, 101, 102}, 4); got != 1 {
		t.Fatalf("len < q+2: VR=%g want 1", got)
	}
}

func TestVarianceRatio_NaNInfNonPositiveGuards(t *testing.T) {
	// Length ≥ q+2 so finitePositive / Inf guards actually run.
	nanCloses := make([]float64, 12)
	infCloses := make([]float64, 12)
	negCloses := make([]float64, 12)
	for i := range nanCloses {
		nanCloses[i] = 100 + float64(i)*0.1
		infCloses[i] = 100 + float64(i)*0.1
		negCloses[i] = 100 + float64(i)*0.1
	}
	nanCloses[5] = math.NaN()
	infCloses[5] = math.Inf(1)
	negCloses[5] = -1

	if got := scoring.VarianceRatio(nanCloses, 4); got != 1 {
		t.Fatalf("NaN close: VR=%g want 1", got)
	}
	if got := scoring.VarianceRatio(infCloses, 4); got != 1 {
		t.Fatalf("Inf close: VR=%g want 1", got)
	}
	if got := scoring.VarianceRatio(negCloses, 4); got != 1 {
		t.Fatalf("non-positive close: VR=%g want 1", got)
	}
	if got := scoring.MeanReversionScore(infCloses); got != 0 {
		t.Fatalf("Inf close: MRS=%g want 0", got)
	}
	flat := make([]float64, 50)
	for i := range flat {
		flat[i] = 100
	}
	if got := scoring.VarianceRatio(flat, 4); got != 1 {
		t.Fatalf("flat series: VR=%g want 1", got)
	}
}

func TestSidewaysV5_MeanReversionWeightZeroSkipsMRS(t *testing.T) {
	cfg := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	if cfg.MeanReversionWeight != 0 {
		t.Fatalf("default MeanReversionWeight=%g want 0", cfg.MeanReversionWeight)
	}
	series := fixtures.Load(t, "tight_range")
	res := scoring.DetectSidewaysV5(series.All(), cfg)
	if _, ok := res.Components["MRS"]; ok {
		t.Fatal("weight=0 must not populate MRS component (skip work)")
	}
}

func TestSidewaysV5_MeanReversionWeightZeroMatchesFourTermAverage(t *testing.T) {
	series := fixtures.Load(t, "tight_range")
	cfg := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	cfg.MeanReversionWeight = 0
	res := scoring.DetectSidewaysV5(series.All(), cfg)
	assertFourTermScore(t, cfg, res)
}

func TestSidewaysV5_NegativeMeanReversionWeightTreatedAsZero(t *testing.T) {
	series := fixtures.Load(t, "tight_range")
	cfg := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	cfg.MeanReversionWeight = -1
	res := scoring.DetectSidewaysV5(series.All(), cfg)
	if _, ok := res.Components["MRS"]; ok {
		t.Fatal("negative weight must not populate MRS")
	}
	assertFourTermScore(t, cfg, res)
}

func TestNewSidewaysV5Config_ClampsNegativeMeanReversionWeight(t *testing.T) {
	t.Cleanup(scoring.ResetConfig)
	cfg := scoring.DefaultAppConfig()
	cfg.Sideways.Weights.MeanReversionWeight = -3
	scoring.ReplaceConfigForTest(cfg)
	got := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	if got.MeanReversionWeight != 0 {
		t.Fatalf("MeanReversionWeight=%g want 0 after clamp", got.MeanReversionWeight)
	}
}

func TestSidewaysV5_MeanReversionWeightActiveFiveTermAverage(t *testing.T) {
	series := fixtures.Load(t, "tight_range")
	cfg := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	cfg.MeanReversionWeight = 1.0
	res := scoring.DetectSidewaysV5(series.All(), cfg)
	mrs, ok := res.Components["MRS"]
	if !ok {
		t.Fatal("weight>0 must populate MRS")
	}
	ccs, oqs, dcs, vos := res.Components["CCS"], res.Components["OQS"], res.Components["DCS"], res.Components["VOS"]
	srm := res.Components["SRM"]
	wSum := cfg.W1*ccs + cfg.W2*oqs + cfg.W3*dcs + cfg.W4*vos + cfg.MeanReversionWeight*mrs
	wTot := cfg.W1 + cfg.W2 + cfg.W3 + cfg.W4 + cfg.MeanReversionWeight
	want := (wSum / wTot) * srm
	if math.Abs(res.Score-want) > 1e-12 {
		t.Fatalf("five-term score=%g want %g", res.Score, want)
	}
	// tight_range MRS≈0 → weight dilutes vs four-term average.
	four := ((cfg.W1*ccs + cfg.W2*oqs + cfg.W3*dcs + cfg.W4*vos) / (cfg.W1 + cfg.W2 + cfg.W3 + cfg.W4)) * srm
	if res.Score > four+1e-12 {
		t.Fatalf("MRS≈0 dilution: score=%g must be ≤ four-term %g", res.Score, four)
	}
}

func TestNewSidewaysV5Config_DefaultMeanReversionWeightZero(t *testing.T) {
	cfg := scoring.NewSidewaysV5ConfigForTimeframe("1h")
	if cfg.MeanReversionWeight != 0 {
		t.Fatalf("MeanReversionWeight=%g want 0 from DefaultAppConfig/YAML", cfg.MeanReversionWeight)
	}
}

func assertFourTermScore(t *testing.T, cfg scoring.SidewaysV5Config, res scoring.SidewaysResult) {
	t.Helper()
	ccs, oqs, dcs, vos := res.Components["CCS"], res.Components["OQS"], res.Components["DCS"], res.Components["VOS"]
	srm := res.Components["SRM"]
	wSum := cfg.W1*ccs + cfg.W2*oqs + cfg.W3*dcs + cfg.W4*vos
	wTot := cfg.W1 + cfg.W2 + cfg.W3 + cfg.W4
	want := (wSum / wTot) * srm
	if math.Abs(res.Score-want) > 1e-12 {
		t.Fatalf("four-term score=%g want %g", res.Score, want)
	}
}

func closesOf(t *testing.T, series domain.CandleSeries) []float64 {
	t.Helper()
	closes := make([]float64, series.Len())
	for i := 0; i < series.Len(); i++ {
		c, err := series.At(i)
		if err != nil {
			t.Fatal(err)
		}
		closes[i] = c.Close()
	}
	return closes
}

func ar1Closes(t *testing.T, phi float64, n int, seed int64) []float64 {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	closes := make([]float64, n)
	closes[0] = 100
	var r float64
	for i := 1; i < n; i++ {
		eps := rng.NormFloat64() * 0.01
		r = phi*r + eps
		closes[i] = closes[i-1] * math.Exp(r)
		if closes[i] <= 0 {
			closes[i] = closes[i-1]
		}
	}
	return closes
}
