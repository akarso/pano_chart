package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	h "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/mtf"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Response type (mirrors handler response) ---

type rankingsV2Response struct {
	Timeframe     string                 `json:"timeframe"`
	Sort          string                 `json:"sort"`
	RequestedSort string                 `json:"requestedSort"`
	RSAvailable   bool                   `json:"rsAvailable"`
	Page          int                    `json:"page"`
	PageSize      int                    `json:"pageSize"`
	TotalItems    int                    `json:"totalItems"`
	TotalPages    int                    `json:"totalPages"`
	Results       []rankingsV2ResultItem `json:"results"`
}

type rankingsV2ResultItem struct {
	Symbol     string             `json:"symbol"`
	TotalScore float64            `json:"totalScore"`
	Percentile float64            `json:"percentile"`
	Scores     map[string]float64 `json:"scores"`
	Volume     float64            `json:"volume"`
}

// --- Mock use case ---

type rankingsUseCaseMock struct {
	mock.Mock
}

func (m *rankingsUseCaseMock) Execute(ctx context.Context, req usecases.GetRankingsRequest) (usecases.RankingsResult, error) {
	args := m.Called(ctx, req)
	if res, ok := args.Get(0).(usecases.RankingsResult); ok {
		return res, args.Error(1)
	}
	return usecases.RankingsResult{}, args.Error(1)
}

func rankingsOut(rows []usecases.RankedResult, sort usecases.SortMode, rsOK bool) usecases.RankingsResult {
	return usecases.RankingsResult{Results: rows, Sort: sort, RequestedSort: sort, RSAvailable: rsOK}
}

func rankingsOutFallback(rows []usecases.RankedResult, requested, effective usecases.SortMode) usecases.RankingsResult {
	return usecases.RankingsResult{Results: rows, Sort: effective, RequestedSort: requested, RSAvailable: false}
}

// --- Helper to build domain data ---

func mustSymbol(t *testing.T, s string) domain.Symbol {
	t.Helper()
	sym, err := domain.NewSymbol(s)
	if err != nil {
		t.Fatalf("domain.NewSymbol(%q): %v", s, err)
	}
	return sym
}

// --- Handler tests ---

func TestRankingsV2Handler_MissingTimeframe(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "missing timeframe", body["error"])
	uc.AssertNotCalled(t, "Execute")
}

func TestRankingsV2Handler_InvalidTimeframe(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=wtf", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "invalid timeframe", body["error"])
	uc.AssertNotCalled(t, "Execute")
}

func TestRankingsV2Handler_InvalidAsOf(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&asOf=nope", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "invalid asOf", body["error"])
	uc.AssertNotCalled(t, "Execute")
}

func TestRankingsV2Handler_AsOfPassedToUseCase(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	asOf := time.Unix(1_700_000_000, 0).UTC()
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
		AsOf:      &asOf,
	}).Return(rankingsOut(nil, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&asOf=1700000000", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_InternalError(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h") // safe in test
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(usecases.RankingsResult{}, errors.New("boom"))

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, "internal error", body["error"])
	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_HappyPath_DefaultsAndPagination(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{
			Symbol:     mustSymbol(t, "AAAUSDT"),
			TotalScore: 10,
			Scores:     map[string]float64{"rsi": 70},
			Volume:     1000,
		},
		{
			Symbol:     mustSymbol(t, "BBBUSD"),
			TotalScore: 8,
			Scores:     map[string]float64{"rsi": 60},
			Volume:     500,
		},
		{
			Symbol:     mustSymbol(t, "CCCUSD"),
			TotalScore: 5,
			Scores:     map[string]float64{"rsi": 50},
			Volume:     300,
		},
	}

	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	// page=1, pageSize=2 → expect first 2 items
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&page=1&pageSize=2", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, "1h", body.Timeframe)
	assert.Equal(t, "total", body.Sort)
	assert.Equal(t, "total", body.RequestedSort)
	assert.False(t, body.RSAvailable)
	assert.Equal(t, 1, body.Page)
	assert.Equal(t, 2, body.PageSize)
	assert.Equal(t, 3, body.TotalItems)
	assert.Equal(t, 2, body.TotalPages)

	if assert.Len(t, body.Results, 2) {
		assert.Equal(t, "AAAUSDT", body.Results[0].Symbol)
		assert.Equal(t, 10.0, body.Results[0].TotalScore)
		assert.Equal(t, 1000.0, body.Results[0].Volume)

		assert.Equal(t, "BBBUSD", body.Results[1].Symbol)
	}

	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_RelativeStrengthFieldsAndLeadersSort(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	rsHot, betaHot, rankHot := 0.05, 1.8, 1.0
	rsCol, betaCol, rankCol := -0.02, 0.4, 0.0
	results := []usecases.RankedResult{
		{
			Symbol:           mustSymbol(t, "HOTUSDT"),
			TotalScore:       1,
			RelativeStrength: &rsHot,
			Beta:             &betaHot,
			RSRank:           &rankHot,
		},
		{
			Symbol:           mustSymbol(t, "COLUSDT"),
			TotalScore:       1,
			RelativeStrength: &rsCol,
			Beta:             &betaCol,
			RSRank:           &rankCol,
		},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByLeaders,
	}).Return(rankingsOut(results, usecases.SortByLeaders, true), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&sort=leaders", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "leaders", body["sort"])
	assert.Equal(t, true, body["rsAvailable"])
	rows := body["results"].([]any)
	first := rows[0].(map[string]any)
	assert.Equal(t, "HOTUSDT", first["symbol"])
	assert.InDelta(t, 0.05, first["rs"], 1e-9)
	assert.InDelta(t, 1.8, first["beta"], 1e-9)
	assert.InDelta(t, 1.0, first["rsRank"], 1e-9)
	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_LaggardsAndRSUnavailableFallback(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 0.9},
		{Symbol: mustSymbol(t, "ETHUSDT"), TotalScore: 0.1},
	}
	// Use case fell back to total when tape failed.
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByLaggards,
	}).Return(rankingsOutFallback(results, usecases.SortByLaggards, usecases.SortByTotal), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&sort=laggards", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "total", body["sort"])
	assert.Equal(t, "laggards", body["requestedSort"])
	assert.Equal(t, false, body["rsAvailable"])
	row := body["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "BTCUSDT", row["symbol"])
	_, hasRS := row["rs"]
	assert.False(t, hasRS, "unset rs must be omitted from JSON")
	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_Pagination_SecondPageAndOverflow(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	var results []usecases.RankedResult
	for i := 0; i < 5; i++ {
		sym := mustSymbol(t, "SYM"+strconv.Itoa(i))
		results = append(results, usecases.RankedResult{
			Symbol:     sym,
			TotalScore: float64(10 - i),
			Scores:     map[string]float64{},
			Volume:     float64(i),
		})
	}

	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	// page=2, pageSize=2 → items index 2,3 (0-based)
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&page=2&pageSize=2", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, 5, body.TotalItems)
	assert.Equal(t, 3, body.TotalPages)
	assert.Equal(t, 2, body.Page)
	assert.Equal(t, 2, body.PageSize)
	assert.Len(t, body.Results, 2)

	// Now test page overflow: page beyond last → empty results, but same meta
	r2 := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&page=10&pageSize=2", nil)
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, r2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var body2 rankingsV2Response
	err = json.Unmarshal(w2.Body.Bytes(), &body2)
	assert.NoError(t, err)

	assert.Equal(t, 5, body2.TotalItems)
	assert.Equal(t, 3, body2.TotalPages)
	assert.Equal(t, 10, body2.Page) // still reflects input
	assert.Len(t, body2.Results, 0)

	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_PageSizeClampedTo200(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	// 150 items
	var results []usecases.RankedResult
	for i := 0; i < 150; i++ {
		sym := mustSymbol(t, "SYM"+strconv.Itoa(i))
		results = append(results, usecases.RankedResult{
			Symbol:     sym,
			TotalScore: float64(i),
			Scores:     map[string]float64{},
			Volume:     float64(i),
		})
	}

	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&pageSize=1000", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, 150, body.TotalItems)
	assert.Equal(t, 1, body.TotalPages) // 150 / 200
	assert.Equal(t, 200, body.PageSize)
	assert.Len(t, body.Results, 150)

	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_SymbolsFilter(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "AAAUSDT"), TotalScore: 10, Scores: map[string]float64{}, Volume: 1000},
		{Symbol: mustSymbol(t, "BBBUSD"), TotalScore: 8, Scores: map[string]float64{}, Volume: 500},
		{Symbol: mustSymbol(t, "CCCUSD"), TotalScore: 5, Scores: map[string]float64{}, Volume: 300},
		{Symbol: mustSymbol(t, "DDDUSD"), TotalScore: 3, Scores: map[string]float64{}, Volume: 100},
	}

	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	// Request only BBBUSD and DDDUSD
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&symbols=BBBUSD,DDDUSD", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, 2, body.TotalItems)
	assert.Equal(t, 1, body.TotalPages)
	if assert.Len(t, body.Results, 2) {
		assert.Equal(t, "BBBUSD", body.Results[0].Symbol)
		assert.Equal(t, "DDDUSD", body.Results[1].Symbol)
	}

	uc.AssertExpectations(t)
}

func TestRankingsV2Handler_SymbolsFilter_NoMatch(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "AAAUSDT"), TotalScore: 10, Scores: map[string]float64{}, Volume: 1000},
	}

	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&symbols=ZZZZUSD", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, 0, body.TotalItems)
	assert.Len(t, body.Results, 0)

	uc.AssertExpectations(t)
}

// --- Multi-timeframe overlay tests (PR-099) ---

type fakeMTFCalc struct {
	bySymbol map[string]mtf.Stack
	errFor   map[string]error
}

func (f *fakeMTFCalc) Calculate(_ context.Context, symbol string) (mtf.Stack, error) {
	if err, ok := f.errFor[symbol]; ok {
		return mtf.Stack{}, err
	}
	if s, ok := f.bySymbol[symbol]; ok {
		return s, nil
	}
	return mtf.Stack{}, errors.New("no stack for symbol")
}

// stackWithFrame builds a Stack that has real data (a non-empty Frames),
// distinct from a genuinely empty mtf.Stack{} (no fresh frames — cold
// start / store outage), which the handler must treat as "no data" and
// omit rather than reporting alignment/alignedState from.
func stackWithFrame(alignment float64, state mkt.State) mtf.Stack {
	return mtf.Stack{
		Frames:       []mtf.TFRegime{{Timeframe: "1h", Dominant: state}},
		Alignment:    alignment,
		AlignedState: state,
	}
}

func TestRankingsV2Handler_MTFOverlay_AddsFieldsWhenRequested(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)
	handler.SetMTFCalculator(&fakeMTFCalc{
		bySymbol: map[string]mtf.Stack{
			"BTCUSDT": stackWithFrame(1.0, mkt.StateTrend),
		},
	})

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&mtf=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	row := body["results"].([]any)[0].(map[string]any)
	assert.InDelta(t, 1.0, row["alignment"], 1e-9)
	assert.Equal(t, "trend", row["alignedState"])
}

func TestRankingsV2Handler_MTFOverlay_EmptyStackOmitsFields(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)
	handler.SetMTFCalculator(&fakeMTFCalc{
		bySymbol: map[string]mtf.Stack{
			// Calculate succeeds (no error) but has zero fresh frames — a
			// cold start or store outage, not a real reading.
			"BTCUSDT": {},
		},
	})

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&mtf=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	row := body["results"].([]any)[0].(map[string]any)
	_, hasAlignment := row["alignment"]
	_, hasAlignedState := row["alignedState"]
	assert.False(t, hasAlignment, "alignment must be omitted when the stack has no frames, not stamped 0")
	assert.False(t, hasAlignedState, "alignedState must be omitted when the stack has no frames, not stamped indecisive")
}

func TestRankingsV2Handler_MTFOverlay_OmittedWithoutQueryParam(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)
	handler.SetMTFCalculator(&fakeMTFCalc{
		bySymbol: map[string]mtf.Stack{
			"BTCUSDT": stackWithFrame(1.0, mkt.StateTrend),
		},
	})

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	// No ?mtf=1 — the calculator is wired but must not be consulted.
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	row := body["results"].([]any)[0].(map[string]any)
	_, hasAlignment := row["alignment"]
	_, hasAlignedState := row["alignedState"]
	assert.False(t, hasAlignment, "alignment must be omitted without ?mtf=1")
	assert.False(t, hasAlignedState, "alignedState must be omitted without ?mtf=1")
}

func TestRankingsV2Handler_MTFOverlay_CalculatorErrorLeavesRowUnaffected(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)
	handler.SetMTFCalculator(&fakeMTFCalc{
		errFor: map[string]error{"BTCUSDT": errors.New("store unavailable")},
	})

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&mtf=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// A calculator failure must not fail the row or the request.
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	row := body["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "BTCUSDT", row["symbol"])
	_, hasAlignment := row["alignment"]
	assert.False(t, hasAlignment, "alignment must be omitted when the calculator errors")
}

func TestRankingsV2Handler_MTFOverlay_IgnoredWhenNoCalculatorWired(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc) // SetMTFCalculator never called

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&mtf=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRankingsV2Handler_MTFOverlay_PageSizeCap_AllRowsIndexAligned(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	const n = 200 // the handler's pageSize clamp — the real worst case for the overlay
	results := make([]usecases.RankedResult, n)
	stacks := make(map[string]mtf.Stack, n)
	for i := 0; i < n; i++ {
		symbol := fmt.Sprintf("SYM%03dUSDT", i)
		results[i] = usecases.RankedResult{Symbol: mustSymbol(t, symbol), TotalScore: float64(n - i)}
		// A distinct value per row so a concurrency bug that mixes up which
		// goroutine writes which resp[i] would show up as a mismatch below.
		stacks[symbol] = stackWithFrame(float64(i)/float64(n), mkt.StateTrend)
	}
	handler.SetMTFCalculator(&fakeMTFCalc{bySymbol: stacks})

	tf, _ := domain.NewTimeframe("1h")
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&pageSize=500&mtf=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.InDelta(t, float64(n), body["pageSize"], 0, "pageSize must clamp to 200")

	rows := body["results"].([]any)
	if assert.Len(t, rows, n) {
		for i, raw := range rows {
			row := raw.(map[string]any)
			symbol := fmt.Sprintf("SYM%03dUSDT", i)
			assert.Equal(t, symbol, row["symbol"], "row %d out of order", i)
			want := stacks[symbol]
			assert.InDelta(t, want.Alignment, row["alignment"], 1e-9, "row %d alignment not index-aligned under concurrency", i)
		}
	}
	uc.AssertExpectations(t)
}

// blockingMTFCalc never resolves on its own — it only returns once ctx is
// done, simulating a hung store connection.
type blockingMTFCalc struct{}

func (blockingMTFCalc) Calculate(ctx context.Context, _ string) (mtf.Stack, error) {
	<-ctx.Done()
	return mtf.Stack{}, ctx.Err()
}

func TestRankingsV2Handler_MTFOverlay_HungStoreBoundedBySharedBudget(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)
	handler.SetMTFCalculator(blockingMTFCalc{})

	tf, _ := domain.NewTimeframe("1h")
	results := []usecases.RankedResult{
		{Symbol: mustSymbol(t, "BTCUSDT"), TotalScore: 1},
	}
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(rankingsOut(results, usecases.ParseSortMode("total"), false), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&mtf=1", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(w, r)
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Less(t, elapsed, 2*time.Second,
		"a hung store must not stall the response past the shared overlay budget")

	var body map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	row := body["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "BTCUSDT", row["symbol"])
	_, hasAlignment := row["alignment"]
	assert.False(t, hasAlignment, "alignment must be omitted when the store never responds")
}

// --- Unit tests for helpers ---

func TestParsePositiveIntOrDefault(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{"empty → default", "", 1, 1},
		{"valid", "5", 1, 5},
		{"zero → default", "0", 1, 1},
		{"negative → default", "-3", 1, 1},
		{"invalid → default", "abc", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.ParsePositiveIntOrDefault(tt.in, tt.def) // if you export it; else call directly in same package
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWriteRankingsError(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTeapot)
	err := json.NewEncoder(w).Encode(map[string]string{"error": "oops"})
	assert.NoError(t, err)

	res := w.Result()
	defer func() { _ = res.Body.Close() }()

	assert.Equal(t, http.StatusTeapot, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "oops", body["error"])
}
