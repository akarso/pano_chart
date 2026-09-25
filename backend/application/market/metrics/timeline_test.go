package metrics

import (
	"fmt"
	"testing"
)

func bar(close float64) barOHLCV {
	return barOHLCV{open: close, high: close, low: close, close: close, volume: 1}
}

func TestSymbolBars_CloseBeforeOrderedPredecessor(t *testing.T) {
	p := &symbolBars{byTS: map[int64]barOHLCV{
		10: bar(100),
		20: bar(110),
		30: {open: 0, high: 0, low: 0, close: 0, volume: 1}, // non-positive skipped
		40: bar(120),
	}}
	if _, ok := p.closeBefore(10); ok {
		t.Fatal("no predecessor before first print")
	}
	got, ok := p.closeBefore(25)
	if !ok || got != 110 {
		t.Fatalf("closeBefore(25)=(%v,%v) want (110,true)", got, ok)
	}
	got, ok = p.closeBefore(40)
	if !ok || got != 110 {
		t.Fatalf("closeBefore(40) skips zero close: (%v,%v) want (110,true)", got, ok)
	}
	got, ok = p.closeBefore(41)
	if !ok || got != 120 {
		t.Fatalf("closeBefore(41)=(%v,%v) want (120,true)", got, ok)
	}
}

func TestActivePaths_PrefersRecentOverLargerOld(t *testing.T) {
	// Far live minority after denser stale stub: live is a resumed last-N.
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for ts := int64(20); ts < 20+resumedLastNBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			0: bar(1), 1: bar(1), 2: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 recent, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("old symbol %s in active set", name)
		}
	}
}

func TestActivePaths_BridgeBarDoesNotPullDeadCluster(t *testing.T) {
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for ts := int64(20); ts < 20+resumedLastNBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	paths["Ar"].byTS[2] = bar(1)
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			0: bar(1), 1: bar(1), 2: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("bridged old symbol %s must stay out", name)
		}
	}
}

func TestActivePaths_FutureSingletonDoesNotOwnQuarter(t *testing.T) {
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			10: bar(1), 11: bar(1), 12: bar(1),
		}}
	}
	paths["Zfuture"] = &symbolBars{byTS: map[int64]barOHLCV{
		1000: bar(1),
		1001: bar(1),
	}}
	active := activePaths(paths)
	if _, ok := active["Zfuture"]; ok {
		t.Fatalf("future singleton must not enter active: %v", keys(active))
	}
	if len(active) != 4 {
		t.Fatalf("active=%d want 4, names=%v", len(active), keys(active))
	}
}

func TestActivePaths_CoordinatedFuturePairDoesNotOwnQuarter(t *testing.T) {
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			10: bar(1), 11: bar(1), 12: bar(1),
		}}
	}
	paths["F1"] = &symbolBars{byTS: map[int64]barOHLCV{1000: bar(1), 1001: bar(1)}}
	paths["F2"] = &symbolBars{byTS: map[int64]barOHLCV{1000: bar(1), 1001: bar(1)}}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
	}
	for _, name := range []string{"F1", "F2"} {
		if _, ok := active[name]; ok {
			t.Fatalf("future pair %s must stay out", name)
		}
	}
}

func TestActivePaths_CoordinatedFutureTrioDoesNotOwnQuarter(t *testing.T) {
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			10: bar(1), 11: bar(1), 12: bar(1),
		}}
	}
	for i := 0; i < 3; i++ {
		name := string(rune('F' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{1000: bar(1), 1001: bar(1)}}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("future trio member %s must stay out", name)
		}
	}
}

func TestActivePaths_FutureTrioAfterLongLivePrefix(t *testing.T) {
	// ~200-bar live last-N; trio at +14d and +80d (4h bars) must not own the tip.
	const liveBars = 200
	const barsPerDay = 6 // 24/4
	for _, aheadDays := range []int{14, 80} {
		aheadDays := aheadDays
		t.Run(fmt.Sprintf("plus%dd", aheadDays), func(t *testing.T) {
			paths := map[string]*symbolBars{}
			for i := 0; i < 4; i++ {
				name := string(rune('A' + i))
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < liveBars; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			start := int64(liveBars - 1 + aheadDays*barsPerDay)
			for i := 0; i < 3; i++ {
				name := string(rune('F' + i))
				paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
					start:     bar(1),
					start + 1: bar(1),
				}}
			}
			active := activePaths(paths)
			if len(active) != 4 {
				t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
			}
			for name := range active {
				if name[0] == 'F' {
					t.Fatalf("future trio at +%dd stole active: %v", aheadDays, keys(active))
				}
			}
		})
	}
}

func TestActivePaths_LongStaleLastNKeepsFarLive(t *testing.T) {
	// Fetcher-shaped: 5 stale-only t0–t199; 4 live overlap last K of prefix
	// then resume after a 50-bar (week-scale) halt. Live must remain.
	const staleBars = 200
	const halt = 50
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	resume := int64(staleBars - 1 + halt)
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for ts := int64(staleBars - islandGapBars); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		byTS[resume] = bar(1)
		byTS[resume+1] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("stale symbol %s stole active: %v", name, keys(active))
		}
	}
}

func TestActivePaths_WeekendHaltKeepsContinuingLive(t *testing.T) {
	// 12-bar weekend hole: below K-strip, keep + cutoff bump must still hand
	// the clock to the resume island.
	const staleBars = 200
	const halt = 12
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	resume := int64(staleBars - 1 + halt)
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for ts := int64(staleBars - islandGapBars); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		byTS[resume] = bar(1)
		byTS[resume+1] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 after %d-bar weekend halt, names=%v", len(active), halt, keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("stale symbol %s stole active after weekend halt: %v", name, keys(active))
		}
	}
}

func TestActivePaths_FourBarHaltKeepsContinuingLive(t *testing.T) {
	// Common 4–16h hole on 4h bars: gap = 4×step > robustStep so the island
	// is detected (old 4×median missed this).
	const step int64 = 4 * 3600
	const staleBars = 200
	const halt = 4
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		byTS := map[int64]barOHLCV{}
		for n := int64(0); n < staleBars; n++ {
			byTS[n*step] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	resume := int64(staleBars - 1 + halt)
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for n := int64(staleBars - islandGapBars); n < staleBars; n++ {
			byTS[n*step] = bar(1)
		}
		byTS[resume*step] = bar(1)
		byTS[(resume+1)*step] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 after %d-bar halt, names=%v", len(active), halt, keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("stale symbol %s stole active after 4-bar halt: %v", name, keys(active))
		}
	}
}

func TestActivePaths_ResumedLastNNoOverlapKeepsLive(t *testing.T) {
	const staleBars = 200
	const halt = 50
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A'+i)) + "o"
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	start := int64(staleBars - 1 + halt)
	for i := 0; i < 4; i++ {
		name := string(rune('A'+i)) + "r"
		byTS := map[int64]barOHLCV{}
		for ts := start; ts < start+200; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 resumed last-N, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[len(name)-1] == 'o' {
			t.Fatalf("stale symbol %s stole active: %v", name, keys(active))
		}
	}
}

func TestActivePaths_NoOverlap99BarsStillStrips(t *testing.T) {
	// Must strip at weekend/4-bar holes too — not only when gap ≥ old K.
	const staleBars = 200
	for _, halt := range []int{4, 12, 50} {
		halt := halt
		t.Run(fmt.Sprintf("halt%d", halt), func(t *testing.T) {
			paths := map[string]*symbolBars{}
			for i := 0; i < 5; i++ {
				name := string(rune('A'+i)) + "o"
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < staleBars; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			start := int64(staleBars - 1 + halt)
			for i := 0; i < 4; i++ {
				name := string(rune('A'+i)) + "r"
				byTS := map[int64]barOHLCV{}
				for ts := start; ts < start+99; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			active := activePaths(paths)
			if len(active) != 5 {
				t.Fatalf("active=%d want 5 stale (99 < resumedLastNBars), names=%v", len(active), keys(active))
			}
			for name := range active {
				if name[len(name)-1] == 'r' {
					t.Fatalf("99-bar island stole active at halt %d: %v", halt, keys(active))
				}
			}
		})
	}
}

func TestActivePaths_NoOverlap100BarsKeepsLive(t *testing.T) {
	const staleBars = 200
	for _, halt := range []int{4, 12, 50} {
		halt := halt
		t.Run(fmt.Sprintf("halt%d", halt), func(t *testing.T) {
			paths := map[string]*symbolBars{}
			for i := 0; i < 5; i++ {
				name := string(rune('A'+i)) + "o"
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < staleBars; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			start := int64(staleBars - 1 + halt)
			for i := 0; i < 4; i++ {
				name := string(rune('A'+i)) + "r"
				byTS := map[int64]barOHLCV{}
				for ts := start; ts < start+100; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			active := activePaths(paths)
			if len(active) != 4 {
				t.Fatalf("active=%d want 4 live at resumedLastNBars, names=%v", len(active), keys(active))
			}
			for name := range active {
				if name[len(name)-1] == 'o' {
					t.Fatalf("stale symbol %s stole active: %v", name, keys(active))
				}
			}
		})
	}
}

func TestActivePaths_FutureQuartetAfterLongLivePrefix(t *testing.T) {
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('F' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			400: bar(1), 401: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 5 {
		t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("future quartet stole active: %v", keys(active))
		}
	}
}

func TestActivePaths_FutureQuartetAfterStaggeredPrefixStillStrips(t *testing.T) {
	// Five live with rotating miss → prefixMax = 4. A 2-bar quartet must not
	// keep via islandMax >= prefixMax (that path is stub-only).
	for _, liveBars := range []int{19, 200} {
		liveBars := liveBars
		t.Run(fmt.Sprintf("prefix%d", liveBars), func(t *testing.T) {
			paths := map[string]*symbolBars{}
			for i := 0; i < 5; i++ {
				name := string(rune('A' + i))
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < int64(liveBars); ts++ {
					if int(ts)%5 == i {
						continue
					}
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			for i := 0; i < 4; i++ {
				name := string(rune('F' + i))
				paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
					400: bar(1), 401: bar(1),
				}}
			}
			active := activePaths(paths)
			if len(active) != 5 {
				t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
			}
			for name := range active {
				if name[0] == 'F' {
					t.Fatalf("staggered-prefix quartet stole active: %v", keys(active))
				}
			}
		})
	}
}

func TestActivePaths_FutureQuartet20BarsStillStrips(t *testing.T) {
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('F' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(400); ts < 420; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 5 {
		t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("20-bar future quartet stole active: %v", keys(active))
		}
	}
}

func TestActivePaths_FutureQuartet20After20PrefixStillStrips(t *testing.T) {
	// Equal length ≠ keep; also pin |prefix|=19 (old short-prefix leave-behind).
	for _, liveBars := range []int{19, 20} {
		liveBars := liveBars
		for _, halt := range []int{4, 12} {
			halt := halt
			t.Run(fmt.Sprintf("prefix%d_halt%d", liveBars, halt), func(t *testing.T) {
				paths := map[string]*symbolBars{}
				for i := 0; i < 5; i++ {
					name := string(rune('A' + i))
					byTS := map[int64]barOHLCV{}
					for ts := int64(0); ts < int64(liveBars); ts++ {
						byTS[ts] = bar(1)
					}
					paths[name] = &symbolBars{byTS: byTS}
				}
				start := int64(liveBars - 1 + halt)
				for i := 0; i < 4; i++ {
					name := string(rune('F' + i))
					byTS := map[int64]barOHLCV{}
					for ts := start; ts < start+20; ts++ {
						byTS[ts] = bar(1)
					}
					paths[name] = &symbolBars{byTS: byTS}
				}
				active := activePaths(paths)
				if len(active) != 5 {
					t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
				}
				for name := range active {
					if name[0] == 'F' {
						t.Fatalf("future quartet stole active: %v", keys(active))
					}
				}
			})
		}
	}
}

func TestActivePaths_FutureQuartetOneOldTickStillStrips(t *testing.T) {
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('F' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			100: bar(1),
			400: bar(1),
			401: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 5 {
		t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("one-tick quartet stole active: %v", keys(active))
		}
	}
}

func TestActivePaths_HalfNewIslandStillStrips(t *testing.T) {
	// Two tip-continuing names must not keep an island that also has listing-only names.
	for _, islandBars := range []int{2, resumedLastNBars} {
		islandBars := islandBars
		t.Run(fmt.Sprintf("island%d", islandBars), func(t *testing.T) {
			const staleBars = 200
			const halt = 12
			paths := map[string]*symbolBars{}
			for i := 0; i < 5; i++ {
				name := string(rune('A'+i)) + "o"
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < staleBars; ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			resume := int64(staleBars - 1 + halt)
			for i := 0; i < 2; i++ {
				name := string(rune('A'+i)) + "r"
				byTS := map[int64]barOHLCV{}
				for ts := int64(staleBars - islandGapBars); ts < staleBars; ts++ {
					byTS[ts] = bar(1)
				}
				for ts := resume; ts < resume+int64(islandBars); ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			for i := 0; i < 2; i++ {
				name := string(rune('F' + i))
				byTS := map[int64]barOHLCV{}
				for ts := resume; ts < resume+int64(islandBars); ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			active := activePaths(paths)
			for i := 0; i < 5; i++ {
				name := string(rune('A'+i)) + "o"
				if _, ok := active[name]; !ok {
					t.Fatalf("stale clock owner %s missing, active=%v", name, keys(active))
				}
			}
			for name := range active {
				if name[0] == 'F' {
					t.Fatalf("listing-only %s rode along on half-new island: %v", name, keys(active))
				}
			}
		})
	}
}

func TestActivePaths_EqualDensityFutureFiveStillStrips(t *testing.T) {
	// 5-vs-5 two-bar future: islandMax >= need but printers are not continuing.
	// Also pin a stub-sized prefix so islandMax ≤ 3 cap blocks stub 5-vs-5 keep.
	for _, liveBars := range []int{7, 200} {
		liveBars := liveBars
		t.Run(fmt.Sprintf("prefix%d", liveBars), func(t *testing.T) {
			paths := map[string]*symbolBars{}
			for i := 0; i < 5; i++ {
				name := string(rune('A' + i))
				byTS := map[int64]barOHLCV{}
				for ts := int64(0); ts < int64(liveBars); ts++ {
					byTS[ts] = bar(1)
				}
				paths[name] = &symbolBars{byTS: byTS}
			}
			for i := 0; i < 5; i++ {
				name := string(rune('F' + i))
				paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
					400: bar(1), 401: bar(1),
				}}
			}
			active := activePaths(paths)
			if len(active) != 5 {
				t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
			}
			for name := range active {
				if name[0] == 'F' {
					t.Fatalf("5-vs-5 future stole active: %v", keys(active))
				}
			}
		})
	}
}

func TestActivePaths_FiveNewAfterFourLiveStillStrips(t *testing.T) {
	// Prefix never reaches need (4 < 5); listing island must not win via !hasNeed leave.
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 5; i++ {
		name := string(rune('F' + i))
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			400: bar(1), 401: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("5-new after 4-live stole active: %v", keys(active))
		}
	}
}

func TestActivePaths_NineTipOverlapIslandDoesNotOwnClock(t *testing.T) {
	// Clock-owner pin: 9 tip prints are not enough to keep the island, so the
	// live prefix owns the clock. Membership via those 9 stamps is allowed.
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('F' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(liveBars - 9); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		byTS[400] = bar(1)
		byTS[401] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		if _, ok := active[name]; !ok {
			t.Fatalf("live clock owner %s missing, active=%v", name, keys(active))
		}
	}
	onlyFuture := true
	for name := range active {
		if name[0] != 'F' {
			onlyFuture = false
			break
		}
	}
	if onlyFuture {
		t.Fatalf("future island owned the clock: %v", keys(active))
	}
}

func TestActivePaths_TenFrontOfWindowPrintsStillStrips(t *testing.T) {
	// 10 prints at the front of last-N do not count — only the tip window does.
	const liveBars = 200
	paths := map[string]*symbolBars{}
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < liveBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 4; i++ {
		name := string(rune('F' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < 10; ts++ {
			byTS[ts] = bar(1)
		}
		byTS[400] = bar(1)
		byTS[401] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 5 {
		t.Fatalf("active=%d want 5 live, names=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'F' {
			t.Fatalf("front-of-window quartet stole active: %v", keys(active))
		}
	}
}

func TestActivePaths_ContinuingLiveAfterLargerStale(t *testing.T) {
	const staleBars = 200
	const halt = 50
	paths := map[string]*symbolBars{}
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("o%d", i)
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	resume := int64(staleBars - 1 + halt)
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("r%d", i)
		byTS := map[int64]barOHLCV{}
		for ts := int64(staleBars - islandGapBars); ts < staleBars; ts++ {
			byTS[ts] = bar(1)
		}
		byTS[resume] = bar(1)
		byTS[resume+1] = bar(1)
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 30 {
		t.Fatalf("active=%d want 30 live, names sample=%v", len(active), keys(active))
	}
	for name := range active {
		if name[0] == 'o' {
			t.Fatalf("stale symbol %s stole active", name)
		}
	}
}

func TestActivePaths_OffGridSecondDoesNotBreakTipMiss(t *testing.T) {
	// One misaligned +1s print must not shrink robustStep to 1s and kill a 4h tip-miss.
	const step int64 = 4 * 3600
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for n := int64(0); n <= 10; n++ {
			byTS[n*step] = bar(1)
		}
		if i == 0 {
			byTS[5*step+1] = bar(1) // off-grid by 1s
		}
		if i < 3 {
			byTS[11*step] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 despite 1s noise, names=%v", len(active), keys(active))
	}
	if _, ok := active["D"]; !ok {
		t.Fatal("D must stay via prior dense run despite off-grid print")
	}
}

func TestActivePaths_LatestTipNotUnionOfEqualDensity(t *testing.T) {
	t.Run("withOld", func(t *testing.T) {
		paths := map[string]*symbolBars{}
		for i := 0; i < 3; i++ {
			name := string(rune('A'+i)) + "early"
			paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
				80: bar(1), 81: bar(1),
			}}
		}
		for i := 0; i < 3; i++ {
			name := string(rune('A'+i)) + "late"
			paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
				90: bar(1), 91: bar(1),
			}}
		}
		for i := 0; i < 2; i++ {
			name := string(rune('A'+i)) + "old"
			paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
				0: bar(1), 1: bar(1),
			}}
		}
		active := activePaths(paths)
		if len(active) != 3 {
			t.Fatalf("active=%d want 3 (latest tip only), names=%v", len(active), keys(active))
		}
		for name := range active {
			if len(name) < 4 || name[len(name)-4:] != "late" {
				t.Fatalf("expected only *late names, got %s in %v", name, keys(active))
			}
		}
	})
	t.Run("clean3vs3", func(t *testing.T) {
		// Majority island (need = 3) must still prefer latest under stub gate.
		paths := map[string]*symbolBars{}
		for i := 0; i < 3; i++ {
			name := string(rune('A'+i)) + "early"
			paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
				80: bar(1), 81: bar(1),
			}}
		}
		for i := 0; i < 3; i++ {
			name := string(rune('A'+i)) + "late"
			paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
				90: bar(1), 91: bar(1),
			}}
		}
		active := activePaths(paths)
		if len(active) != 3 {
			t.Fatalf("active=%d want 3 late, names=%v", len(active), keys(active))
		}
		for name := range active {
			if len(name) < 4 || name[len(name)-4:] != "late" {
				t.Fatalf("expected only *late names, got %s in %v", name, keys(active))
			}
		}
	})
}

func TestActivePaths_TipMissStaysOnPriorDenseRun(t *testing.T) {
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts <= 10; ts++ {
			byTS[ts] = bar(1)
		}
		if i < 3 {
			byTS[11] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 (miss on tip must not eject), names=%v", len(active), keys(active))
	}
	if _, ok := active["D"]; !ok {
		t.Fatal("D must stay via prior denseFloor bar")
	}
}

func TestActivePaths_FiveBarHoleTipMissStays(t *testing.T) {
	// Five-bar hole before tip inside the recent window: name on the prior
	// side that missed the tip must remain (runHoleBars=6 steps).
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts <= 18; ts++ {
			byTS[ts] = bar(1)
		}
		if i < 3 {
			byTS[24] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if len(active) != 4 {
		t.Fatalf("active=%d want 4 across 5-bar hole, names=%v", len(active), keys(active))
	}
	if _, ok := active["D"]; !ok {
		t.Fatal("D must stay across 5-bar hole via prior dense run")
	}
}

func TestActivePaths_SixBarHoleTipMissDrops(t *testing.T) {
	// Six missing bars → gap of 7 steps > runHoleBars; prior side is dropped.
	paths := map[string]*symbolBars{}
	for i := 0; i < 4; i++ {
		name := string(rune('A' + i))
		byTS := map[int64]barOHLCV{}
		for ts := int64(0); ts <= 25; ts++ {
			byTS[ts] = bar(1)
		}
		if i < 3 {
			byTS[32] = bar(1) // gap 25→32 = 7 steps; cutoff keeps 25 in recent
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	active := activePaths(paths)
	if _, ok := active["D"]; ok {
		t.Fatalf("D must drop across 6+ missing bars, active=%v", keys(active))
	}
	if len(active) != 3 {
		t.Fatalf("active=%d want 3 (tip-side only), names=%v", len(active), keys(active))
	}
}

func TestActivePaths_MinorityDisjointIgnored(t *testing.T) {
	paths := map[string]*symbolBars{}
	want := map[string]struct{}{}
	for i := 0; i < 3; i++ {
		name := string(rune('A'+i)) + "n"
		want[name] = struct{}{}
		byTS := map[int64]barOHLCV{}
		for ts := int64(10); ts < 10+resumedLastNBars; ts++ {
			byTS[ts] = bar(1)
		}
		paths[name] = &symbolBars{byTS: byTS}
	}
	for i := 0; i < 2; i++ {
		name := string(rune('A'+i)) + "x"
		paths[name] = &symbolBars{byTS: map[int64]barOHLCV{
			0: bar(1), 1: bar(1),
		}}
	}
	active := activePaths(paths)
	if len(active) != 3 {
		t.Fatalf("active=%d want 3, names=%v", len(active), keys(active))
	}
	for name := range want {
		if _, ok := active[name]; !ok {
			t.Fatalf("missing live name %s in active=%v", name, keys(active))
		}
	}
	for name := range active {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected name %s in active=%v", name, keys(active))
		}
	}
}

func keys(m map[string]*symbolBars) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
