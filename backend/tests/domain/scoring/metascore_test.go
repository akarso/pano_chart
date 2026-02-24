package scoring_test

import (
	"pano_chart/backend/domain/scoring"
	"testing"
)

type mockSubscore struct {
	name       string
	value      float64
	confidence float64
}

func TestMetaScorer_ProfileIntegration(t *testing.T) {
	subscores := []scoring.Subscore{
		&mockSubscore{"A", 0.7, 1.0},
		&mockSubscore{"B", 0.3, 1.0},
	}
	profile := scoring.DefaultTimeframeProfiles["15m"]
	cfg := profile.MetaConfig
	cfg.Mode = scoring.WeightedAdditive
	cfg.BaseWeights = map[string]float64{"A": 1, "B": 1}
	ms := scoring.NewMetaScorer(subscores, cfg)
	score, breakdown := ms.ScoreWithBreakdown(nil, profile.SubConfigs)
	if score < 0.49 || score > 0.51 {
		t.Errorf("expected score ~0.5 for 15m profile, got %v", score)
	}
	if len(breakdown) != 2 {
		t.Errorf("expected 2 breakdown entries for 15m profile")
	}
	if profile.CandleCount != 120 {
		t.Errorf("expected CandleCount 120 for 15m profile, got %v", profile.CandleCount)
	}
}

func (m *mockSubscore) Name() string { return m.name }
func (m *mockSubscore) Compute(data interface{}, cfg interface{}) scoring.SubscoreResult {
	return scoring.SubscoreResult{
		Value:      m.value,
		Confidence: m.confidence,
		Meta:       map[string]float64{"debug": m.value},
	}
}

func TestMetaScorer_WeightedAdditive(t *testing.T) {
	subscores := []scoring.Subscore{
		&mockSubscore{"A", 0.8, 1.0},
		&mockSubscore{"B", 0.2, 1.0},
	}
	cfg := scoring.MetaConfig{
		Mode:                scoring.WeightedAdditive,
		BaseWeights:         map[string]float64{"A": 2, "B": 1},
		UseConfidenceWeight: false,
	}
	ms := scoring.NewMetaScorer(subscores, cfg)
	score, breakdown := ms.ScoreWithBreakdown(nil, nil)
	if score < 0.59 || score > 0.61 {
		t.Errorf("expected score ~0.6, got %v", score)
	}
	if len(breakdown) != 2 {
		t.Errorf("expected 2 breakdown entries")
	}
}

func TestMetaScorer_WeightedMultiplicative(t *testing.T) {
	subscores := []scoring.Subscore{
		&mockSubscore{"A", 0.8, 1.0},
		&mockSubscore{"B", 0.2, 1.0},
	}
	cfg := scoring.MetaConfig{
		Mode:                scoring.WeightedMultiplicative,
		BaseWeights:         map[string]float64{"A": 2, "B": 1},
		UseConfidenceWeight: false,
	}
	ms := scoring.NewMetaScorer(subscores, cfg)
	score, _ := ms.ScoreWithBreakdown(nil, nil)
	if score > 0.13 || score < 0.11 {
		t.Errorf("expected score ~0.12, got %v", score)
	}
}

func TestMetaScorer_ConfidenceWeighting(t *testing.T) {
	subscores := []scoring.Subscore{
		&mockSubscore{"A", 0.8, 0.5},
		&mockSubscore{"B", 0.2, 1.0},
	}
	cfg := scoring.MetaConfig{
		Mode:                scoring.WeightedAdditive,
		BaseWeights:         map[string]float64{"A": 2, "B": 1},
		UseConfidenceWeight: true,
	}
	ms := scoring.NewMetaScorer(subscores, cfg)
	score, _ := ms.ScoreWithBreakdown(nil, nil)
	if score > 0.51 || score < 0.45 {
		t.Errorf("expected score ~0.46, got %v", score)
	}
}

func TestMetaScorer_Clamp(t *testing.T) {
	subscores := []scoring.Subscore{
		&mockSubscore{"A", 1.5, 2.0},
		&mockSubscore{"B", -0.5, -1.0},
	}
	cfg := scoring.MetaConfig{
		Mode:                scoring.WeightedAdditive,
		BaseWeights:         map[string]float64{"A": 1, "B": 1},
		UseConfidenceWeight: false,
	}
	ms := scoring.NewMetaScorer(subscores, cfg)
	score, breakdown := ms.ScoreWithBreakdown(nil, nil)
	if score != 0.5 {
		t.Errorf("expected clamped score 0.5, got %v", score)
	}
	if breakdown["A"].Value != 1.0 || breakdown["B"].Value != 0.0 {
		t.Errorf("expected clamped breakdown values")
	}
}
