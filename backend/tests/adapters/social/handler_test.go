package social_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"
)

type stubProvider struct {
	platform string
	posts    []domain.Post
}

func (s *stubProvider) Platform() string                              { return s.platform }
func (s *stubProvider) Fetch(_ domain.Account) ([]domain.Post, error) { return s.posts, nil }

func newTestService() *appsocial.Service {
	p := &stubProvider{
		platform: "twitter",
		posts:    []domain.Post{{ID: "p1", Author: "@test", Title: "hello", URL: "http://example.com/1", Timestamp: 1000}},
	}
	return appsocial.NewService(
		p,
		infrasocial.NewMemoryAccountStore(),
		infrasocial.NewMemorySubscriptionStore(),
		appsocial.NewPostCache(60*time.Second),
	)
}

func TestSubscribeHandler_Success(t *testing.T) {
	svc := newTestService()
	handler := adhttp.NewSocialSubscribeHandler(svc)

	body := `{"user_id":"u1","handle":"alice"}`
	req := httptest.NewRequest(http.MethodPost, "/api/social/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeHandler_MissingFields(t *testing.T) {
	svc := newTestService()
	handler := adhttp.NewSocialSubscribeHandler(svc)

	body := `{"user_id":"u1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/social/subscribe", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSubscribeHandler_WrongMethod(t *testing.T) {
	svc := newTestService()
	handler := adhttp.NewSocialSubscribeHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/subscribe", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestFeedHandler_Success(t *testing.T) {
	svc := newTestService()
	handler := adhttp.NewSocialFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/feed?handle=alice", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Handle string `json:"handle"`
		Count  int    `json:"count"`
		Posts  []struct {
			ID string `json:"id"`
		} `json:"posts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected 1 post, got %d", resp.Count)
	}
	if resp.Posts[0].ID != "p1" {
		t.Fatalf("expected post id 'p1', got '%s'", resp.Posts[0].ID)
	}
}

func TestFeedHandler_MissingHandle(t *testing.T) {
	svc := newTestService()
	handler := adhttp.NewSocialFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/feed", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAccountsHandler_Success(t *testing.T) {
	svc := newTestService()

	// Subscribe first.
	subHandler := adhttp.NewSocialSubscribeHandler(svc)
	body := `{"user_id":"u1","handle":"alice"}`
	subReq := httptest.NewRequest(http.MethodPost, "/api/social/subscribe", bytes.NewBufferString(body))
	subW := httptest.NewRecorder()
	subHandler.ServeHTTP(subW, subReq)

	// Now query accounts.
	handler := adhttp.NewSocialAccountsHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/social/accounts?user_id=u1", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count    int      `json:"count"`
		Accounts []string `json:"accounts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected 1 account, got %d", resp.Count)
	}
}

func TestUnsubscribeHandler_Success(t *testing.T) {
	svc := newTestService()

	// Subscribe first.
	subHandler := adhttp.NewSocialSubscribeHandler(svc)
	subBody := `{"user_id":"u1","handle":"bob"}`
	subReq := httptest.NewRequest(http.MethodPost, "/api/social/subscribe", bytes.NewBufferString(subBody))
	subW := httptest.NewRecorder()
	subHandler.ServeHTTP(subW, subReq)

	// Unsubscribe.
	handler := adhttp.NewSocialUnsubscribeHandler(svc)
	body := `{"user_id":"u1","handle":"bob"}`
	req := httptest.NewRequest(http.MethodPost, "/api/social/unsubscribe", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
