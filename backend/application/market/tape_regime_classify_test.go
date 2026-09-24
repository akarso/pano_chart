package market

import (
	"math"
	"testing"

	mkt "pano_chart/backend/domain/market"
)

func TestClassifyTape_TinyPositiveTotalNormalizes(t *testing.T) {
	// A positive but tiny total must not invent sideways weight when the
	// sideways calculator measured zero.
	tape := classifyTape(0.04, "up", 0, 0, 0, 0, 0)
	if tape.Structure.Sideways != 0 {
		t.Fatalf("Structure.Sideways=%.4f want 0 (measured)", tape.Structure.Sideways)
	}
	if math.Abs(tape.Structure.Trend-1) > 1e-9 {
		t.Fatalf("Structure.Trend=%.4f want 1", tape.Structure.Trend)
	}
	if tape.State == mkt.StateTrend {
		t.Fatal("weak TapeTrend must not become TREND via mix share")
	}
}

func TestClassifyTape_WeakTapeTrendNotPromoted(t *testing.T) {
	// Weak TapeTrend with other raw scores ≈ 0: mix share would be ~100% trend,
	// but the exclusive gate must keep state off TREND and the caption clear.
	tape := classifyTape(0.35, "up", 0, 0, 0, 0, 0)
	if tape.State == mkt.StateTrend {
		t.Fatalf("weak TapeTrend promoted to TREND via mix %+v", tape.Structure)
	}
	if tape.Structure.Trend < 0.5 {
		t.Fatalf("fixture broken: Structure.Trend=%.3f (want mix-dominant trend share)", tape.Structure.Trend)
	}
	if isTrendCaption(tape.Label) {
		t.Fatalf("trend caption on non-TREND state: %q", tape.Label)
	}
	if tape.State == mkt.StateSideways && tape.Label != "Sideways" {
		t.Fatalf("label=%q want Sideways for state=%s", tape.Label, tape.State)
	}
	if tape.State == mkt.StateIndecisive && tape.Label != "Mixed conditions" {
		t.Fatalf("label=%q want Mixed conditions for state=%s", tape.Label, tape.State)
	}
}

func TestClassifyTape_Coexistence(t *testing.T) {
	tape := classifyTape(0.55, "up", 0.05, 0.8, 0.75, 0.85, 0)
	if tape.TrendScore < 0.5 {
		t.Fatal("fixture: need TapeTrend ≥ 0.5")
	}
	if tape.Structure.Trend >= 0.4 {
		t.Fatalf("fixture: Structure.Trend=%.3f want <0.4", tape.Structure.Trend)
	}
	if tape.State != mkt.StateTrend {
		t.Fatalf("state=%s want TREND", tape.State)
	}
	if tape.Confidence != tape.TrendScore {
		t.Fatalf("confidence=%.3f want raw TapeTrend %.3f", tape.Confidence, tape.TrendScore)
	}
	if tape.Label == "No clear trend" || tape.Label == "Mixed conditions" {
		t.Fatalf("label=%q on TREND headline", tape.Label)
	}
}

func TestClassifyTape_CompressionCaption(t *testing.T) {
	tape := classifyTape(0.1, "neutral", 0.05, 0.9, 0.05, 0, 0)
	if tape.State != mkt.StateCompression {
		t.Fatalf("state=%s want compression", tape.State)
	}
	if tape.Label != "Compression" {
		t.Fatalf("label=%q want Compression", tape.Label)
	}
}

func TestBuildTapeLabel_MatchesState(t *testing.T) {
	cases := []struct {
		state mkt.State
		want  string
	}{
		{mkt.StateCompression, "Compression"},
		{mkt.StateExpansion, "Expansion"},
		{mkt.StateSideways, "Sideways"},
		{mkt.StateIndecisive, "Mixed conditions"},
		{mkt.StateSilent, "No clear trend"},
		{mkt.StateTrend, "Trend weakening"}, // health 0.6
	}
	for _, tc := range cases {
		got := BuildTapeLabel(tc.state, 0.6)
		if got != tc.want {
			t.Fatalf("state=%s label=%q want %q", tc.state, got, tc.want)
		}
	}
	if got := BuildTapeLabel(mkt.StateTrend, 0.8); got != "Strong trend" {
		t.Fatalf("healthy trend label=%q", got)
	}
}

func isTrendCaption(label string) bool {
	switch label {
	case "Strong trend", "Trend weakening", "Trend breaking down":
		return true
	default:
		return false
	}
}
