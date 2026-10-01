package domain_test

import (
	"testing"

	"pano_chart/backend/domain"
)

func TestEvaluationSnapshot_ZeroValue(t *testing.T) {
	var snap domain.EvaluationSnapshot

	if snap.Symbol != "" {
		t.Errorf("zero value Symbol should be empty, got %q", snap.Symbol)
	}
	if snap.Timeframe != "" {
		t.Errorf("zero value Timeframe should be empty, got %q", snap.Timeframe)
	}
	if snap.AlgoVersion != "" {
		t.Errorf("zero value AlgoVersion should be empty, got %q", snap.AlgoVersion)
	}
	if snap.SidewaysScore != 0 {
		t.Errorf("zero value SidewaysScore should be 0, got %f", snap.SidewaysScore)
	}
	if snap.TrendScore != 0 {
		t.Errorf("zero value TrendScore should be 0, got %f", snap.TrendScore)
	}
	if snap.Price != 0 {
		t.Errorf("zero value Price should be 0, got %f", snap.Price)
	}
}

func TestAlgoVersion_IsSet(t *testing.T) {
	if domain.AlgoVersion == "" {
		t.Error("AlgoVersion constant must not be empty")
	}
}

func TestEvaluationIdentityOK_TrendAlgo(t *testing.T) {
	prev := domain.ActiveTrendAlgo()
	t.Cleanup(func() { domain.ConfigureTrendAlgo(prev) })

	domain.ConfigureTrendAlgo("predictability")
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "") {
		t.Fatal("empty TrendAlgo must match predictability (legacy rows)")
	}
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "predictability") {
		t.Fatal("predictability must match")
	}
	if domain.EvaluationIdentityOK(domain.AlgoVersion, "strength") {
		t.Fatal("strength must not match predictability process")
	}

	domain.ConfigureTrendAlgo("strength")
	if domain.EvaluationIdentityOK(domain.AlgoVersion, "") {
		t.Fatal("legacy empty TrendAlgo must not match strength process")
	}
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "strength") {
		t.Fatal("strength must match")
	}
	if domain.EvaluationIdentityOK("old", "strength") {
		t.Fatal("wrong AlgoVersion must fail")
	}
}
