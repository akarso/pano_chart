package usecases_test

import (
	"testing"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain/scoring"
)

func TestParseTrendAlgo(t *testing.T) {
	cases := []struct {
		in   string
		want usecases.TrendAlgoMode
		ok   bool
	}{
		{"", usecases.TrendAlgoPredictability, true},
		{"predictability", usecases.TrendAlgoPredictability, true},
		{"strength", usecases.TrendAlgoStrength, true},
		{"Strength", usecases.TrendAlgoStrength, true},
		{" STRENGTH ", usecases.TrendAlgoStrength, true},
		{"strenght", usecases.TrendAlgoPredictability, false},
		{"v2", usecases.TrendAlgoPredictability, false},
	}
	for _, tc := range cases {
		got, ok := usecases.ParseTrendAlgo(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ParseTrendAlgo(%q)=(%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTrendCalcFor_DefaultAndStrength(t *testing.T) {
	if _, ok := usecases.TrendCalcFor(usecases.TrendAlgoPredictability).(*scoring.TrendPredictabilityScoreCalculator); !ok {
		t.Fatal("predictability mode")
	}
	if _, ok := usecases.TrendCalcFor(usecases.TrendAlgoStrength).(*scoring.TrendStrengthScoreCalculator); !ok {
		t.Fatal("strength mode")
	}
}
