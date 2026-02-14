package usecases_test

import (
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// fakeRepo records calls and can be configured to return a series or an error.
type fakeRepo struct {
	called   bool
	lastSym  domain.Symbol
	lastTf   domain.Timeframe
	lastFrom time.Time
	lastTo   time.Time
	series   domain.CandleSeries
	err      error
}

func (f *fakeRepo) GetSeries(sym domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	f.called = true
	f.lastSym = sym
	f.lastTf = tf
	f.lastFrom = from
	f.lastTo = to
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

func (f *fakeRepo) GetLastNCandles(sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

func TestGetCandleSeries_DelegatesToRepository(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(10 * time.Minute)

	repo := &fakeRepo{}
	uc := usecases.NewGetCandleSeries(repo)

	_, _ = uc.Execute(sym, tf, from, to)

	if !repo.called {
		t.Fatal("expected repository to be called")
	}
	if repo.lastSym != sym {
		t.Fatalf("expected symbol %v, got %v", sym, repo.lastSym)
	}
	if repo.lastTf != tf {
		t.Fatalf("expected timeframe %v, got %v", tf, repo.lastTf)
	}
	if !repo.lastFrom.Equal(from) || !repo.lastTo.Equal(to) {
		t.Fatalf("expected time range to be forwarded unchanged")
	}
}

func TestGetCandleSeries_ReturnsCandleSeries(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(10 * time.Minute)

	candle := domain.NewCandleUnsafe(sym, tf, from, 100, 110, 90, 105, 1000)
	series, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{candle})

	repo := &fakeRepo{series: series}
	uc := usecases.NewGetCandleSeries(repo)

	res, err := uc.Execute(sym, tf, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Len() != series.Len() {
		t.Fatalf("expected series length %d, got %d", series.Len(), res.Len())
	}
}

func TestGetCandleSeries_PropagatesRepositoryError(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(10 * time.Minute)

	repoErr := errors.New("repository failure")
	repo := &fakeRepo{err: repoErr}
	uc := usecases.NewGetCandleSeries(repo)

	_, err := uc.Execute(sym, tf, from, to)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != repoErr {
		t.Fatalf("expected error to be propagated unchanged")
	}
}
