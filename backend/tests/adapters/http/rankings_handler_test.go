package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httpadapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type stubRanker struct {
	Result []usecases.RankedSymbol
}

func (s *stubRanker) Rank(_ map[domain.Symbol]domain.CandleSeries) ([]usecases.RankedSymbol, error) {
	return s.Result, nil
}

func TestRankingsHandler_IntegrationStub(t *testing.T) {
	// Setup stub data
	symA, _ := domain.NewSymbol("BTCUSDT")
	symB, _ := domain.NewSymbol("ETHUSD")
	stubResult := []usecases.RankedSymbol{
		{Symbol: symA, Scores: map[string]float64{"gain": 0.5}, TotalScore: 0.5},
		{Symbol: symB, Scores: map[string]float64{"gain": 0.3}, TotalScore: 0.3},
	}
	handler := httpadapter.RankingsHandler{
		Ranker: &stubRanker{Result: stubResult},
		Symbols: []domain.Symbol{symA, symB},
		TestSeries: map[domain.Symbol]domain.CandleSeries{
			symA: domain.CandleSeries{},
			symB: domain.CandleSeries{},
		},
	}
	req := httptest.NewRequest("GET", "/api/rankings?timeframe=1h&limit=1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Timeframe string `json:"timeframe"`
		Count     int    `json:"count"`
		Results   []struct {
			Symbol     string             `json:"symbol"`
			TotalScore float64            `json:"totalScore"`
			Scores     map[string]float64 `json:"scores"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp.Count != 1 || len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Symbol != "BTCUSDT" || resp.Results[0].TotalScore != 0.5 {
		t.Errorf("unexpected result: %+v", resp.Results[0])
	}
}

func TestRankingsHandler_MissingTimeframe(t *testing.T) {
	h := &httpadapter.RankingsHandler{Ranker: &stubRanker{}}
	r := httptest.NewRequest("GET", "/api/rankings", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRankingsHandler_InvalidLimit(t *testing.T) {
	h := &httpadapter.RankingsHandler{Ranker: &stubRanker{}}
	r := httptest.NewRequest("GET", "/api/rankings?timeframe=1h&limit=bad", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
