package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	adhttp "pano_chart/backend/adapters/http"
	appscoring "pano_chart/backend/application/scoring"
)

type fakeDistribution struct {
	dist  []float64
	names []string
	err   error
	calc  string
	tf    string
}

func (f *fakeDistribution) Distribution(_ context.Context, calculator, tf string) ([]float64, error) {
	f.calc = calculator
	f.tf = tf
	return f.dist, f.err
}

func (f *fakeDistribution) Calculators(context.Context) ([]string, error) {
	return f.names, nil
}

func debugReq(method, url string) *http.Request {
	return httptest.NewRequest(method, url, nil)
}

func TestScoreDistributionHandler_JSON(t *testing.T) {
	api := &fakeDistribution{dist: []float64{0.1, 0.2}}
	h := adhttp.NewScoreDistributionHandler(api)
	req := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=sideways&timeframe=1H")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if api.calc != "sideways" || api.tf != "1h" {
		t.Fatalf("query calc=%q tf=%q", api.calc, api.tf)
	}
	var got struct {
		Calculator   string    `json:"calculator"`
		Timeframe    string    `json:"timeframe"`
		Distribution []float64 `json:"distribution"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Calculator != "sideways" || got.Timeframe != "1h" || len(got.Distribution) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestScoreDistributionHandler_Errors(t *testing.T) {
	h := adhttp.NewScoreDistributionHandler(&fakeDistribution{
		err:   appscoring.ErrNoSamples,
		names: []string{"Sideways Consistency"},
	})

	missing := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=sideways")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing status=%d", rr.Code)
	}

	post := debugReq(http.MethodPost, "/api/debug/score-distribution?calculator=sideways&timeframe=1h")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status=%d", rr.Code)
	}

	none := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=Sideways%20Consistency&timeframe=1h")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, none)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("empty status=%d body=%s", rr.Code, rr.Body.String())
	}

	unknown := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=nope&timeframe=1h")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, unknown)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown status=%d", rr.Code)
	}
	var body struct {
		Error       string   `json:"error"`
		Calculators []string `json:"calculators"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "unknown calculator" || len(body.Calculators) != 1 || body.Calculators[0] != "Sideways Consistency" {
		t.Fatalf("%+v", body)
	}

	h = adhttp.NewScoreDistributionHandler(&fakeDistribution{err: errors.New("db")})
	fail := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=sideways&timeframe=1h")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, fail)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("fail status=%d", rr.Code)
	}
}

func TestScoreDistributionHandler_RemoteAddrIsNotTheGate(t *testing.T) {
	// A same-host reverse proxy presents RemoteAddr as 127.0.0.1 for an
	// external client. The handler answers either peer. The debug route is
	// kept off the public mux and bound with LoopbackListenAddr instead.
	h := adhttp.NewScoreDistributionHandler(&fakeDistribution{dist: []float64{0.1}})
	for _, peer := range []string{"127.0.0.1:4321", "203.0.113.5:4321"} {
		req := debugReq(http.MethodGet, "/api/debug/score-distribution?calculator=sideways&timeframe=1h")
		req.RemoteAddr = peer
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", peer, rr.Code)
		}
	}
}

func TestLoopbackListenAddr(t *testing.T) {
	got, err := adhttp.LoopbackListenAddr("")
	if err != nil || got != "127.0.0.1:8081" {
		t.Fatalf("default=%q err=%v", got, err)
	}
	got, err = adhttp.LoopbackListenAddr("127.0.0.1:9090")
	if err != nil || got != "127.0.0.1:9090" {
		t.Fatalf("v4=%q err=%v", got, err)
	}
	got, err = adhttp.LoopbackListenAddr("[::1]:9090")
	if err != nil || got != "[::1]:9090" {
		t.Fatalf("v6=%q err=%v", got, err)
	}
	for _, addr := range []string{"0.0.0.0:8081", ":8081", "203.0.113.5:8081"} {
		if _, err := adhttp.LoopbackListenAddr(addr); err == nil {
			t.Fatalf("%s accepted", addr)
		}
	}
}

func TestLoopbackListenerBindsLoopbackOnly(t *testing.T) {
	addr, err := adhttp.LoopbackListenAddr("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() {
		t.Fatalf("listener=%v", ln.Addr())
	}
}
