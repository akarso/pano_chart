package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	appsignal "pano_chart/backend/application/signal"
	infrasignal "pano_chart/backend/infrastructure/signal"
)

type fakeScorecardAPI struct {
	card     appsignal.Scorecard
	sum      appsignal.SummaryResult
	err      error
	lastGet  [4]string
	lastSum  [2]string
	getCalls int
}

func (f *fakeScorecardAPI) Get(_ context.Context, kind, label, tf, sinceRaw string) (appsignal.Scorecard, error) {
	f.getCalls++
	f.lastGet = [4]string{kind, label, tf, sinceRaw}
	return f.card, f.err
}
func (f *fakeScorecardAPI) Summary(_ context.Context, tf, sinceRaw string) (appsignal.SummaryResult, error) {
	f.lastSum = [2]string{tf, sinceRaw}
	return f.sum, f.err
}

func TestScorecardHandler_GetJSON(t *testing.T) {
	base := 0.4
	api := &fakeScorecardAPI{card: appsignal.Scorecard{
		Kind: "badge", Label: "trend_up", Timeframe: "1h",
		Total: 20, Hits: 10, HitRate: 0.5, Baseline: &base,
		Buckets: []appsignal.Bucket{{Lo: 0, Hi: 0.1, N: 2, Hits: 1, HitRate: 0.5}},
		Since:   time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
	}}
	h := adhttp.NewScorecardHandler(api)
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=trend_up&timeframe=1h&since=30d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%q", ct)
	}
	if api.lastGet[3] != "30d" {
		t.Fatalf("sinceRaw passed=%q", api.lastGet[3])
	}
	var got appsignal.Scorecard
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "badge" || got.Label != "trend_up" || got.Total != 20 || got.Baseline == nil || *got.Baseline != 0.4 {
		t.Fatalf("%+v", got)
	}
}

func TestScorecardHandler_GetNullBaseline(t *testing.T) {
	api := &fakeScorecardAPI{card: appsignal.Scorecard{
		Kind: "badge", Label: "trend_up", Total: 5, Hits: 3, HitRate: 0.6, Baseline: nil,
	}}
	h := adhttp.NewScorecardHandler(api)
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=trend_up", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["baseline"]) != "null" {
		t.Fatalf("baseline=%s", raw["baseline"])
	}
}

func TestScorecardHandler_SummaryJSON(t *testing.T) {
	api := &fakeScorecardAPI{sum: appsignal.SummaryResult{
		Timeframe: "1h",
		Since:     time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		SinceRaw:  "30d",
		Items: []appsignal.SummaryRow{
			{Kind: "badge", Label: "trend_up", HitRate: 0.58, N: 412},
		},
	}}
	h := adhttp.NewScorecardHandler(api)
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards/summary?timeframe=1h", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if api.lastSum[1] != "30d" {
		t.Fatalf("default sinceRaw=%q", api.lastSum[1])
	}
	var got appsignal.SummaryResult
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Timeframe != "1h" || got.SinceRaw != "30d" || len(got.Items) != 1 || got.Items[0].N != 412 {
		t.Fatalf("%+v", got)
	}
	if !got.Since.Equal(time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("since must come from cached payload, got %v", got.Since)
	}
}

func TestScorecardHandler_requiresKindLabel(t *testing.T) {
	h := adhttp.NewScorecardHandler(&fakeScorecardAPI{})
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "" {
		t.Fatalf("body=%v", body)
	}
}

func TestScorecardHandler_redisGetCancelIs499(t *testing.T) {
	started := make(chan struct{})
	redis := &blockGetRedis{started: started}
	next := &fakeScorecardAPI{card: appsignal.Scorecard{Kind: "badge", Label: "trend_up", Total: 1}}
	cache := infrasignal.NewRedisCachedScorecard(next, redis, "scorecards")
	h := adhttp.NewScorecardHandler(cache)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=trend_up&timeframe=1h", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rr, req)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("redis Get never blocked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return")
	}
	if rr.Code != 499 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if next.getCalls != 0 {
		t.Fatalf("next calls=%d", next.getCalls)
	}
}

type blockGetRedis struct {
	started chan struct{}
	once    sync.Once
}

func (b *blockGetRedis) Get(ctx context.Context, _ string) (string, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return "", ctx.Err()
}
func (b *blockGetRedis) Set(context.Context, string, string, time.Duration) error { return nil }

func TestScorecardHandler_rejectsBadKind(t *testing.T) {
	h := adhttp.NewScorecardHandler(&fakeScorecardAPI{})
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=other&label=x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestScorecardHandler_rejectsBadLabel(t *testing.T) {
	h := adhttp.NewScorecardHandler(&fakeScorecardAPI{})
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=a%7Cb", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestScorecardHandler_rejectsGarbageSince(t *testing.T) {
	h := adhttp.NewScorecardHandler(&fakeScorecardAPI{})
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=trend_up&since=30dgarbage", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestScorecardHandler_canceledWritesNothing(t *testing.T) {
	api := &fakeScorecardAPI{err: context.Canceled}
	h := adhttp.NewScorecardHandler(api)
	req := httptest.NewRequest(http.MethodGet, "/api/scorecards?kind=badge&label=trend_up", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 499 {
		t.Fatalf("status=%d want 499", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "" {
		t.Fatalf("body=%v", body)
	}
}
