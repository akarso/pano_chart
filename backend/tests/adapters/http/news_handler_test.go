package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/domain"
)

// fakeNewsUseCase implements usecases.NewsUseCase for testing.
type fakeNewsUseCase struct {
	articles []domain.NewsArticle
	err      error
}

func (f *fakeNewsUseCase) List(ctx context.Context, limit int) ([]domain.NewsArticle, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit > 0 && limit < len(f.articles) {
		return f.articles[:limit], nil
	}
	return f.articles, nil
}

func (f *fakeNewsUseCase) GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error) {
	if f.err != nil {
		return domain.NewsArticle{}, f.err
	}
	for _, a := range f.articles {
		if a.Slug() == slug {
			return a, nil
		}
	}
	return domain.NewsArticle{}, fmt.Errorf("news article not found: %s", slug)
}

func makeTestNewsArticle(t *testing.T, slug, title string, date time.Time, status domain.NewsStatus) domain.NewsArticle {
	t.Helper()
	a, err := domain.NewNewsArticle(slug, title, date, status, []string{"test"}, "Some body text.", nil, "")
	if err != nil {
		t.Fatalf("makeTestNewsArticle: %v", err)
	}
	return a
}

func TestNewsHandler_List_Success(t *testing.T) {
	d1 := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	uc := &fakeNewsUseCase{
		articles: []domain.NewsArticle{
			makeTestNewsArticle(t, "add-1w-timeframe", "1W Timeframe", d1, domain.NewsStatusPlanned),
			makeTestNewsArticle(t, "fear-greed", "Fear & Greed", d2, domain.NewsStatusReleased),
		},
	}

	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news?limit=10", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp []struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Date    string `json:"date"`
		Status  string `json:"status"`
		Excerpt string `json:"excerpt"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(resp))
	}
	if resp[0].Slug != "add-1w-timeframe" {
		t.Errorf("resp[0].Slug = %q", resp[0].Slug)
	}
	if resp[0].Status != "planned" {
		t.Errorf("resp[0].Status = %q", resp[0].Status)
	}
	if resp[0].Date != "2026-03-05" {
		t.Errorf("resp[0].Date = %q", resp[0].Date)
	}
}

func TestNewsHandler_List_EmptyResult(t *testing.T) {
	uc := &fakeNewsUseCase{articles: []domain.NewsArticle{}}
	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp []interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp))
	}
}

func TestNewsHandler_List_InvalidLimit(t *testing.T) {
	uc := &fakeNewsUseCase{}
	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news?limit=abc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNewsHandler_Single_Success(t *testing.T) {
	d := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	eta := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	a, _ := domain.NewNewsArticle("add-1w-timeframe", "1W Timeframe", d, domain.NewsStatusPlanned, []string{"feature"}, "Markdown body here.", &eta, "normal")
	uc := &fakeNewsUseCase{articles: []domain.NewsArticle{a}}

	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news/add-1w-timeframe", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Slug     string   `json:"slug"`
		Title    string   `json:"title"`
		Date     string   `json:"date"`
		Status   string   `json:"status"`
		Tags     []string `json:"tags"`
		Body     string   `json:"body"`
		ETA      string   `json:"eta"`
		Priority string   `json:"priority"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Slug != "add-1w-timeframe" {
		t.Errorf("slug = %q", resp.Slug)
	}
	if resp.Body != "Markdown body here." {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.ETA != "2026-03-20" {
		t.Errorf("eta = %q", resp.ETA)
	}
	if resp.Priority != "normal" {
		t.Errorf("priority = %q", resp.Priority)
	}
	if len(resp.Tags) != 1 || resp.Tags[0] != "feature" {
		t.Errorf("tags = %v", resp.Tags)
	}
}

func TestNewsHandler_Single_NotFound(t *testing.T) {
	uc := &fakeNewsUseCase{articles: []domain.NewsArticle{}}
	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestNewsHandler_MethodNotAllowed(t *testing.T) {
	uc := &fakeNewsUseCase{}
	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodPost, "/api/news", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestNewsHandler_List_WithETA(t *testing.T) {
	d := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	eta := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	a, _ := domain.NewNewsArticle("s", "T", d, domain.NewsStatusPlanned, nil, "body", &eta, "")
	uc := &fakeNewsUseCase{articles: []domain.NewsArticle{a}}

	handler := adhttp.NewNewsHandler(uc)
	req := httptest.NewRequest(http.MethodGet, "/api/news", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp []struct {
		ETA string `json:"eta"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp[0].ETA != "2026-03-20" {
		t.Errorf("eta = %q", resp[0].ETA)
	}
}
