package notifications_test

import (
	"encoding/json"
	"math"
	"testing"

	"pano_chart/backend/application/notifications"
)

func TestDownsampleSparkline_110To30KeepsFirstLast(t *testing.T) {
	in := make([]float64, 110)
	for i := range in {
		in[i] = float64(i)
	}
	out := notifications.DownsampleSparkline(in, 30)
	if len(out) != 30 {
		t.Fatalf("len=%d want 30", len(out))
	}
	if out[0] != 0 {
		t.Fatalf("first=%v want 0", out[0])
	}
	if out[len(out)-1] != 109 {
		t.Fatalf("last=%v want 109", out[len(out)-1])
	}
	// Even spacing: index i maps to i*(n-1)/(target-1).
	for i, v := range out {
		want := float64(i * 109 / 29)
		if v != want {
			t.Fatalf("out[%d]=%v want %v", i, v, want)
		}
	}
}

func TestDownsampleSparkline_EmptyAndShort(t *testing.T) {
	if got := notifications.DownsampleSparkline(nil, 30); len(got) != 0 {
		t.Fatalf("nil input → %v", got)
	}
	short := []float64{1, 2, 3}
	got := notifications.DownsampleSparkline(short, 30)
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("short copy = %v", got)
	}
	got[0] = 99
	if short[0] != 1 {
		t.Fatal("downsample must copy, not alias")
	}
}

func TestDownsampleSparkline_TargetOneOrLess(t *testing.T) {
	in := []float64{10, 20, 30}
	if got := notifications.DownsampleSparkline(in, 1); len(got) != 1 || got[0] != 10 {
		t.Fatalf("target 1 = %v", got)
	}
	if got := notifications.DownsampleSparkline(in, 0); len(got) != 0 {
		t.Fatalf("target 0 = %v", got)
	}
}

func TestDownsampleSparkline_DropsNonFinite(t *testing.T) {
	in := []float64{1, math.NaN(), 2, math.Inf(1), 3}
	got := notifications.DownsampleSparkline(in, 30)
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("got %v", got)
	}
}

func ptr(v float64) *float64 { return &v }

func TestAttachContext_JSONShape(t *testing.T) {
	data := map[string]string{"type": "setup", "symbol": "BTCUSDT"}
	rs := 0.032
	out := notifications.AttachContext(data, notifications.AlertContext{
		TapeRegime:     "trend",
		TapeBias:       "up",
		TapeConfidence: ptr(0.62),
		SymbolScore:    ptr(0.81),
		RS:             &rs,
		Alignment:      ptr(0.75),
		Sparkline:      []float64{100, 101, 102},
	})
	raw, ok := out["context"]
	if !ok || raw == "" {
		t.Fatal("missing context")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tapeRegime"] != "trend" || parsed["tapeBias"] != "up" {
		t.Fatalf("tape fields = %v", parsed)
	}
	if math.Abs(parsed["tapeConfidence"].(float64)-0.62) > 1e-9 {
		t.Fatalf("confidence = %v", parsed["tapeConfidence"])
	}
	if math.Abs(parsed["symbolScore"].(float64)-0.81) > 1e-9 {
		t.Fatalf("symbolScore = %v", parsed["symbolScore"])
	}
	if math.Abs(parsed["rs"].(float64)-0.032) > 1e-9 {
		t.Fatalf("rs = %v", parsed["rs"])
	}
	if math.Abs(parsed["alignment"].(float64)-0.75) > 1e-9 {
		t.Fatalf("alignment = %v", parsed["alignment"])
	}
	spark, ok := parsed["sparkline"].([]any)
	if !ok || len(spark) != 3 {
		t.Fatalf("sparkline = %v", parsed["sparkline"])
	}
	if out["type"] != "setup" || out["symbol"] != "BTCUSDT" {
		t.Fatalf("routing keys mutated: %v", out)
	}
}

func TestAttachContext_TapeOnlyOmitsSymbolFields(t *testing.T) {
	out := notifications.AttachContext(map[string]string{"type": "market"}, notifications.AlertContext{
		TapeRegime:     "compression",
		TapeBias:       "neutral",
		TapeConfidence: ptr(0.4),
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["symbolScore"]; ok {
		t.Fatal("symbolScore should be omitted")
	}
	if _, ok := parsed["rs"]; ok {
		t.Fatal("rs should be omitted")
	}
	if _, ok := parsed["alignment"]; ok {
		t.Fatal("alignment should be omitted")
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatal("sparkline should be omitted")
	}
	if parsed["tapeRegime"] != "compression" {
		t.Fatalf("tapeRegime = %v", parsed["tapeRegime"])
	}
}

func TestAttachContext_EmptyOmitsKey(t *testing.T) {
	out := notifications.AttachContext(map[string]string{"type": "market"}, notifications.AlertContext{})
	if _, ok := out["context"]; ok {
		t.Fatalf("empty context must not attach, got %q", out["context"])
	}
}

func TestAttachContext_ZeroScoreAndAlignmentPresent(t *testing.T) {
	out := notifications.AttachContext(map[string]string{"type": "setup"}, notifications.AlertContext{
		SymbolScore: ptr(0),
		Alignment:   ptr(0),
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["symbolScore"].(float64) != 0 {
		t.Fatalf("symbolScore=%v want 0", parsed["symbolScore"])
	}
	if parsed["alignment"].(float64) != 0 {
		t.Fatalf("alignment=%v want 0", parsed["alignment"])
	}
}

func TestAttachContext_NonFiniteSparklineKeepsTape(t *testing.T) {
	out := notifications.AttachContext(map[string]string{"type": "setup"}, notifications.AlertContext{
		TapeRegime: "trend",
		TapeBias:   "up",
		Sparkline:  []float64{math.NaN(), math.Inf(1)},
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out["context"]), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tapeRegime"] != "trend" {
		t.Fatalf("tape must survive bad sparkline: %v", parsed)
	}
	if _, ok := parsed["sparkline"]; ok {
		t.Fatalf("non-finite-only sparkline should be omitted, got %v", parsed["sparkline"])
	}
}
