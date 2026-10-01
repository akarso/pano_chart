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
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "", "predictability") {
		t.Fatal("empty TrendAlgo must match predictability want (legacy rows)")
	}
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "predictability", "") {
		t.Fatal("empty want must default to predictability")
	}
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "predictability", "predictability") {
		t.Fatal("predictability must match")
	}
	if domain.EvaluationIdentityOK(domain.AlgoVersion, "strength", "predictability") {
		t.Fatal("strength must not match predictability want")
	}
	if domain.EvaluationIdentityOK(domain.AlgoVersion, "", "strength") {
		t.Fatal("legacy empty TrendAlgo must not match strength want")
	}
	if !domain.EvaluationIdentityOK(domain.AlgoVersion, "strength", "strength") {
		t.Fatal("strength must match")
	}
	if domain.EvaluationIdentityOK("old", "strength", "strength") {
		t.Fatal("wrong AlgoVersion must fail")
	}
}
