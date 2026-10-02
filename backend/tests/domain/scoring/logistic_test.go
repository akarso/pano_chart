package scoring_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"pano_chart/backend/domain/scoring"
)

func TestPredict_MatchesHandComputedSigmoid(t *testing.T) {
	model := scoring.Model{
		Type: "logistic",
		Weights: map[string]float64{
			"trend": 2.0,
			"er":    1.0,
		},
		Bias: -1.0,
	}
	features := map[string]float64{"trend": 0.5, "er": 0.2}
	want := 1 / (1 + math.Exp(-0.2))
	got := scoring.Predict(features, model)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("Predict=%.15f want %.15f", got, want)
	}
}

func TestPredict_MissingFeaturesAreZero(t *testing.T) {
	model := scoring.Model{
		Weights: map[string]float64{"trend": 1.0, "missing": 10.0},
		Bias:    0,
	}
	got := scoring.Predict(map[string]float64{"trend": 1.0}, model)
	want := 1 / (1 + math.Exp(-1.0))
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("Predict=%.15f want %.15f", got, want)
	}
}

func TestClassifyStructure_NormalizesAndPicksArgmax(t *testing.T) {
	model := scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 10}, Bias: 0},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -5},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: -5},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: -5},
		},
	}
	features := map[string]float64{"trend": 1.0, "sideways": 0, "compression": 0, "expansion": 0}
	tr, sw, co, ex, dom, ok := scoring.ClassifyStructure(features, model)
	if !ok {
		t.Fatal("expected ok")
	}
	if dom != "trend" {
		t.Fatalf("dominant=%q want trend", dom)
	}
	sum := tr + sw + co + ex
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("structure sum=%v want 1", sum)
	}
	if tr <= sw || tr <= co || tr <= ex {
		t.Fatalf("trend %.4f should dominate", tr)
	}
}

func TestClassifyStructure_PlaceholderOrIncomplete_NotOK(t *testing.T) {
	_, _, _, _, _, ok := scoring.ClassifyStructure(nil, scoring.Model{
		Placeholder: true,
		Classes: map[string]scoring.ClassParams{
			"trend": {}, "sideways": {}, "compression": {}, "expansion": {},
		},
	})
	if ok {
		t.Fatal("placeholder must not classify")
	}
	_, _, _, _, _, ok = scoring.ClassifyStructure(nil, scoring.Model{
		Weights: map[string]float64{"trend": 1},
	})
	if ok {
		t.Fatal("binary-only model must not classify structure")
	}
	_, _, _, _, _, ok = scoring.ClassifyStructure(nil, scoring.Model{
		Classes: map[string]scoring.ClassParams{"trend": {}},
	})
	if ok {
		t.Fatal("partial classes must not classify")
	}
}

func TestValidateForInference_RequiresFourClasses(t *testing.T) {
	m := scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend": {}, "sideways": {}, "compression": {},
		},
	}
	if err := m.ValidateForInference(); err == nil {
		t.Fatal("expected error for missing expansion")
	}
}

func TestValidate_RejectsNonFinite(t *testing.T) {
	m := scoring.Model{
		Type:    "logistic",
		Weights: map[string]float64{"trend": math.NaN()},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected NaN rejection")
	}
}

func TestLoadRegimeModel_StubFileIsPlaceholder(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "config", "regime_model.yaml"),
		filepath.Join("config", "regime_model.yaml"),
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Fatal("regime_model.yaml not found")
	}
	m, err := scoring.LoadRegimeModel(path)
	if err != nil {
		t.Fatalf("LoadRegimeModel: %v", err)
	}
	if !m.Placeholder {
		t.Fatal("shipped stub must be placeholder: true")
	}
	if err := m.ValidateForInference(); err == nil {
		t.Fatal("placeholder must fail ValidateForInference")
	}
	p := scoring.Predict(map[string]float64{"trend": 0.8, "er": 0.5}, *m)
	if p <= 0 || p >= 1 {
		t.Fatalf("Predict out of (0,1): %v", p)
	}
}

func TestEfficiencyRatio_PerfectTrend(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5}
	er := scoring.EfficiencyRatio(closes)
	if math.Abs(er-1) > 1e-12 {
		t.Fatalf("ER=%.12f want 1", er)
	}
}

func TestEfficiencyRatio_RoundTrip(t *testing.T) {
	closes := []float64{1, 2, 1, 2, 1}
	er := scoring.EfficiencyRatio(closes)
	if er >= 0.5 {
		t.Fatalf("choppy ER=%.4f want <0.5", er)
	}
}

func TestRegimeModelMode_DefaultHeuristic(t *testing.T) {
	scoring.ResetConfig()
	t.Cleanup(scoring.ResetConfig)
	if got := scoring.RegimeModelMode(); got != "heuristic" {
		t.Fatalf("mode=%q", got)
	}
}

func TestValidateRegimeModelMode_RejectsTypo(t *testing.T) {
	scoring.ResetConfig()
	t.Cleanup(scoring.ResetConfig)
	scoring.ReplaceConfigForTest(&scoring.AppConfig{
		Scoring: scoring.ScoringYAML{RegimeModel: "lerned"},
	})
	if err := scoring.ValidateRegimeModelMode(); err == nil {
		t.Fatal("expected error for typo")
	}
	scoring.ReplaceConfigForTest(&scoring.AppConfig{
		Scoring: scoring.ScoringYAML{RegimeModel: "learned"},
	})
	if err := scoring.ValidateRegimeModelMode(); err != nil {
		t.Fatalf("learned should be ok: %v", err)
	}
}

func TestRegimeModelPath_ConfigOverride(t *testing.T) {
	scoring.ResetConfig()
	t.Cleanup(scoring.ResetConfig)
	scoring.ReplaceConfigForTest(&scoring.AppConfig{
		Scoring: scoring.ScoringYAML{RegimeModelPath: "/tmp/custom_regime.yaml"},
	})
	if got := scoring.RegimeModelPath(); got != "/tmp/custom_regime.yaml" {
		t.Fatalf("path=%q", got)
	}
}
