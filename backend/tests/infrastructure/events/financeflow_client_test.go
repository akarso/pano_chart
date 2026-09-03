package events_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pano_chart/backend/infrastructure/events"
)

func TestFinanceFlowClient_FetchEvents_Success(t *testing.T) {
	resp := map[string]interface{}{
		"success": true,
		"code":    200,
		"message": "ok",
		"data": []map[string]interface{}{
			{
				"country":        "United States",
				"report_name":    "S&P Global Manufacturing PMI Final",
				"actual":         "52.7",
				"previous":       "51.2",
				"consensus":      "52.5",
				"economicImpact": "Major",
				"report_date":    "2025-03-03",
				"datetime":       "2025-03-03 14:45:00",
			},
			{
				"country":        "Germany",
				"report_name":    "CPI YoY",
				"actual":         "2.3%",
				"previous":       "2.2%",
				"consensus":      "2.3%",
				"economicImpact": "Moderate",
				"report_date":    "2025-03-03",
				"datetime":       "2025-03-03 08:00:00",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/financial-calendar" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("api_key") != "test-key" {
			t.Errorf("unexpected api_key: %s", q.Get("api_key"))
		}
		if q.Get("date_from") != "2025-03-01" {
			t.Errorf("unexpected date_from: %s", q.Get("date_from"))
		}
		if q.Get("date_to") != "2025-03-07" {
			t.Errorf("unexpected date_to: %s", q.Get("date_to"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := events.NewFinanceFlowClient("test-key", srv.URL, srv.Client())
	from := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	result, err := client.FetchEvents(context.Background(), from, to, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}

	if result[0].Country() != "United States" {
		t.Errorf("event[0].Country = %q, want %q", result[0].Country(), "United States")
	}
	if result[0].Title() != "S&P Global Manufacturing PMI Final" {
		t.Errorf("event[0].Title = %q", result[0].Title())
	}
	if result[0].Impact() != "high" {
		t.Errorf("event[0].Impact = %q, want high (Major maps to high)", result[0].Impact())
	}

	if result[1].Impact() != "medium" {
		t.Errorf("event[1].Impact = %q, want medium (Moderate maps to medium)", result[1].Impact())
	}
}

func TestFinanceFlowClient_FetchEvents_WithCountry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("country") != "United States" {
			t.Errorf("expected country param, got %q", q.Get("country"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"code":    200,
			"message": "ok",
			"data":    []interface{}{},
		})
	}))
	defer srv.Close()

	client := events.NewFinanceFlowClient("test-key", srv.URL, srv.Client())
	from := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	result, err := client.FetchEvents(context.Background(), from, to, "United States")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events, got %d", len(result))
	}
}

func TestFinanceFlowClient_FetchEvents_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"code":    401,
			"message": "invalid api key",
			"data":    nil,
		})
	}))
	defer srv.Close()

	client := events.NewFinanceFlowClient("bad-key", srv.URL, srv.Client())
	from := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	_, err := client.FetchEvents(context.Background(), from, to, "")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestFinanceFlowClient_FetchEvents_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := events.NewFinanceFlowClient("test-key", srv.URL, srv.Client())
	from := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	_, err := client.FetchEvents(context.Background(), from, to, "")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestFinanceFlowClient_FetchEvents_BadDatetimeSkipped(t *testing.T) {
	resp := map[string]interface{}{
		"success": true,
		"code":    200,
		"message": "ok",
		"data": []map[string]interface{}{
			{
				"country":        "US",
				"report_name":    "Good Event",
				"economicImpact": "Major",
				"datetime":       "2025-03-03 14:45:00",
			},
			{
				"country":        "US",
				"report_name":    "Bad Datetime Event",
				"economicImpact": "Low",
				"datetime":       "not-a-date",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := events.NewFinanceFlowClient("test-key", srv.URL, srv.Client())
	from := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	result, err := client.FetchEvents(context.Background(), from, to, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 event (bad datetime skipped), got %d", len(result))
	}
}
