package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"encoding/json"

	adhttp "pano_chart/backend/adapters/http"
		"pano_chart/backend/domain"
)

// fakeUseCase implements usecases.GetCandleSeries for testing.
type fakeUseCase struct {
	called   bool
	lastSym  domain.Symbol
	lastTf   domain.Timeframe
	lastFrom time.Time
	lastTo   time.Time
	series   domain.CandleSeries
	err      error
}

func (f *fakeUseCase) Execute(sym domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
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

func TestGetCandleSeriesHandler_Returns200OnSuccess(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	c := domain.NewCandleUnsafe(sym, tf, from, 100, 110, 90, 105, 1000)
	series, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{c})

	uc := &fakeUseCase{series: series}
	h := adhttp.NewGetCandleSeriesHandler(uc)

	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTCUSDT&timeframe=1m&from="+from.Format(time.RFC3339)+"&to="+from.Add(time.Minute).Format(time.RFC3339), nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	res := w.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var body struct {
		Symbol    string `json:"symbol"`
		Timeframe string `json:"timeframe"`
		Candles   []struct {
			Timestamp string  `json:"timestamp"`
			Open      float64 `json:"open"`
			High      float64 `json:"high"`
			Low       float64 `json:"low"`
			Close     float64 `json:"close"`
			Volume    float64 `json:"volume"`
		} `json:"candles"`
	}

	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.Symbol != sym.String() {
		t.Errorf("expected symbol %s, got %s", sym.String(), body.Symbol)
	}
	if body.Timeframe != tf.String() {
		t.Errorf("expected timeframe %s, got %s", tf.String(), body.Timeframe)
	}
	if len(body.Candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(body.Candles))
	}
}

func TestGetCandleSeriesHandler_Returns400OnInvalidParams(t *testing.T) {
	uc := &fakeUseCase{}
	h := adhttp.NewGetCandleSeriesHandler(uc)

	// Missing required params
	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTCUSDT", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	res := w.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestGetCandleSeriesHandler_CallsUseCaseWithCorrectArgs(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1m")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(5 * time.Minute)

	uc := &fakeUseCase{}
	h := adhttp.NewGetCandleSeriesHandler(uc)

	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTCUSDT&timeframe=1m&from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339), nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if !uc.called {
		t.Fatal("expected use case to be called")
	}
	if uc.lastSym != sym {
		t.Fatalf("expected symbol %v, got %v", sym, uc.lastSym)
	}
	if uc.lastTf != tf {
		t.Fatalf("expected timeframe %v, got %v", tf, uc.lastTf)
	}
	if !uc.lastFrom.Equal(from) || !uc.lastTo.Equal(to) {
		t.Fatalf("expected time range forwarded unchanged")
	}
}

func TestGetCandleSeriesHandler_Returns500OnUseCaseError(t *testing.T) {
	uc := &fakeUseCase{err: errors.New("boom")}
	h := adhttp.NewGetCandleSeriesHandler(uc)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(5 * time.Minute)

	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTCUSDT&timeframe=1m&from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339), nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	res := w.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.StatusCode)
	}
}
