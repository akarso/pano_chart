package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/plan"
)

type stubPlanEval struct {
	result plan.PlanResult
	err    error
}

func (s *stubPlanEval) Evaluate(_ context.Context, _, _ string, risk float64) (plan.PlanResult, error) {
	if s.err != nil {
		return plan.PlanResult{}, s.err
	}
	out := s.result
	if risk > 0 && out.Plan.Valid {
		out.Size = plan.Size(risk, out.Plan.LongEntry, out.Plan.LongStop)
		out.ShortSize = plan.Size(risk, out.Plan.ShortEntry, out.Plan.ShortStop)
	}
	return out, nil
}

func TestPlanHandler_HappyPath(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 1, 0.8)
	h := httpAdapter.NewPlanHandler(&stubPlanEval{result: plan.PlanResult{Plan: p}})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan?timeframe=1h&risk=100", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["valid"] != true {
		t.Fatalf("valid=%v", body["valid"])
	}
	if body["size"].(float64) != 80 {
		t.Fatalf("size=%v want 80", body["size"])
	}
	if body["longEntry"].(float64) != 100.25 {
		t.Fatalf("longEntry=%v", body["longEntry"])
	}
	if body["longTargetFull"].(float64) != 109.75 {
		t.Fatalf("longTargetFull=%v", body["longTargetFull"])
	}
	if body["shortSize"].(float64) != 80 {
		t.Fatalf("shortSize=%v", body["shortSize"])
	}
}

func TestPlanHandler_InvalidPlanZerosLevels(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 1, 0.2)
	h := httpAdapter.NewPlanHandler(&stubPlanEval{result: plan.PlanResult{Plan: p}})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan?timeframe=1h&risk=100", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["valid"] != false {
		t.Fatal("want valid=false")
	}
	if body["reason"] != "range quality" {
		t.Fatalf("reason=%v", body["reason"])
	}
	if body["longEntry"].(float64) != 0 || body["size"].(float64) != 0 {
		t.Fatalf("levels/size must be zero when invalid: longEntry=%v size=%v",
			body["longEntry"], body["size"])
	}
}

func TestPlanHandler_MissingRiskSizeZero(t *testing.T) {
	p := plan.BuildRangePlanFromBounds("BTCUSDT", "1h", 100, 110, 105, 1, 0.8)
	h := httpAdapter.NewPlanHandler(&stubPlanEval{result: plan.PlanResult{Plan: p}})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["size"].(float64) != 0 {
		t.Fatalf("size=%v want 0 without risk", body["size"])
	}
}

func TestPlanHandler_InvalidRisk(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan?risk=-1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestPlanHandler_MethodNotAllowed(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{})
	req := httptest.NewRequest(http.MethodPost, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestPlanHandler_DataUnavailable(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{err: plan.ErrDataUnavailable})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want 422", rec.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	errObj, _ := body["error"].(map[string]interface{})
	if errObj["code"] != "DATA_UNAVAILABLE" {
		t.Fatalf("code=%v body=%s", errObj["code"], rec.Body.String())
	}
}

func TestPlanHandler_ClientCanceled(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{err: context.Canceled})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 499 {
		t.Fatalf("status=%d want 499", rec.Code)
	}
}

func TestPlanHandler_DeadlineExceeded(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{err: context.DeadlineExceeded})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d want 504", rec.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	errObj, _ := body["error"].(map[string]interface{})
	if errObj["code"] != "DEADLINE_EXCEEDED" {
		t.Fatalf("code=%v body=%s", errObj["code"], rec.Body.String())
	}
}

func TestPlanHandler_NilEvaluator(t *testing.T) {
	h := httpAdapter.NewPlanHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rec.Code)
	}
}

func TestPlanHandler_WrappedDataUnavailable(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{
		err: errors.Join(plan.ErrDataUnavailable, errors.New("upstream")),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestPlanHandler_InvalidTimeframe(t *testing.T) {
	h := httpAdapter.NewPlanHandler(&stubPlanEval{})
	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan?timeframe=bogus", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestSymbolRouter_PlanSuffix(t *testing.T) {
	detail := &stubHandler{}
	regimes := &stubHandler{}
	plans := &stubHandler{}
	router := httpAdapter.NewSymbolRouter(detail, regimes)
	router.SetPlanHandler(plans)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if !plans.called {
		t.Fatal("expected plan handler")
	}
	if detail.called || regimes.called {
		t.Fatal("detail/regimes must not handle /plan")
	}
}

func TestSymbolRouter_PlanUnsetReturns503(t *testing.T) {
	detail := &stubHandler{}
	regimes := &stubHandler{}
	router := httpAdapter.NewSymbolRouter(detail, regimes)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/plan", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if detail.called {
		t.Fatal("must not fall through to detail")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503 body=%s", rec.Code, rec.Body.String())
	}
}
