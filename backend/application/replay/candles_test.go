package replay_test

import (
	"context"
	"testing"
	"time"

	"pano_chart/backend/application/replay"
	"pano_chart/backend/domain"
)

type stubCandles struct {
	lastNCalls  int
	seriesCalls int
	from, to    time.Time
	series      domain.CandleSeries
}

func (s *stubCandles) GetSeries(_ context.Context, _ domain.Symbol, _ domain.Timeframe, from, to time.Time) (domain.CandleSeries, error) {
	s.seriesCalls++
	s.from, s.to = from, to
	return s.series, nil
}

func (s *stubCandles) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	s.lastNCalls++
	return s.series, nil
}

func TestParseUnixSeconds(t *testing.T) {
	got, err := replay.ParseUnixSeconds("")
	if err != nil || got != nil {
		t.Fatalf("empty: got=%v err=%v", got, err)
	}
	got, err = replay.ParseUnixSeconds("1700000000")
	if err != nil || got == nil || got.Unix() != 1700000000 {
		t.Fatalf("valid: got=%v err=%v", got, err)
	}
	if _, err := replay.ParseUnixSeconds("0"); err == nil {
		t.Fatal("expected error for 0")
	}
	if _, err := replay.ParseUnixSeconds("abc"); err == nil {
		t.Fatal("expected error for abc")
	}
	if _, err := replay.ParseUnixSeconds("999999999999999999999"); err == nil {
		t.Fatal("expected error for overflow")
	}
}

func TestValidateAsOf_RejectsFuture(t *testing.T) {
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	if err := replay.ValidateAsOf(future, now); err == nil {
		t.Fatal("expected future asOf to fail")
	}
	if err := replay.ValidateAsOf(now, now); err != nil {
		t.Fatalf("now: %v", err)
	}
}

func TestFetchCandles_LiveUsesLastN(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	repo := &stubCandles{}
	_, err := replay.FetchCandles(context.Background(), repo, sym, tf, 110)
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastNCalls != 1 || repo.seriesCalls != 0 {
		t.Fatalf("lastN=%d series=%d", repo.lastNCalls, repo.seriesCalls)
	}
}

func TestFetchCandles_AsOfUsesSeriesWindow(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	asOf := time.Unix(1_700_000_000, 0).UTC()
	repo := &stubCandles{}
	ctx := replay.WithAsOf(context.Background(), asOf)
	_, err := replay.FetchCandles(ctx, repo, sym, tf, 110)
	if err != nil {
		t.Fatal(err)
	}
	if repo.seriesCalls != 1 || repo.lastNCalls != 0 {
		t.Fatalf("lastN=%d series=%d", repo.lastNCalls, repo.seriesCalls)
	}
	wantFrom := asOf.Add(-112 * time.Hour)
	wantTo := asOf.Add(time.Hour)
	if !repo.from.Equal(wantFrom) || !repo.to.Equal(wantTo) {
		t.Fatalf("window from=%v to=%v want from=%v to=%v", repo.from, repo.to, wantFrom, wantTo)
	}
}

func TestFetchCandles_AsOfDropsFormingBar(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Two bars: first closed, second still forming at asOf mid-bar with a
	// wick close that must not affect the returned window.
	bars := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, base, 100, 101, 99, 100, 1),
		domain.NewCandleUnsafe(sym, tf, base.Add(time.Hour), 100, 200, 50, 199, 1),
	}
	series, err := domain.NewCandleSeries(sym, tf, bars)
	if err != nil {
		t.Fatal(err)
	}
	repo := &stubCandles{series: series}
	asOf := base.Add(90 * time.Minute) // inside second bar
	ctx := replay.WithAsOf(context.Background(), asOf)
	got, err := replay.FetchCandles(ctx, repo, sym, tf, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != 1 {
		t.Fatalf("len=%d want only completed first bar", got.Len())
	}
	c, _ := got.At(0)
	if c.Close() != 100 {
		t.Fatalf("close=%v want 100 not forming bar close 199", c.Close())
	}
}
