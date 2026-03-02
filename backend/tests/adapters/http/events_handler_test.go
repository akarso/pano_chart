package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type fakeEventsUseCase struct {
	events  []domain.Event
	err     error
	lastReq usecases.GetEventsRequest
}

func (f *fakeEventsUseCase) Execute(ctx context.Context, req usecases.GetEventsRequest) ([]domain.Event, error) {
	f.lastReq = req
	return f.events, f.err
}

func makeTestEvent(t *testing.T, country, title string, impact domain.EventImpact, ts time.Time) domain.Event {
	t.Helper()
	ev, err := domain.NewEvent("", country, title, impact, ts)
	if err != nil {
		t.Fatalf("makeTestEvent: %v", err)
	}
	return ev
}

func TestEventsHandler_Success(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	uc := &fakeEventsUseCase{
		events: []domain.Event{
			makeTestEvent(t, "United States", "PMI Final", domain.EventImpactHigh, ts),
			makeTestEvent(t, "Germany", "CPI YoY", domain.EventImpactMedium, ts.Add(time.Hour)),
		},
	}

	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?date_from=2025-03-01&date_to=2025-03-07", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Events []struct {
			ID        string `json:"id"`
			Country   string `json:"country"`
			Title     string `json:"title"`
			Impact    string `json:"impact"`
			Timestamp string `json:"timestamp"`
		} `json:"events"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}
	if resp.Events[0].Country != "United States" {
		t.Errorf("events[0].Country = %q", resp.Events[0].Country)
	}
	if resp.Events[0].Impact != "high" {
		t.Errorf("events[0].Impact = %q, want %q", resp.Events[0].Impact, "high")
	}
	if resp.Events[0].Timestamp != "2025-03-03T14:45:00Z" {
		t.Errorf("events[0].Timestamp = %q", resp.Events[0].Timestamp)
	}
}

func TestEventsHandler_DefaultDateRange(t *testing.T) {
	uc := &fakeEventsUseCase{events: []domain.Event{}}

	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	now := time.Now().UTC()

	// DateFrom defaults to yesterday
	expectedFrom := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	actualFrom := uc.lastReq.DateFrom.Truncate(24 * time.Hour)
	diffFrom := expectedFrom.Sub(actualFrom)
	if diffFrom < -24*time.Hour || diffFrom > 24*time.Hour {
		t.Errorf("default DateFrom off by more than a day: got %v, expected ~%v", actualFrom, expectedFrom)
	}

	// DateTo defaults to tomorrow (+1 day)
	expectedTo := now.AddDate(0, 0, 1).Truncate(24 * time.Hour)
	actualTo := uc.lastReq.DateTo.Truncate(24 * time.Hour)
	diffTo := expectedTo.Sub(actualTo)
	if diffTo < -24*time.Hour || diffTo > 24*time.Hour {
		t.Errorf("default DateTo off by more than a day: got %v, expected ~%v", actualTo, expectedTo)
	}

	// Country defaults to "United States"
	if uc.lastReq.Country != "United States" {
		t.Errorf("default country = %q, want %q", uc.lastReq.Country, "United States")
	}
}

func TestEventsHandler_InvalidDateFrom(t *testing.T) {
	uc := &fakeEventsUseCase{}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?date_from=bad-date", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEventsHandler_InvalidDateTo(t *testing.T) {
	uc := &fakeEventsUseCase{}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?date_to=bad-date", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEventsHandler_DateToBeforeDateFrom(t *testing.T) {
	uc := &fakeEventsUseCase{}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?date_from=2025-03-07&date_to=2025-03-01", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEventsHandler_EmptyResponse(t *testing.T) {
	uc := &fakeEventsUseCase{events: []domain.Event{}}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?date_from=2025-03-01&date_to=2025-03-07", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Events []interface{} `json:"events"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(resp.Events))
	}
}

func TestEventsHandler_MethodNotAllowed(t *testing.T) {
	uc := &fakeEventsUseCase{}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestEventsHandler_PassesQueryParams(t *testing.T) {
	uc := &fakeEventsUseCase{events: []domain.Event{}}
	handler := adhttp.NewEventsHandler(uc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/events?date_from=2025-03-01&date_to=2025-03-07&impact=high&country=Germany", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if uc.lastReq.Impact != "high" {
		t.Errorf("impact = %q, want %q", uc.lastReq.Impact, "high")
	}
	if uc.lastReq.Country != "Germany" {
		t.Errorf("country = %q, want %q", uc.lastReq.Country, "Germany")
	}
}
