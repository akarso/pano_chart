package scoring_test

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
)

func TestTimeframeAwareCalculator_DiffersByTimeframeConfig_V5(t *testing.T) {
	// V5 also re-resolves IdealATRRange inside Score from IdealATRRangeMap;
	// this still checks the wrapper + factory path end-to-end.
	calc := scoring.NewTimeframeAwareCalculator("Sideways Consistency", func(tf string) scoring.SymbolScoreCalculator {
		return &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe(tf),
		}
	})

	s15, err := calc.Score(sidewaysSeries(t, "15m"))
	if err != nil {
		t.Fatalf("15m Score: %v", err)
	}
	s1d, err := calc.Score(sidewaysSeries(t, "1d"))
	if err != nil {
		t.Fatalf("1d Score: %v", err)
	}
	if s15 == s1d {
		t.Fatalf("expected different scores for 15m vs 1d configs, both got %f", s15)
	}
}

func TestTimeframeAwareCalculator_DiffersByTimeframeConfig_V3(t *testing.T) {
	// V3 bakes RMin/RMax at construction with no Score-time re-resolve —
	// this is the failure mode a hardcoded "1h" calculator would miss.
	calc := scoring.NewTimeframeAwareCalculator("Sideways Consistency", func(tf string) scoring.SymbolScoreCalculator {
		return &scoring.SidewaysV3ScoreCalculator{
			Config: scoring.DefaultSidewaysV3Config(tf),
		}
	})

	s15, err := calc.Score(v3ChannelSeries(t, "15m"))
	if err != nil {
		t.Fatalf("15m Score: %v", err)
	}
	s1d, err := calc.Score(v3ChannelSeries(t, "1d"))
	if err != nil {
		t.Fatalf("1d Score: %v", err)
	}
	if s15 == 0 {
		t.Fatal("expected non-zero 15m V3 score for in-band channel")
	}
	if s1d != 0 {
		t.Fatalf("expected zero 1d V3 score (channel below RMin), got %f", s1d)
	}
	if s15 == s1d {
		t.Fatalf("expected V3 15m vs 1d scores to differ, both got %f", s15)
	}
}

// v3ChannelSeries is ~1.2% tall — inside 15m RMin/RMax, below 1d RMin.
func v3ChannelSeries(t *testing.T, tfStr string) domain.CandleSeries {
	t.Helper()
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		t.Fatal(err)
	}
	sym := domain.NewSymbolUnsafe("TESTUSDT")
	n := 80
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	step := tf.Duration()
	const mid, half = 100.0, 0.6
	for i := 0; i < n; i++ {
		c := mid - half + 0.05
		if i%2 == 0 {
			c = mid + half - 0.05
		}
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*step),
			c, mid+half, mid-half, c, 1000,
		)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return series
}

func TestTimeframeAwareCalculator_CacheIsDeterministic(t *testing.T) {
	calls := 0
	calc := scoring.NewTimeframeAwareCalculator("Sideways Consistency", func(tf string) scoring.SymbolScoreCalculator {
		calls++
		return &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe(tf),
		}
	})

	series := sidewaysSeries(t, "15m")
	a, err := calc.Score(series)
	if err != nil {
		t.Fatalf("first Score: %v", err)
	}
	b, err := calc.Score(series)
	if err != nil {
		t.Fatalf("second Score: %v", err)
	}
	if a != b {
		t.Errorf("same timeframe must be deterministic: %f vs %f", a, b)
	}
	if calls != 1 {
		t.Errorf("Factory should run once per timeframe (cache); got %d calls", calls)
	}
	if math.IsNaN(a) {
		t.Error("score is NaN")
	}
}

func TestTimeframeAwareCalculator_Name(t *testing.T) {
	calc := scoring.NewTimeframeAwareCalculator("Sideways Consistency", nil)
	if calc.Name() != "Sideways Consistency" {
		t.Errorf("Name() = %q", calc.Name())
	}
}

// sidewaysSeries builds 110 bars of a mild oscillating range suitable for
// SidewaysV5/V3 (non-zero ATR, roughly channel-like).
func sidewaysSeries(t *testing.T, tfStr string) domain.CandleSeries {
	t.Helper()
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		t.Fatal(err)
	}
	sym := domain.NewSymbolUnsafe("TESTUSDT")
	n := 110
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	step := tf.Duration()
	for i := 0; i < n; i++ {
		wave := math.Sin(float64(i) * 0.35)
		mid := 100.0 + wave*1.5
		o := mid - 0.2
		c := mid + 0.2
		h := mid + 0.8
		l := mid - 0.8
		candles[i] = domain.NewCandleUnsafe(sym, tf, base.Add(time.Duration(i)*step), o, h, l, c, 1000)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return series
}
