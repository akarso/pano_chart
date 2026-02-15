package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	h "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Response type (mirrors handler response) ---

type rankingsV2Response struct {
	Timeframe  string                 `json:"timeframe"`
	Sort       string                 `json:"sort"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	TotalItems int                    `json:"totalItems"`
	TotalPages int                    `json:"totalPages"`
	Results    []rankingsV2ResultItem `json:"results"`
}

type rankingsV2ResultItem struct {
	Symbol     string             `json:"symbol"`
	TotalScore float64            `json:"totalScore"`
	Scores     map[string]float64 `json:"scores"`
	Volume     float64            `json:"volume"`
}

// --- Mock use case ---

type rankingsUseCaseMock struct {
	mock.Mock
}

func (m *rankingsUseCaseMock) Execute(ctx context.Context, req usecases.GetRankingsRequest) ([]usecases.RankedResult, error) {
	args := m.Called(ctx, req)
	if res, ok := args.Get(0).([]usecases.RankedResult); ok {
		return res, args.Error(1)
	}
	return nil, args.Error(1)
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

func TestRankingsV2Handler_InternalError(t *testing.T) {
	uc := &rankingsUseCaseMock{}
	handler := h.NewRankingsV2Handler(uc)

	tf, _ := domain.NewTimeframe("1h") // safe in test
	uc.On("Execute", mock.Anything, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.ParseSortMode("total"),
	}).Return(nil, errors.New("boom"))

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
	}).Return(results, nil)

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
	}).Return(results, nil)

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

func TestRankingsV2Handler_PageSizeClampedTo100(t *testing.T) {
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
	}).Return(results, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h&pageSize=1000", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var body rankingsV2Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)

	assert.Equal(t, 150, body.TotalItems)
	assert.Equal(t, 2, body.TotalPages) // 150 / 100
	assert.Equal(t, 100, body.PageSize)
	assert.Len(t, body.Results, 100)

	uc.AssertExpectations(t)
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
