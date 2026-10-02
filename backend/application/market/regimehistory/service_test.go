package regimehistory_test

import (
	"context"
	"testing"
	"time"

	"pano_chart/backend/application/market/regimehistory"
	"pano_chart/backend/application/replay"
	mkt "pano_chart/backend/domain/market"
)

type stubHistoryRepo struct {
	periods []mkt.RegimePeriod
}

func (s stubHistoryRepo) GetLatest(_ string) (*mkt.RegimePeriod, error) { return nil, nil }
func (s stubHistoryRepo) Append(_ string, _ mkt.RegimePeriod) error     { return nil }
func (s stubHistoryRepo) CloseCurrent(_ string, _ int64) error          { return nil }
func (s stubHistoryRepo) UpdateDuration(_ string, _ int) error          { return nil }
func (s stubHistoryRepo) GetHistory(_ context.Context, _ string, _ int) ([]mkt.RegimePeriod, error) {
	return append([]mkt.RegimePeriod(nil), s.periods...), nil
}

func TestGetHistory_PointInTimeAtAsOf(t *testing.T) {
	end1 := int64(1_700_000_000)
	end2 := int64(1_700_100_000)
	svc := regimehistory.NewService(stubHistoryRepo{periods: []mkt.RegimePeriod{
		{Regime: mkt.RegimeTrend, StartTimestamp: 0, EndTimestamp: &end1, DurationCandles: 3},
		{Regime: mkt.RegimeSideways, StartTimestamp: end1, EndTimestamp: &end2, DurationCandles: 5},
		{Regime: mkt.RegimeCompression, StartTimestamp: end2, EndTimestamp: nil, DurationCandles: 2},
	}})

	asOf := time.Unix(end1+500, 0).UTC()
	ctx := replay.WithAsOf(context.Background(), asOf)
	hist, err := svc.GetHistory(ctx, "4h", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Periods) != 2 {
		t.Fatalf("periods=%d want trend + sideways-at-asOf", len(hist.Periods))
	}
	if hist.Periods[1].EndTimestamp != nil {
		t.Fatal("covering period should be open at asOf")
	}
	if hist.CurrentAge != hist.Periods[1].DurationCandles || hist.CurrentAge <= 0 {
		t.Fatalf("CurrentAge=%d duration=%d", hist.CurrentAge, hist.Periods[1].DurationCandles)
	}

	liveAge, _ := svc.CurrentAge("4h")
	replayAge, _ := svc.AgeAtAsOf(ctx, "4h", asOf)
	if replayAge <= 0 {
		t.Fatalf("replayAge=%d", replayAge)
	}
	_ = liveAge // live open period may differ; replay age is boundary-based at asOf
}
