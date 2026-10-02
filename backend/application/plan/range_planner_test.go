package plan_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/plan"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	fixtures "pano_chart/backend/tests/fixtures/candles"
)

func TestBuildRangePlanFromBounds_HandBuiltValid(t *testing.T) {
	// ROADMAP: Low≈100 High≈110 ATR=1 → LongEntry 100.25 LongStop 99 Mid 105 RR≈3.8
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 1, 0.8)
	if !p.Valid {
		t.Fatalf("Valid=false reason=%q", p.Reason)
	}
	if math.Abs(p.LongEntry-100.25) > 1e-9 {
		t.Fatalf("LongEntry=%v want 100.25", p.LongEntry)
	}
	if math.Abs(p.LongStop-99) > 1e-9 {
		t.Fatalf("LongStop=%v want 99", p.LongStop)
	}
	if math.Abs(p.LongTarget-105) > 1e-9 {
		t.Fatalf("LongTarget=%v want 105 (Mid)", p.LongTarget)
	}
	if math.Abs(p.RiskReward-3.8) > 1e-9 {
		t.Fatalf("RR=%v want 3.8", p.RiskReward)
	}
	if math.Abs(p.ShortEntry-109.75) > 1e-9 {
		t.Fatalf("ShortEntry=%v want 109.75", p.ShortEntry)
	}
	if math.Abs(p.Position-0.5) > 1e-9 {
		t.Fatalf("Position=%v want 0.5", p.Position)
	}
}

func TestBuildRangePlanFromBounds_LowQualityInvalid(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 1, 0.2)
	if p.Valid {
		t.Fatal("expected Valid=false")
	}
	if p.Reason != "range quality" {
		t.Fatalf("Reason=%q want range quality", p.Reason)
	}
	assertLevelsCleared(t, p)
	// RR kept for diagnostics (Mid geometry ⇒ 3.8)
	if math.Abs(p.RiskReward-3.8) > 1e-9 {
		t.Fatalf("RiskReward=%v want 3.8 (diagnostic)", p.RiskReward)
	}
}

func TestBuildRangePlanFromBounds_NarrowChannelWidth(t *testing.T) {
	// widthATR=3.2 < 3.5 → channel width (binding gate; Mid RR is implied by width)
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 103.2, 101.6, 1, 0.8)
	if p.Valid {
		t.Fatal("expected Valid=false")
	}
	if p.Reason != "channel width" {
		t.Fatalf("Reason=%q want channel width", p.Reason)
	}
	assertLevelsCleared(t, p)
	// RR = 3.2/2.5 − 0.2 = 1.08 — still reported for diagnostics
	if math.Abs(p.RiskReward-1.08) > 1e-9 {
		t.Fatalf("RiskReward=%v want 1.08", p.RiskReward)
	}
}

func TestBuildRangePlanFromBounds_BorderlineWidthValid(t *testing.T) {
	// widthATR=3.5 → Mid RR = 3.5/2.5 − 0.2 = 1.2 exactly (invariant of Mid geometry)
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 103.5, 101.75, 1, 0.8)
	if !p.Valid {
		t.Fatalf("Valid=false reason=%q RR=%v", p.Reason, p.RiskReward)
	}
	if math.Abs(p.RiskReward-1.2) > 1e-9 {
		t.Fatalf("RR=%v want 1.2", p.RiskReward)
	}
}

func TestBuildRangePlanFromBounds_ATRUnavailable(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 0, 0.8)
	if p.Valid || p.Reason != "atr unavailable" {
		t.Fatalf("Valid=%v Reason=%q", p.Valid, p.Reason)
	}
}

func TestBuildRangePlanFromBounds_DegenerateChannel(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 100, 100, 1, 0.8)
	if p.Valid || p.Reason != "degenerate channel" {
		t.Fatalf("Valid=%v Reason=%q", p.Valid, p.Reason)
	}
}

func TestBuildRangePlanFromBounds_NonPositiveStop(t *testing.T) {
	// Low − ATR ≤ 0
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 0.5, 10.5, 5.5, 1, 0.8)
	if p.Valid || p.Reason != "non-positive stop" {
		t.Fatalf("Valid=%v Reason=%q", p.Valid, p.Reason)
	}
	assertLevelsCleared(t, p)
	if p.Position != 0.5 {
		t.Fatalf("Position=%v want 0.5 (set before stop check)", p.Position)
	}
}

func TestBuildRangePlanFromBounds_PositionClamped(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 150, 1, 0.8)
	if p.Position != 1 {
		t.Fatalf("Position=%v want 1 (clamped)", p.Position)
	}
	p2 := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 50, 1, 0.8)
	if p2.Position != 0 {
		t.Fatalf("Position=%v want 0 (clamped)", p2.Position)
	}
}

func TestBuildRangePlan_TrendingFixtureInvalid(t *testing.T) {
	series := fixtures.Load(t, "messy_uptrend")
	p := plan.BuildRangePlan("BTCUSDT", series.Timeframe().String(), series)
	if p.Valid {
		t.Fatalf("trending series Valid=true quality=%.3f", p.RangeQuality)
	}
	if p.Reason != "range quality" {
		t.Fatalf("Reason=%q want range quality (got quality=%.3f)", p.Reason, p.RangeQuality)
	}
	assertLevelsCleared(t, p)
}

func TestBuildRangePlan_InsufficientBars(t *testing.T) {
	series := oscillatingRange(t, 10, 100, 110)
	p := plan.BuildRangePlan("BTCUSDT", "1h", series)
	if p.Valid || p.Reason != "insufficient bars" {
		t.Fatalf("Valid=%v Reason=%q", p.Valid, p.Reason)
	}
}

func TestBuildRangePlan_RangingSeriesValid(t *testing.T) {
	series := oscillatingRange(t, 110, 100, 110)
	p := plan.BuildRangePlan("BTCUSDT", "1h", series)
	if p.ATR <= 0 {
		t.Fatalf("ATR=%v want >0", p.ATR)
	}
	if p.RangeQuality < 0.5 {
		t.Fatalf("quality=%.3f want ≥0.5 (Sideways path)", p.RangeQuality)
	}
	if !p.Valid {
		t.Fatalf("Valid=false reason=%q low=%v high=%v atr=%v rr=%v",
			p.Reason, p.Low, p.High, p.ATR, p.RiskReward)
	}
	if p.RiskReward < 1.2 {
		t.Fatalf("RR=%v want ≥1.2", p.RiskReward)
	}
	if p.LongEntry == 0 || p.ShortEntry == 0 {
		t.Fatal("expected non-zero levels when Valid")
	}
}

func TestChannelFromSwings_IgnoresEdgeSpike(t *testing.T) {
	series := oscillatingRange(t, 110, 100, 110)
	candles := append([]domain.Candle(nil), series.All()...)
	last := candles[len(candles)-1]
	// Wick spike on the final bar — cannot confirm as a pivot (needs 3 bars after).
	candles[len(candles)-1] = domain.NewCandleUnsafe(
		last.Symbol(), last.Timeframe(), last.Timestamp(),
		last.Open(), 200, last.Low(), last.Close(), last.Volume(),
	)
	spiked, err := domain.NewCandleSeries(last.Symbol(), last.Timeframe(), candles)
	if err != nil {
		t.Fatal(err)
	}
	p := plan.BuildRangePlan("BTCUSDT", "1h", spiked)
	if p.High > 115 {
		t.Fatalf("High=%v warped by edge spike (want ~110)", p.High)
	}
}

func TestChannelFromSwings_EqualTouchesStillConfirm(t *testing.T) {
	// Plateau highs/lows within the pivot window: strict pivots would reject
	// both equal touches; AllowEqual must still yield a channel.
	series := plateauRange(t, 110, 100, 110)
	p := plan.BuildRangePlan("BTCUSDT", "1h", series)
	if p.Reason == "degenerate channel" {
		t.Fatal("equal high/low touches must not yield degenerate channel")
	}
	if math.Abs(p.High-110) > 0.5 {
		t.Fatalf("High=%v want ~110", p.High)
	}
	if math.Abs(p.Low-100) > 0.5 {
		t.Fatalf("Low=%v want ~100", p.Low)
	}
}

func TestBuildRangePlanFromBounds_NaNPricePositionZero(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, math.NaN(), 1, 0.8)
	if math.IsNaN(p.Position) || math.IsInf(p.Position, 0) {
		t.Fatalf("Position=%v want finite", p.Position)
	}
	if p.Position != 0 {
		t.Fatalf("Position=%v want 0 for NaN price", p.Position)
	}
}

func TestSize(t *testing.T) {
	got := plan.Size(100, 100.25, 99)
	if math.Abs(got-80) > 1e-9 {
		t.Fatalf("Size=%v want 80", got)
	}
	if plan.Size(100, 100, 100) != 0 {
		t.Fatal("zero distance must return 0")
	}
	if plan.Size(0, 100.25, 99) != 0 {
		t.Fatal("non-positive risk must return 0")
	}
	// Finite operands can still overflow to +Inf; Size must not return Inf.
	if out := plan.Size(math.MaxFloat64, 100.25, 100); math.IsInf(out, 0) || out != 0 {
		t.Fatalf("Size=%v want 0 (non-finite result rejected)", out)
	}
}

func TestWindowBars_MatchesSidewaysCandleCount(t *testing.T) {
	if got := plan.WindowBars("1h"); got != 110 {
		t.Fatalf("WindowBars(1h)=%d want 110 (Sideways default)", got)
	}
}

func TestServiceEvaluate_SizesWhenValid(t *testing.T) {
	series := oscillatingRange(t, 110, 100, 110)
	svc := plan.NewService(&stubCandles{series: series})
	res, err := svc.Evaluate(context.Background(), "BTCUSDT", "1h", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Plan.Valid {
		t.Fatalf("Valid=false reason=%q quality=%.3f", res.Plan.Reason, res.Plan.RangeQuality)
	}
	if res.Size <= 0 || res.ShortSize <= 0 {
		t.Fatalf("size=%v shortSize=%v", res.Size, res.ShortSize)
	}
}

func TestServiceEvaluate_NoSizeWhenInvalid(t *testing.T) {
	series := fixtures.Load(t, "messy_uptrend")
	svc := plan.NewService(&stubCandles{series: series})
	res, err := svc.Evaluate(context.Background(), "BTCUSDT", series.Timeframe().String(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan.Valid {
		t.Fatal("expected invalid")
	}
	if res.Size != 0 || res.ShortSize != 0 {
		t.Fatalf("size=%v shortSize=%v want 0", res.Size, res.ShortSize)
	}
}

func TestServiceEvaluate_DataUnavailable(t *testing.T) {
	svc := plan.NewService(&stubCandles{err: errors.New("upstream 429")})
	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "1h", 100)
	if !errors.Is(err, plan.ErrDataUnavailable) {
		t.Fatalf("err=%v want ErrDataUnavailable", err)
	}
}

func TestServiceEvaluate_ContextCanceled(t *testing.T) {
	svc := plan.NewService(&stubCandles{err: context.Canceled})
	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "1h", 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled (not DATA_UNAVAILABLE)", err)
	}
	if errors.Is(err, plan.ErrDataUnavailable) {
		t.Fatal("cancel must not wrap as ErrDataUnavailable")
	}
}

func TestServiceEvaluate_EmptySeries(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("1h")
	empty, err := domain.NewCandleSeries(sym, tf, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := plan.NewService(&stubCandles{series: empty})
	_, err = svc.Evaluate(context.Background(), "BTCUSDT", "1h", 100)
	if !errors.Is(err, plan.ErrDataUnavailable) {
		t.Fatalf("err=%v want ErrDataUnavailable", err)
	}
}

func assertLevelsCleared(t *testing.T, p plan.RangePlan) {
	t.Helper()
	if p.LongEntry != 0 || p.LongStop != 0 || p.LongTarget != 0 ||
		p.ShortEntry != 0 || p.ShortStop != 0 || p.ShortTarget != 0 ||
		p.LongTargetFull != 0 || p.ShortTargetFull != 0 {
		t.Fatalf("expected cleared levels when invalid: %+v", p)
	}
}

// oscillatingRange builds a triangle channel with confirmed interior swing
// pivots at low/high. Half-period 11 keeps extrema on unique bars so a
// pivot=3 window sees a single strict high/low (sine sampling can tie).
func oscillatingRange(t *testing.T, n int, low, high float64) domain.CandleSeries {
	t.Helper()
	sym, err := domain.NewSymbol("BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	tf, err := domain.NewTimeframe("1h")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	mid := (low + high) / 2
	amp := (high - low) / 2
	half := 11 // odd → unique crest/trough indices
	candles := make([]domain.Candle, n)
	for i := 0; i < n; i++ {
		// Triangle wave in [-1,1], crest at i% (2*half) == 0 after offset.
		pos := i % (2 * half)
		var unit float64
		if pos <= half {
			unit = -1 + 2*float64(pos)/float64(half)
		} else {
			unit = 1 - 2*float64(pos-half)/float64(half)
		}
		c := mid + amp*unit
		wick := 0.2
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*time.Hour),
			c, c+wick, c-wick, c, 1000,
		)
	}
	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return series
}

// plateauRange is like oscillatingRange but holds two consecutive bars at each
// crest/trough with identical highs/lows — the case strict pivots reject.
func plateauRange(t *testing.T, n int, low, high float64) domain.CandleSeries {
	t.Helper()
	series := oscillatingRange(t, n, low, high)
	candles := append([]domain.Candle(nil), series.All()...)
	sym := candles[0].Symbol()
	tf := candles[0].Timeframe()
	half := 11
	period := 2 * half
	for i := 0; i < n; i++ {
		pos := i % period
		// Crest at pos==half, trough at pos==0: duplicate onto the next bar.
		if pos == half && i+1 < n {
			c := candles[i]
			candles[i+1] = domain.NewCandleUnsafe(
				sym, tf, candles[i+1].Timestamp(),
				c.Close(), c.High(), c.Low(), c.Close(), c.Volume(),
			)
		}
		if pos == 0 && i+1 < n && i > 0 {
			c := candles[i]
			candles[i+1] = domain.NewCandleUnsafe(
				sym, tf, candles[i+1].Timestamp(),
				c.Close(), c.High(), c.Low(), c.Close(), c.Volume(),
			)
		}
	}
	out, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type stubCandles struct {
	series domain.CandleSeries
	err    error
}

func (s *stubCandles) GetSeries(context.Context, domain.Symbol, domain.Timeframe, time.Time, time.Time) (domain.CandleSeries, error) {
	if s.err != nil {
		return domain.CandleSeries{}, s.err
	}
	return s.series, nil
}

func (s *stubCandles) GetLastNCandles(context.Context, domain.Symbol, domain.Timeframe, int) (domain.CandleSeries, error) {
	if s.err != nil {
		return domain.CandleSeries{}, s.err
	}
	return s.series, nil
}

var _ ports.CandleRepositoryPort = (*stubCandles)(nil)
