package usecases

import (
	"math"
	"testing"

	"pano_chart/backend/domain"
)

func f64(v float64) *float64 { return &v }

func fillTape(n int, start float64, step float64) (ts []int64, closes []float64, tape map[int64]float64) {
	ts = make([]int64, n)
	closes = make([]float64, n)
	tape = make(map[int64]float64, n)
	closes[0] = start
	ts[0] = 0
	tape[0] = start
	for i := 1; i < n; i++ {
		ts[i] = int64(i)
		closes[i] = closes[i-1] * step
		tape[ts[i]] = closes[i]
	}
	return ts, closes, tape
}

func TestApplyRelativeStrength_IdenticalToTape(t *testing.T) {
	mktR := make([]float64, 24)
	for i := range mktR {
		mktR[i] = 0.01 * float64((i%5)-2)
		if mktR[i] == 0 {
			mktR[i] = 0.005
		}
	}
	ts := make([]int64, len(mktR)+1)
	closes := make([]float64, len(mktR)+1)
	tape := map[int64]float64{}
	closes[0], ts[0] = 100, 0
	tape[0] = 100
	for i, r := range mktR {
		ts[i+1] = int64(i + 1)
		closes[i+1] = closes[i] * math.Exp(r)
		tape[ts[i+1]] = closes[i+1]
	}
	results := []RankedResult{{
		Symbol: domain.NewSymbolUnsafe("AAAUSDT"), Sparkline: closes, sparkTS: ts,
	}}
	if n := applyRelativeStrength(results, tape, nil); n != 1 {
		t.Fatalf("scored=%d want 1", n)
	}
	if results[0].RelativeStrength == nil || math.Abs(*results[0].RelativeStrength) > 1e-12 {
		t.Fatalf("RS=%v want ~0", ptrVal(results[0].RelativeStrength))
	}
	if results[0].Beta == nil || math.Abs(*results[0].Beta-1) > 1e-9 {
		t.Fatalf("Beta=%v want ~1", ptrVal(results[0].Beta))
	}
}

func ptrVal(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

func TestApplyRelativeStrength_DoubleTapeReturns(t *testing.T) {
	mktR := make([]float64, 24)
	for i := range mktR {
		mktR[i] = 0.01 * float64((i%5)-2)
		if mktR[i] == 0 {
			mktR[i] = 0.005
		}
	}
	ts := make([]int64, len(mktR)+1)
	sym := make([]float64, len(mktR)+1)
	mkt := make([]float64, len(mktR)+1)
	tape := map[int64]float64{}
	sym[0], mkt[0], ts[0] = 100, 100, 0
	tape[0] = 100
	for i, r := range mktR {
		ts[i+1] = int64(i + 1)
		mkt[i+1] = mkt[i] * math.Exp(r)
		sym[i+1] = sym[i] * math.Exp(2*r)
		tape[ts[i+1]] = mkt[i+1]
	}
	results := []RankedResult{{
		Symbol: domain.NewSymbolUnsafe("HOTUSDT"), Sparkline: sym, sparkTS: ts,
	}}
	applyRelativeStrength(results, tape, nil)
	if results[0].Beta == nil || math.Abs(*results[0].Beta-2) > 1e-9 {
		t.Fatalf("Beta=%v want 2", results[0].Beta)
	}
}

func TestApplyRelativeStrength_ShortRocketDoesNotScore(t *testing.T) {
	// Full-window tape (40 stamps → need 20). Two-stamp rocket must stay unset.
	_, _, tape := fillTape(40, 100, 1.001)
	rocketTS := []int64{38, 39}
	rocket := []float64{100, 200} // +100% in two bars
	fullTS := make([]int64, 40)
	full := make([]float64, 40)
	full[0] = 100
	for i := 1; i < 40; i++ {
		fullTS[i] = int64(i)
		full[i] = full[i-1] * 1.001
	}
	fullTS[0] = 0
	results := []RankedResult{
		{Symbol: domain.NewSymbolUnsafe("ROCKETUSDT"), Sparkline: rocket, sparkTS: rocketTS},
		{Symbol: domain.NewSymbolUnsafe("STEADYUSDT"), Sparkline: full, sparkTS: fullTS},
	}
	applyRelativeStrength(results, tape, nil)
	if results[0].RelativeStrength != nil {
		t.Fatal("2-stamp rocket must not score against long tape")
	}
	if results[1].RelativeStrength == nil {
		t.Fatal("full-window name should score")
	}
}

func TestApplyRelativeStrength_BelowMinOverlapLeavesUnset(t *testing.T) {
	_, _, tape := fillTape(40, 100, 1.01)
	results := []RankedResult{{
		Symbol:    domain.NewSymbolUnsafe("AAAUSDT"),
		Sparkline: []float64{100, 110, 120},
		sparkTS:   []int64{0, 1, 2}, // 3 < need 20
	}}
	if n := applyRelativeStrength(results, tape, nil); n != 0 {
		t.Fatalf("scored=%d want 0", n)
	}
	if results[0].RelativeStrength != nil {
		t.Fatal("short overlap must leave unset")
	}
}

func TestApplyRelativeStrength_UnequalLengthsUsesOverlapOnly(t *testing.T) {
	mktR := make([]float64, 29)
	for i := range mktR {
		mktR[i] = 0.01 * float64((i%5)-2)
		if mktR[i] == 0 {
			mktR[i] = 0.005
		}
	}
	tape := map[int64]float64{0: 100}
	px := 100.0
	for i, r := range mktR {
		px *= math.Exp(r)
		tape[int64(i+1)] = px
	}
	sparkTS := make([]int64, 25)
	spark := make([]float64, 25)
	for i := 0; i < 5; i++ {
		sparkTS[i] = int64(-5 + i)
		spark[i] = 50
	}
	for i := 5; i < 25; i++ {
		sparkTS[i] = int64(i - 5) // 0..19
		spark[i] = tape[sparkTS[i]]
	}
	results := []RankedResult{{
		Symbol: domain.NewSymbolUnsafe("AAAUSDT"), Sparkline: spark, sparkTS: sparkTS,
	}}
	applyRelativeStrength(results, tape, nil)
	if results[0].RelativeStrength == nil || math.Abs(*results[0].RelativeStrength) > 1e-12 {
		t.Fatalf("RS on overlap should be ~0, got %v", ptrVal(results[0].RelativeStrength))
	}
	if results[0].Beta == nil || math.Abs(*results[0].Beta-1) > 1e-9 {
		t.Fatalf("Beta on overlap should be ~1, got %v", ptrVal(results[0].Beta))
	}
}

func TestOlsBeta_GappedOverlapStillOnePeriod(t *testing.T) {
	// Explicit: consecutive overlap points form one return even with a hole in the
	// underlying tape (stamps 0,1,3 — missing 2). Pin closed-form OLS.
	sym := []float64{100, 101, 104}
	mkt := []float64{100, 101, 102}
	x1 := math.Log(101.0 / 100.0)
	y1 := math.Log(101.0 / 100.0)
	x2 := math.Log(102.0 / 101.0)
	y2 := math.Log(104.0 / 101.0)
	// slope = (n*Σxy − Σx*Σy) / (n*Σx² − (Σx)²) with n=2
	n := 2.0
	sx, sy := x1+x2, y1+y2
	sxx, sxy := x1*x1+x2*x2, x1*y1+x2*y2
	want := (n*sxy - sx*sy) / (n*sxx - sx*sx)
	got := olsBetaAligned(sym, mkt)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("beta=%v want closed-form %v", got, want)
	}
}

func TestAssignRSRank_SkipsUnsetRows(t *testing.T) {
	results := []RankedResult{
		{Symbol: domain.NewSymbolUnsafe("HOTUSDT"), RelativeStrength: f64(0.1)},
		{Symbol: domain.NewSymbolUnsafe("SKIPUSDT")},
		{Symbol: domain.NewSymbolUnsafe("LOWUSDT"), RelativeStrength: f64(-0.1)},
	}
	assignRSRank(results)
	if results[1].RSRank != nil {
		t.Fatal("unset row must not get RSRank")
	}
}

func TestApplyRelativeStrength_ExcludedSymbolSkipped(t *testing.T) {
	ts, closes, tape := fillTape(25, 100, 1.01)
	flat := make([]float64, 25)
	for i := range flat {
		flat[i] = 100
	}
	results := []RankedResult{
		{Symbol: domain.NewSymbolUnsafe("USDCUSDT"), Sparkline: flat, sparkTS: ts},
		{Symbol: domain.NewSymbolUnsafe("BTCUSDT"), Sparkline: closes, sparkTS: ts},
	}
	skip := skipMap{"USDCUSDT": {}}
	if n := applyRelativeStrength(results, tape, skip); n != 1 {
		t.Fatalf("scored=%d want 1", n)
	}
	if results[0].RelativeStrength != nil {
		t.Fatal("excluded stable must stay unset")
	}
}

type skipMap map[string]struct{}

func (s skipMap) Skip(sym string) bool {
	_, ok := s[sym]
	return ok
}

func TestSortResults_LeadersPutsHighestRSFirst_UnsetLast(t *testing.T) {
	results := []RankedResult{
		{Symbol: domain.NewSymbolUnsafe("LOWUSDT"), RelativeStrength: f64(-0.05)},
		{Symbol: domain.NewSymbolUnsafe("SKIPUSDT")},
		{Symbol: domain.NewSymbolUnsafe("HIUSDT"), RelativeStrength: f64(0.10)},
	}
	sortResults(results, SortByLeaders)
	if results[0].Symbol.String() != "HIUSDT" || results[2].Symbol.String() != "SKIPUSDT" {
		t.Fatalf("order=%v", results)
	}
}

func TestParseSortMode_LeadersLaggards(t *testing.T) {
	if ParseSortMode("leaders") != SortByLeaders || ParseSortMode("laggards") != SortByLaggards {
		t.Fatal("parse")
	}
}

func TestOlsBeta_FlatTapeReturnsZero(t *testing.T) {
	if olsBetaAligned([]float64{100, 110, 120, 130}, []float64{100, 100, 100, 100}) != 0 {
		t.Fatal("flat tape")
	}
}

func TestEffectiveSort_FallsBackWhenRSUnavailable(t *testing.T) {
	if effectiveSort(SortByLeaders, false) != SortByTotal {
		t.Fatal("fallback")
	}
}

func TestMinOverlapRequired(t *testing.T) {
	if minOverlapRequired(110) != 20 {
		t.Fatalf("110 → %d want 20", minOverlapRequired(110))
	}
	if minOverlapRequired(10) != 5 {
		t.Fatalf("10 → %d want 5", minOverlapRequired(10))
	}
	if minOverlapRequired(2) != 2 {
		t.Fatalf("2 → %d want 2", minOverlapRequired(2))
	}
}

func TestRSZeroScoredTransient(t *testing.T) {
	tape := map[int64]float64{}
	for i := int64(0); i < 40; i++ {
		tape[i] = 100 + float64(i)
	}
	need := minOverlapRequired(len(tape)) // 20
	short := RankedResult{
		Symbol: domain.NewSymbolUnsafe("SHORTUSDT"),
		sparkTS: func() []int64 {
			ts := make([]int64, need-1)
			for i := range ts {
				ts[i] = int64(i)
			}
			return ts
		}(),
	}
	capable := RankedResult{
		Symbol: domain.NewSymbolUnsafe("FULLUSDT"),
		sparkTS: func() []int64 {
			ts := make([]int64, need)
			for i := range ts {
				ts[i] = int64(1000 + i) // no overlap with tape keys
			}
			return ts
		}(),
	}
	excluded := RankedResult{Symbol: domain.NewSymbolUnsafe("USDCUSDT"), sparkTS: capable.sparkTS}
	skip := skipFn(func(s string) bool { return s == "USDCUSDT" })

	t.Run("incompleteUniverse", func(t *testing.T) {
		if !rsZeroScoredTransient([]RankedResult{short}, tape, nil, 110, 10) {
			t.Fatal("partial coverage must be transient")
		}
	})
	t.Run("precisionBelowFloor", func(t *testing.T) {
		if rsZeroScoredTransient([]RankedResult{short}, tape, nil, 10, 1) {
			t.Fatal("precision < need is stable")
		}
	})
	t.Run("allShortHistory", func(t *testing.T) {
		if rsZeroScoredTransient([]RankedResult{short}, tape, nil, 110, 1) {
			t.Fatal("all eligible short is stable")
		}
	})
	t.Run("capableUnaligned", func(t *testing.T) {
		if !rsZeroScoredTransient([]RankedResult{capable}, tape, nil, 110, 1) {
			t.Fatal("capable but unscored must be transient")
		}
	})
	t.Run("allExcluded", func(t *testing.T) {
		if rsZeroScoredTransient([]RankedResult{excluded}, tape, skip, 110, 1) {
			t.Fatal("only excluded names is stable")
		}
	})
}

type skipFn func(string) bool

func (f skipFn) Skip(s string) bool { return f(s) }
