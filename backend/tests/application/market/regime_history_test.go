package market_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/regimehistory"
	"pano_chart/backend/application/market/transition"
	mkt "pano_chart/backend/domain/market"
)

// ---------- SQLite repository ----------

func newTestRepo(t *testing.T) *regimehistory.SQLiteRepository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	repo, err := regimehistory.NewSQLiteRepositoryFromDB(db)
	if err != nil {
		t.Fatalf("creating repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestSQLiteRepository_AppendAndGetLatest(t *testing.T) {
	repo := newTestRepo(t)

	// No history yet.
	latest, err := repo.GetLatest("4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest != nil {
		t.Fatal("expected nil for empty history")
	}

	// Append a period.
	err = repo.Append("4h", mkt.RegimePeriod{
		Regime:          mkt.RegimeCompression,
		StartTimestamp:  1000,
		DurationCandles: 1,
	})
	if err != nil {
		t.Fatalf("append error: %v", err)
	}

	latest, err = repo.GetLatest("4h")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil")
		return
	}
	if latest.Regime != mkt.RegimeCompression {
		t.Errorf("regime: got %q, want compression", latest.Regime)
	}
	if latest.DurationCandles != 1 {
		t.Errorf("duration: got %d, want 1", latest.DurationCandles)
	}
	if latest.EndTimestamp != nil {
		t.Error("expected nil end timestamp for open period")
	}
}

func TestSQLiteRepository_UpdateDuration(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeSideways, StartTimestamp: 1000, DurationCandles: 1,
	})

	err := repo.UpdateDuration("4h", 5)
	if err != nil {
		t.Fatalf("update duration: %v", err)
	}

	latest, _ := repo.GetLatest("4h")
	if latest.DurationCandles != 5 {
		t.Errorf("duration: got %d, want 5", latest.DurationCandles)
	}
}

func TestSQLiteRepository_CloseCurrent(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeTrend, StartTimestamp: 1000, DurationCandles: 3,
	})

	err := repo.CloseCurrent("4h", 2000)
	if err != nil {
		t.Fatalf("close current: %v", err)
	}

	latest, _ := repo.GetLatest("4h")
	if latest.EndTimestamp == nil || *latest.EndTimestamp != 2000 {
		t.Errorf("end timestamp: got %v, want 2000", latest.EndTimestamp)
	}
}

func TestSQLiteRepository_GetHistory(t *testing.T) {
	repo := newTestRepo(t)

	// Insert 3 periods.
	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeSideways, StartTimestamp: 1000, DurationCandles: 5,
	})
	_ = repo.CloseCurrent("4h", 2000)

	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeCompression, StartTimestamp: 2000, DurationCandles: 10,
	})
	_ = repo.CloseCurrent("4h", 3000)

	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeTrend, StartTimestamp: 3000, DurationCandles: 3,
	})

	periods, err := repo.GetHistory("4h", 50)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %d", len(periods))
	}

	// Should be oldest-first.
	if periods[0].Regime != mkt.RegimeSideways {
		t.Errorf("first period: got %q", periods[0].Regime)
	}
	if periods[2].Regime != mkt.RegimeTrend {
		t.Errorf("last period: got %q", periods[2].Regime)
	}
	// Last period should still be open.
	if periods[2].EndTimestamp != nil {
		t.Error("last period should be open")
	}
}

func TestSQLiteRepository_GetHistory_respectsLimit(t *testing.T) {
	repo := newTestRepo(t)

	for i := 0; i < 5; i++ {
		ts := int64(1000 + i*1000)
		_ = repo.Append("4h", mkt.RegimePeriod{
			Regime: mkt.RegimeSideways, StartTimestamp: ts, DurationCandles: 1,
		})
		if i < 4 {
			_ = repo.CloseCurrent("4h", ts+500)
		}
	}

	periods, _ := repo.GetHistory("4h", 2)
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods, got %d", len(periods))
	}
	// Should be the most recent 2, ordered oldest-first.
	if periods[0].StartTimestamp != 4000 {
		t.Errorf("expected start=4000, got %d", periods[0].StartTimestamp)
	}
}

func TestSQLiteRepository_timeframeIsolation(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.Append("4h", mkt.RegimePeriod{
		Regime: mkt.RegimeTrend, StartTimestamp: 1000, DurationCandles: 5,
	})
	_ = repo.Append("1h", mkt.RegimePeriod{
		Regime: mkt.RegimeCompression, StartTimestamp: 2000, DurationCandles: 3,
	})

	latest4h, _ := repo.GetLatest("4h")
	latest1h, _ := repo.GetLatest("1h")

	if latest4h.Regime != mkt.RegimeTrend {
		t.Errorf("4h: got %q", latest4h.Regime)
	}
	if latest1h.Regime != mkt.RegimeCompression {
		t.Errorf("1h: got %q", latest1h.Regime)
	}
}

// ---------- Tracker ----------

func TestTracker_firstObservation(t *testing.T) {
	repo := newTestRepo(t)
	tracker := regimehistory.NewTracker(repo)

	err := tracker.Update("4h", mkt.RegimeCompression, 1000)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	latest, _ := repo.GetLatest("4h")
	if latest == nil {
		t.Fatal("expected non-nil")
		return
	}
	if latest.Regime != mkt.RegimeCompression {
		t.Errorf("regime: %q", latest.Regime)
	}
	if latest.DurationCandles != 1 {
		t.Errorf("duration: %d", latest.DurationCandles)
	}
}

func TestTracker_sameRegimeIncrementsDuration(t *testing.T) {
	repo := newTestRepo(t)
	tracker := regimehistory.NewTracker(repo)

	_ = tracker.Update("4h", mkt.RegimeTrend, 1000)
	_ = tracker.Update("4h", mkt.RegimeTrend, 2000)
	_ = tracker.Update("4h", mkt.RegimeTrend, 3000)

	latest, _ := repo.GetLatest("4h")
	if latest.DurationCandles != 3 {
		t.Errorf("duration: got %d, want 3", latest.DurationCandles)
	}
}

func TestTracker_regimeChangeClosesAndOpensNew(t *testing.T) {
	repo := newTestRepo(t)
	tracker := regimehistory.NewTracker(repo)

	_ = tracker.Update("4h", mkt.RegimeCompression, 1000)
	_ = tracker.Update("4h", mkt.RegimeCompression, 2000)
	_ = tracker.Update("4h", mkt.RegimeTrend, 3000)

	periods, _ := repo.GetHistory("4h", 50)
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods, got %d", len(periods))
	}

	// First period should be closed.
	if periods[0].EndTimestamp == nil || *periods[0].EndTimestamp != 3000 {
		t.Errorf("first period end: %v", periods[0].EndTimestamp)
	}
	if periods[0].DurationCandles != 2 {
		t.Errorf("first period duration: %d", periods[0].DurationCandles)
	}

	// Second period should be open.
	if periods[1].Regime != mkt.RegimeTrend {
		t.Errorf("second period regime: %q", periods[1].Regime)
	}
	if periods[1].EndTimestamp != nil {
		t.Error("second period should be open")
	}
	if periods[1].DurationCandles != 1 {
		t.Errorf("second period duration: %d", periods[1].DurationCandles)
	}
}

// ---------- Service ----------

func TestService_GetHistory(t *testing.T) {
	repo := newTestRepo(t)
	svc := regimehistory.NewService(repo)

	// Empty history.
	h, err := svc.GetHistory("4h", 50)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if h.CurrentAge != 0 {
		t.Errorf("age: got %d, want 0", h.CurrentAge)
	}
	if len(h.Periods) != 0 {
		t.Errorf("expected 0 periods, got %d", len(h.Periods))
	}

	// Add some history.
	tracker := regimehistory.NewTracker(repo)
	_ = tracker.Update("4h", mkt.RegimeSideways, 1000)
	_ = tracker.Update("4h", mkt.RegimeSideways, 2000)
	_ = tracker.Update("4h", mkt.RegimeCompression, 3000)

	h, _ = svc.GetHistory("4h", 50)
	if h.CurrentAge != 1 {
		t.Errorf("current age: got %d, want 1", h.CurrentAge)
	}
	if len(h.Periods) != 2 {
		t.Errorf("expected 2 periods, got %d", len(h.Periods))
	}
	if h.Timeframe != "4h" {
		t.Errorf("timeframe: %q", h.Timeframe)
	}
}

func TestService_CurrentAge(t *testing.T) {
	repo := newTestRepo(t)
	svc := regimehistory.NewService(repo)

	age, _ := svc.CurrentAge("4h")
	if age != 0 {
		t.Errorf("expected 0, got %d", age)
	}

	tracker := regimehistory.NewTracker(repo)
	_ = tracker.Update("4h", mkt.RegimeTrend, 1000)
	_ = tracker.Update("4h", mkt.RegimeTrend, 2000)
	_ = tracker.Update("4h", mkt.RegimeTrend, 3000)

	age, _ = svc.CurrentAge("4h")
	if age != 3 {
		t.Errorf("expected 3, got %d", age)
	}
}

// ---------- TransitionService with AgeProvider ----------

func TestTransitionService_usesAgeProvider(t *testing.T) {
	repo := newTestRepo(t)
	tracker := regimehistory.NewTracker(repo)
	historySvc := regimehistory.NewService(repo)

	// Build up regime history: 20 candles of compression.
	for i := 0; i < 20; i++ {
		_ = tracker.Update("4h", mkt.RegimeCompression, int64(1000+i*100))
	}

	provider := &fakeTransitionRegimeProvider{
		summary: mkt.RegimeSummary{
			Regime:     mkt.RegimeCompression,
			Prevalence: 0.8,
			Metrics: mkt.RegimeMetrics{
				CompressionBreadth:  0.4,
				VolatilityExpansion: 1.2,
			},
		},
	}

	eng := transition.NewTransitionEngine()
	svc := transition.NewTransitionService(provider, eng)
	svc.SetAgeProvider(historySvc)

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// With 20 candles history, horizon should reflect that.
	if result.Horizon != "20 candles (~3.3d)" {
		t.Errorf("horizon: got %q, want %q", result.Horizon, "20 candles (~3.3d)")
	}

	// Probabilities should be non-negative and sum to 1.
	sum := result.Probabilities.Trend + result.Probabilities.Sideways + result.Probabilities.Expansion
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("probability sum: %f", sum)
	}
}

// ---------- HTTP handler ----------

type fakeHistoryProvider struct {
	history mkt.RegimeHistory
	err     error
}

func (f *fakeHistoryProvider) GetHistory(tf string, limit int) (mkt.RegimeHistory, error) {
	if f.err != nil {
		return mkt.RegimeHistory{}, f.err
	}
	h := f.history
	h.Timeframe = tf
	return h, nil
}

func TestRegimeHistoryHandler_success(t *testing.T) {
	end := int64(2000)
	provider := &fakeHistoryProvider{
		history: mkt.RegimeHistory{
			Periods: []mkt.RegimePeriod{
				{Regime: mkt.RegimeSideways, StartTimestamp: 1000, EndTimestamp: &end, DurationCandles: 5},
				{Regime: mkt.RegimeCompression, StartTimestamp: 2000, DurationCandles: 7},
			},
			CurrentAge: 7,
		},
	}
	handler := adhttp.NewMarketRegimeHistoryHandler(provider)

	req := httptest.NewRequest("GET", "/api/market/regime/history?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}

	var resp struct {
		Timeframe  string `json:"timeframe"`
		CurrentAge int    `json:"currentAge"`
		History    []struct {
			Regime          string `json:"regime"`
			Start           int64  `json:"start"`
			End             *int64 `json:"end"`
			DurationCandles int    `json:"durationCandles"`
		} `json:"history"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Timeframe != "1h" {
		t.Errorf("timeframe: %q", resp.Timeframe)
	}
	if resp.CurrentAge != 7 {
		t.Errorf("currentAge: %d", resp.CurrentAge)
	}
	if len(resp.History) != 2 {
		t.Fatalf("history len: %d", len(resp.History))
	}
	if resp.History[0].Regime != "sideways" {
		t.Errorf("first regime: %q", resp.History[0].Regime)
	}
	if resp.History[0].End == nil || *resp.History[0].End != 2000 {
		t.Errorf("first end: %v", resp.History[0].End)
	}
	if resp.History[1].End != nil {
		t.Error("second period should have null end")
	}
}

func TestRegimeHistoryHandler_defaultTimeframe(t *testing.T) {
	provider := &fakeHistoryProvider{
		history: mkt.RegimeHistory{CurrentAge: 0},
	}
	handler := adhttp.NewMarketRegimeHistoryHandler(provider)

	req := httptest.NewRequest("GET", "/api/market/regime/history", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Timeframe string `json:"timeframe"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Timeframe != "4h" {
		t.Errorf("default timeframe: got %q", resp.Timeframe)
	}
}

func TestRegimeHistoryHandler_error(t *testing.T) {
	provider := &fakeHistoryProvider{err: context.DeadlineExceeded}
	handler := adhttp.NewMarketRegimeHistoryHandler(provider)

	req := httptest.NewRequest("GET", "/api/market/regime/history", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d", rec.Code)
	}
}
