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

func TestVolatilityHandler_LegacyFormat(t *testing.T) {
	// Write old-style flat JSON: {"buckets": [...]}
	legacy := vol.Result{
		Buckets: []vol.BucketResult{
			{MinuteOfDay: 0, AvgMove: 0.01, SpikeProb: 0.1, Normalized: 0.8},
			{MinuteOfDay: 1, AvgMove: 0.02, SpikeProb: 0.9, Normalized: 1.5},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "vol_legacy.json")
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	handler := httpAdapter.NewVolatilityHandler(path)

	req := httptest.NewRequest(http.MethodGet, "/api/volatility?timeframe=1m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tfr vol.TimeframeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &tfr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(tfr.Timeframe) != "1m" {
		t.Fatalf("expected timeframe 1m, got %s", tfr.Timeframe)
	}
	if len(tfr.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(tfr.Buckets))
	}
}

func TestVolatilityHandler_Reload_PicksUpNewData(t *testing.T) {
	dir := t.TempDir()
	path := writeVolJSON(t, dir, sampleFullResult())
	handler := httpAdapter.NewVolatilityHandler(path)

	if _, err := handler.CurrentResult(); err != nil {
		t.Fatalf("initial load: unexpected error: %v", err)
	}

	updated := sampleFullResult()
	updated.Intraday[0].Buckets[0].SpikeProb = 0.5 // was 0.1
	if err := os.WriteFile(path, mustMarshal(t, updated), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	if err := handler.Reload(); err != nil {
		t.Fatalf("Reload: unexpected error: %v", err)
	}

	result, err := handler.CurrentResult()
	if err != nil {
		t.Fatalf("CurrentResult after Reload: unexpected error: %v", err)
	}
	if result.Intraday[0].Buckets[0].SpikeProb != 0.5 {
		t.Errorf("expected the reloaded SpikeProb 0.5, got %v", result.Intraday[0].Buckets[0].SpikeProb)
	}
}

// TestVolatilityHandler_Reload_FailurePreservesOldCache is the CR follow-up
// regression test: a transient failure during Reload() (file briefly
// missing/unreadable — a momentary disk hiccup, or vol_aggregate mid-write)
// must not wipe previously-good cached data. Before this fix, Reload()
// cleared h.cached up front, so a failed reload permanently blacked out
// /api/volatility (and silently degraded VolatilitySeasonalityProvider to
// neutral) until the next successful reload, up to an hour later given
// cmd/api/main.go's periodic reload ticker.
func TestVolatilityHandler_Reload_FailurePreservesOldCache(t *testing.T) {
	dir := t.TempDir()
	path := writeVolJSON(t, dir, sampleFullResult())
	handler := httpAdapter.NewVolatilityHandler(path)

	good, err := handler.CurrentResult()
	if err != nil {
		t.Fatalf("initial load: unexpected error: %v", err)
	}
	if len(good.Intraday) == 0 {
		t.Fatal("test fixture must have loaded successfully before simulating a failure")
	}

	// Simulate a transient disk hiccup: the file becomes unreadable.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if err := handler.Reload(); err == nil {
		t.Fatal("expected Reload to fail when the file is missing")
	}

	// The handler must still serve the last-good cached data, not
	// DATA_UNAVAILABLE.
	stillGood, err := handler.CurrentResult()
	if err != nil {
		t.Fatalf("expected CurrentResult to still return the preserved cache, got error: %v", err)
	}
	if len(stillGood.Intraday) == 0 || stillGood.Intraday[0].Buckets[0].SpikeProb != good.Intraday[0].Buckets[0].SpikeProb {
		t.Errorf("expected the pre-failure cached data to survive, got %+v", stillGood)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/volatility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected ServeHTTP to still return 200 from the preserved cache, got %d: %s", rec.Code, rec.Body.String())
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
