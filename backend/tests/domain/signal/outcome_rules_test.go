package signal_test

import (
	"math"
	"testing"
	"time"

	domainsignal "pano_chart/backend/domain/signal"
)

func TestComputePathStats_long(t *testing.T) {
	// entry 100, atr 2; path goes up to 106 then dips to 98
	closes := []float64{101, 104, 106}
	highs := []float64{102, 105, 107}
	lows := []float64{99, 100, 98}
	s := domainsignal.ComputePathStats(100, 2, 1, closes, highs, lows)
	wantFwd := (106 - 100) / 100.0
	if math.Abs(s.ForwardReturn-wantFwd) > 1e-9 {
		t.Fatalf("fwd=%v want %v", s.ForwardReturn, wantFwd)
	}
	// MFE: (107-100)/2 = 3.5
	if math.Abs(s.MaxFavorable-3.5) > 1e-9 {
		t.Fatalf("mfe=%v want 3.5", s.MaxFavorable)
	}
	// MAE: (100-98)/2 = 1.0
	if math.Abs(s.MaxAdverse-1.0) > 1e-9 {
		t.Fatalf("mae=%v want 1.0", s.MaxAdverse)
	}
	if s.RangeHigh != 107 || s.RangeLow != 98 {
		t.Fatalf("range h/l = %v/%v", s.RangeHigh, s.RangeLow)
	}
}

func TestComputePathStats_short(t *testing.T) {
	closes := []float64{99, 96}
	highs := []float64{101, 98}
	lows := []float64{98, 95}
	s := domainsignal.ComputePathStats(100, 2, -1, closes, highs, lows)
	// MFE short: (100-95)/2 = 2.5
	if math.Abs(s.MaxFavorable-2.5) > 1e-9 {
		t.Fatalf("mfe=%v want 2.5", s.MaxFavorable)
	}
	// MAE short: (101-100)/2 = 0.5
	if math.Abs(s.MaxAdverse-0.5) > 1e-9 {
		t.Fatalf("mae=%v want 0.5", s.MaxAdverse)
	}
}

func TestRule_trendUp(t *testing.T) {
	cases := []struct {
		name    string
		fwd     float64
		mae     float64
		success bool
	}{
		{"win", 0.02, 1.5, true},
		{"neg_return", -0.01, 0.5, false},
		{"adverse_too_deep", 0.03, 2.0, false},
		{"adverse_over", 0.03, 2.1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := domainsignal.Signal{ID: "1", Label: "trend_up"}
			stats := domainsignal.PathStats{ForwardReturn: tc.fwd, MaxAdverse: tc.mae}
			o := domainsignal.Grade(sig, stats, "")
			if o.Success != tc.success {
				t.Fatalf("success=%v want %v rule=%s", o.Success, tc.success, o.Rule)
			}
			if o.Rule != domainsignal.RuleTrendUp {
				t.Fatalf("rule=%q", o.Rule)
			}
		})
	}
}

func TestRule_trendDown(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "trend_down"}
	ok := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: -0.02, MaxAdverse: 1.0}, "")
	if !ok.Success {
		t.Fatal("expected success")
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: 0.01, MaxAdverse: 0.5}, "")
	if fail.Success {
		t.Fatal("expected failure on positive fwd")
	}
}

func TestRule_rangeStay(t *testing.T) {
	sig := domainsignal.Signal{
		ID: "1", Label: "sideways", ATR: 2,
		Context: map[string]float64{"range_low": 100, "range_high": 110},
	}
	// closes within [99, 111] = [100-1, 110+1] with tol=0.5*atr=1
	ok := domainsignal.Grade(sig, domainsignal.PathStats{Closes: []float64{100, 105, 110}}, "")
	if !ok.Success {
		t.Fatal("expected stay success")
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{Closes: []float64{100, 98}}, "")
	if fail.Success {
		t.Fatal("expected breach failure")
	}
	// setup range uses same rule
	sig.Label = "range"
	ok2 := domainsignal.Grade(sig, domainsignal.PathStats{Closes: []float64{101, 109}}, "")
	if !ok2.Success || ok2.Rule != domainsignal.RuleRangeStay {
		t.Fatalf("range label: success=%v rule=%s", ok2.Success, ok2.Rule)
	}
}

func TestRule_rangeInsufficientContext(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "sideways", ATR: 2}
	o := domainsignal.Grade(sig, domainsignal.PathStats{Closes: []float64{100}}, "")
	if o.Success || o.Rule != domainsignal.RuleInsufficientContext {
		t.Fatalf("%+v", o)
	}
	if !domainsignal.ExcludedFromHitRate(o.Rule) {
		t.Fatal("insufficient_context must be excluded from hit-rate")
	}
}

func TestRule_compressionExpand(t *testing.T) {
	sig := domainsignal.Signal{
		ID: "1", Label: "compression",
		Context: map[string]float64{"range_low": 100, "range_high": 102}, // emit range=2
	}
	// realized ≥ 3
	ok := domainsignal.Grade(sig, domainsignal.PathStats{RangeHigh: 106, RangeLow: 100}, "")
	if !ok.Success {
		t.Fatal("expected expand success")
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{RangeHigh: 102.5, RangeLow: 100}, "")
	if fail.Success {
		t.Fatal("expected no-expand failure")
	}
}

func TestRule_breakoutUp(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "breakout_up"}
	ok := domainsignal.Grade(sig, domainsignal.PathStats{MaxFavorable: 2.5, MaxAdverse: 0.8}, "")
	if !ok.Success {
		t.Fatal("expected breakout success")
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{MaxFavorable: 2.5, MaxAdverse: 1.0}, "")
	if fail.Success {
		t.Fatal("expected mae gate failure")
	}
}

func TestRule_breakoutDown(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "breakout_down"}
	ok := domainsignal.Grade(sig, domainsignal.PathStats{MaxFavorable: 2.0, MaxAdverse: 0.5}, "")
	if !ok.Success {
		t.Fatal("expected breakout_down success")
	}
}

func TestRule_regimeTrend(t *testing.T) {
	sig := domainsignal.Signal{
		ID: "1", Label: "regime:trend", Price: 100, ATR: 4,
		Context: map[string]float64{"bias": 1},
	}
	// thresh = 0.5 * 0.04 = 0.02
	ok := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: 0.03}, "")
	if !ok.Success {
		t.Fatal("expected regime trend success")
	}
	failSign := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: -0.03}, "")
	if failSign.Success {
		t.Fatal("expected wrong-sign failure")
	}
	failSmall := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: 0.01}, "")
	if failSmall.Success {
		t.Fatal("expected small-move failure")
	}
}

func TestRule_regimeSideways(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "regime:sideways", Price: 100, ATR: 4}
	ok := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: 0.01}, "")
	if !ok.Success {
		t.Fatal("expected sideways success")
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{ForwardReturn: 0.03}, "")
	if fail.Success {
		t.Fatal("expected large-move failure")
	}
}

func TestRule_transition(t *testing.T) {
	sig := domainsignal.Signal{ID: "1", Label: "transition:sideways"}
	ok := domainsignal.Grade(sig, domainsignal.PathStats{}, "sideways")
	if !ok.Success || ok.Rule != "transition" {
		t.Fatalf("success=%v rule=%s", ok.Success, ok.Rule)
	}
	fail := domainsignal.Grade(sig, domainsignal.PathStats{}, "trend")
	if fail.Success {
		t.Fatal("expected mismatch failure")
	}
}

func TestRule_unsupported(t *testing.T) {
	for _, label := range []string{"gain", "regime:expansion", "regime:compression"} {
		o := domainsignal.Grade(domainsignal.Signal{ID: "1", Label: label}, domainsignal.PathStats{}, "")
		if o.Success || o.Rule != domainsignal.RuleUnsupported {
			t.Fatalf("%s: %+v", label, o)
		}
		if !domainsignal.ExcludedFromHitRate(o.Rule) {
			t.Fatalf("%s must be excluded from hit-rate", label)
		}
	}
}

func TestDirectionFromLabel(t *testing.T) {
	if domainsignal.DirectionFromLabel("trend_up") != 1 {
		t.Fatal()
	}
	if domainsignal.DirectionFromLabel("breakout_down") != -1 {
		t.Fatal()
	}
	if domainsignal.DirectionFromLabel("sideways") != 0 {
		t.Fatal()
	}
}

func TestGrade_preservesPathMetrics(t *testing.T) {
	sig := domainsignal.Signal{ID: "abc", Label: "trend_up", EmittedAt: time.Unix(1, 0)}
	stats := domainsignal.PathStats{ForwardReturn: 0.1, MaxFavorable: 3, MaxAdverse: 1}
	o := domainsignal.Grade(sig, stats, "")
	if o.SignalID != "abc" || o.ForwardReturn != 0.1 || o.MaxFavorable != 3 || o.MaxAdverse != 1 {
		t.Fatalf("%+v", o)
	}
}
