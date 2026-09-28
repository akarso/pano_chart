package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
)

type stubHandler struct {
	called bool
	path   string
}

func (s *stubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.called = true
	s.path = r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func TestSymbolRouter_RegimesSuffixRoutesToRegimesHandler(t *testing.T) {
	detail := &stubHandler{}
	regimes := &stubHandler{}
	router := httpAdapter.NewSymbolRouter(detail, regimes)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/regimes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !regimes.called {
		t.Fatal("expected the regimes handler to be called")
	}
	if detail.called {
		t.Fatal("detail handler must not be called for a /regimes path")
	}
}

func TestSymbolRouter_BareSymbolRoutesToDetailHandler(t *testing.T) {
	detail := &stubHandler{}
	regimes := &stubHandler{}
	router := httpAdapter.NewSymbolRouter(detail, regimes)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !detail.called {
		t.Fatal("expected the detail handler to be called for a bare symbol path")
	}
	if regimes.called {
		t.Fatal("regimes handler must not be called for a bare symbol path")
	}
}
