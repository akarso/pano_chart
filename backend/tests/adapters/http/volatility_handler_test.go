package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	vol "pano_chart/backend/infrastructure/volatility"
)

func writeVolJSON(t *testing.T, dir string, result vol.FullResult) string {
	t.Helper()
	path := filepath.Join(dir, "vol_test.json")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func sampleFullResult() vol.FullResult {
	return vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{
				Timeframe: vol.TF1m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 0, AvgMove: 0.01, SpikeProb: 0.1, Normalized: 0.8},
					{MinuteOfDay: 1, AvgMove: 0.02, SpikeProb: 0.9, Normalized: 1.5},
				},
			},
			{
				Timeframe: vol.TF5m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 0, AvgMove: 0.05, SpikeProb: 0.3, Normalized: 1.0},
				},
			},
		},
		Weekly: vol.WeeklyResult{
			Buckets: []vol.WeeklyBucket{
				{MinuteOfWeek: 0, AvgMove: 0.01, SpikeProb: 0.1, Normalized: 1.0},
			},
		},
	}
}

func TestVolatilityHandler_Returns1mByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeVolJSON(t, dir, sampleFullResult())
	handler := httpAdapter.NewVolatilityHandler(path)

	req := httptest.NewRequest(http.MethodGet, "/api/volatility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp vol.TimeframeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Timeframe != vol.TF1m {
		t.Errorf("expected 1m, got %s", resp.Timeframe)
	}
	if len(resp.Buckets) != 2 {
		t.Errorf("expected 2 buckets, got %d", len(resp.Buckets))
	}
}

func TestVolatilityHandler_TimeframeParam(t *testing.T) {
	dir := t.TempDir()
	path := writeVolJSON(t, dir, sampleFullResult())
	handler := httpAdapter.NewVolatilityHandler(path)

	req := httptest.NewRequest(http.MethodGet, "/api/volatility?timeframe=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp vol.TimeframeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Timeframe != vol.TF5m {
		t.Errorf("expected 5m, got %s", resp.Timeframe)
	}
}

func TestVolatilityHandler_UnknownTimeframe(t *testing.T) {
	dir := t.TempDir()
	path := writeVolJSON(t, dir, sampleFullResult())
	handler := httpAdapter.NewVolatilityHandler(path)

	req := httptest.NewRequest(http.MethodGet, "/api/volatility?timeframe=99m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestVolatilityHandler_MethodNotAllowed(t *testing.T) {
	handler := httpAdapter.NewVolatilityHandler("/nonexistent")

	req := httptest.NewRequest(http.MethodPost, "/api/volatility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestVolatilityHandler_MissingFile(t *testing.T) {
	handler := httpAdapter.NewVolatilityHandler("/nonexistent/vol.json")

	req := httptest.NewRequest(http.MethodGet, "/api/volatility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
