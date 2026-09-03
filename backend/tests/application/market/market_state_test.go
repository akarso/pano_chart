package market_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	adhttp "pano_chart/backend/adapters/http"
	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

type fakeEvalProvider struct {
	evals []domain.EvaluationSnapshot
	err   error
}

func (f *fakeEvalProvider) GetLatestEvaluations(_ string) ([]domain.EvaluationSnapshot, error) {
	return f.evals, f.err
}

func TestMarketState_Constants(t *testing.T) {
	if mkt.StateSideways != "sideways" {
		t.Errorf("expected sideways, got %s", mkt.StateSideways)
	}
	if mkt.StateCompression != "compression" {
		t.Errorf("expected compression, got %s", mkt.StateCompression)
	}
	if mkt.StateExpansion != "expansion" {
		t.Errorf("expected expansion, got %s", mkt.StateExpansion)
	}
	if mkt.StateTrend != "trend" {
		t.Errorf("expected trend, got %s", mkt.StateTrend)
	}
}

func TestClassify_BreakoutUp(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{BreakoutUpScore: 0.9, CompressionScore: 0.1, TrendScore: 0.1},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateExpansion {
		t.Errorf("expected expansion, got %s", s.State)
	}
}

func TestClassify_BreakoutDown(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{BreakoutDownScore: 0.75},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateExpansion {
		t.Errorf("expected expansion, got %s", s.State)
	}
}

func TestClassify_Compression(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{CompressionScore: 0.8, TrendScore: 0.3},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateCompression {
		t.Errorf("expected compression, got %s", s.State)
	}
}

func TestClassify_Trend(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.70},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateTrend {
		t.Errorf("expected trend, got %s", s.State)
	}
}

func TestClassify_DefaultSideways(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.9, TrendScore: 0.1},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateSideways {
		t.Errorf("expected sideways, got %s", s.State)
	}
}

func TestClassify_BreakoutTakesPriority(t *testing.T) {
	// When scores are equal, indecisive takes over.
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{BreakoutUpScore: 0.9, CompressionScore: 0.9, TrendScore: 0.9},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateIndecisive {
		t.Errorf("expected indecisive when all scores equal, got %s", s.State)
	}
}

func TestService_MajorityCompression(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{CompressionScore: 0.8},
			{CompressionScore: 0.75},
			{TrendScore: 0.8},
		},
	})
	summary, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if summary.State != mkt.StateCompression {
		t.Errorf("expected compression, got %s", summary.State)
	}
	if summary.SymbolCount != 3 {
		t.Errorf("expected 3 symbols, got %d", summary.SymbolCount)
	}
}

func TestService_BreadthRatios(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.9, TrendScore: 0.1}, // sideways-heavy
			{SidewaysScore: 0.9, TrendScore: 0.1}, // sideways-heavy
			{TrendScore: 0.8},                     // trend-dominated
			{BreakoutUpScore: 0.9},                // breakout-dominated
		},
	})
	summary, err := svc.Calculate("1h")
	if err != nil {
		t.Fatal(err)
	}

	// Continuous proportional breadth — no field should be exactly zero.
	if summary.Breadth.Sideways <= 0 {
		t.Errorf("expected positive Sideways breadth, got %f", summary.Breadth.Sideways)
	}
	if summary.Breadth.Trend <= 0 {
		t.Errorf("expected positive Trend breadth, got %f", summary.Breadth.Trend)
	}
	if summary.Breadth.Expansion <= 0 {
		t.Errorf("expected positive Expansion breadth, got %f", summary.Breadth.Expansion)
	}
	// All four fields should approximately sum to 1.0.
	sum := summary.Breadth.Sideways + summary.Breadth.Compression + summary.Breadth.Expansion + summary.Breadth.Trend
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("expected breadth sum ≈ 1.0, got %f", sum)
	}
	// Sideways-heavy tokens should push sideways breadth highest.
	if summary.Breadth.Sideways <= summary.Breadth.Expansion {
		t.Errorf("expected sideways > expansion, got sideways=%f expansion=%f",
			summary.Breadth.Sideways, summary.Breadth.Expansion)
	}
}

func TestService_EmptyEvaluations(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{},
	})
	summary, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if summary.State != mkt.StateSideways {
		t.Errorf("expected sideways for empty, got %s", summary.State)
	}
	if summary.SymbolCount != 0 {
		t.Errorf("expected 0 symbols, got %d", summary.SymbolCount)
	}
	if summary.Confidence != 0 {
		t.Errorf("expected 0 confidence for empty, got %f", summary.Confidence)
	}
}

func TestService_ProviderError(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		err: http.ErrServerClosed,
	})
	_, err := svc.Calculate("4h")
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestService_TimeframePassedThrough(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{{SidewaysScore: 0.5}},
	})
	summary, err := svc.Calculate("15m")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Timeframe != "15m" {
		t.Errorf("expected timeframe 15m, got %s", summary.Timeframe)
	}
}

func TestMarketHandler_DefaultTimeframe(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.5},
		},
	})
	h := adhttp.NewMarketHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/state", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["timeframe"] != "4h" {
		t.Errorf("expected default timeframe 4h, got %v", resp["timeframe"])
	}
	if resp["state"] != "sideways" {
		t.Errorf("expected state sideways, got %v", resp["state"])
	}
}

func TestMarketHandler_ExplicitTimeframe(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8},
		},
	})
	h := adhttp.NewMarketHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/state?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["timeframe"] != "1h" {
		t.Errorf("expected timeframe 1h, got %v", resp["timeframe"])
	}
	if resp["state"] != "trend" {
		t.Errorf("expected state trend, got %v", resp["state"])
	}
}

func TestMarketHandler_JSONFields(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.5},
			{SidewaysScore: 0.3},
			{TrendScore: 0.8},
		},
	})
	h := adhttp.NewMarketHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/state?timeframe=4h", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	requiredFields := []string{"timeframe", "state", "confidence", "breadth", "symbolCount"}
	for _, f := range requiredFields {
		if _, ok := resp[f]; !ok {
			t.Errorf("missing required field %q", f)
		}
	}
	breadth, ok := resp["breadth"].(map[string]interface{})
	if !ok {
		t.Fatal("breadth not an object")
	}
	breadthFields := []string{"sideways", "compression", "expansion", "trend"}
	for _, f := range breadthFields {
		if _, ok := breadth[f]; !ok {
			t.Errorf("missing breadth field %q", f)
		}
	}
	symbolCount := resp["symbolCount"].(float64)
	if symbolCount != 3 {
		t.Errorf("expected symbolCount 3, got %v", symbolCount)
	}
}

func TestClassify_Bias_Up(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8, Bias: "up"},
			{TrendScore: 0.6, Bias: "up"},
			{TrendScore: 0.3, Bias: "down"},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.Bias != "up" {
		t.Errorf("expected bias up, got %s", s.Bias)
	}
}

func TestClassify_Bias_Down(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8, Bias: "down"},
			{TrendScore: 0.7, Bias: "down"},
			{TrendScore: 0.2, Bias: "up"},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.Bias != "down" {
		t.Errorf("expected bias down, got %s", s.Bias)
	}
}

func TestClassify_Bias_Neutral_NoBias(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.5},
			{TrendScore: 0.5},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.Bias != "neutral" {
		t.Errorf("expected bias neutral, got %s", s.Bias)
	}
}

func TestMarketHandler_BiasInResponse(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8, Bias: "down"},
		},
	})
	h := adhttp.NewMarketHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/state?timeframe=4h", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["bias"] != "down" {
		t.Errorf("expected bias down in response, got %v", resp["bias"])
	}
}

// ---------- Indecisive ----------

func TestClassify_Indecisive_NoDominantRegime(t *testing.T) {
	// All four regimes roughly equal → max < 0.50 → indecisive.
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.5, TrendScore: 0.4, CompressionScore: 0.3, BreakoutUpScore: 0.3},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateIndecisive {
		t.Errorf("expected indecisive when no regime dominant, got %s", s.State)
	}
}

func TestClassify_Indecisive_CloseGap(t *testing.T) {
	// 55% sideways, 45% trend → gap < 30% → indecisive.
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.55, TrendScore: 0.45},
		},
	})
	s, err := svc.Calculate("15m")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateIndecisive {
		t.Errorf("expected indecisive when gap < 30%%, got %s (confidence=%f)", s.State, s.Confidence)
	}
}

func TestClassify_NotIndecisive_ClearDominance(t *testing.T) {
	// 80% trend, 20% sideways → above 50% and gap > 30%.
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8, SidewaysScore: 0.2},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State == mkt.StateIndecisive {
		t.Errorf("clear 80/20 dominance should NOT be indecisive, got %s", s.State)
	}
}

// ---------- Silent ----------

func TestClassify_Silent_FlatWithLowVolume(t *testing.T) {
	// Sideways-dominant, near-zero return, volume data present and normal.
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := range evals {
		evals[i] = domain.EvaluationSnapshot{
			SidewaysScore: 0.9,
			TrendScore:    0.1,
			ATR:           1,
			RecentReturn:  0.1,
			Volume:        1000,
		}
	}
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: evals})
	s, err := svc.Calculate("15m")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mkt.StateSilent {
		t.Errorf("expected silent for flat low-volume market, got %s", s.State)
	}
}

func TestClassify_NotSilent_HighVolume(t *testing.T) {
	// Sideways-dominant, flat, but volume elevated → not silent.
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := range evals {
		vol := 1000.0
		if i < 3 {
			vol = 5000 // elevated volume on some tokens
		}
		evals[i] = domain.EvaluationSnapshot{
			SidewaysScore: 0.9,
			TrendScore:    0.1,
			ATR:           1,
			RecentReturn:  0.1,
			Volume:        vol,
		}
	}
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: evals})
	s, err := svc.Calculate("15m")
	if err != nil {
		t.Fatal(err)
	}
	if s.State == mkt.StateSilent {
		t.Errorf("should not be silent when volume is elevated, got %s", s.State)
	}
}

func TestClassify_NotSilent_NoVolumeData(t *testing.T) {
	// Sideways-dominant but no volume data → can't determine silence.
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{SidewaysScore: 0.9, TrendScore: 0.1},
		},
	})
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.State == mkt.StateSilent {
		t.Errorf("should not be silent without volume data, got %s", s.State)
	}
}
