package usecases

import (
	"math"
	"sort"

	"pano_chart/backend/application/market/metrics"
)

// minRSOverlapBars is the preferred absolute floor for scoring RS. When the
// tape is shorter, we use ceil(len(tape)/2) but never less than 2.
const minRSOverlapBars = 20

// minOverlapRequired is how many shared stamps a row needs before RS is set.
func minOverlapRequired(tapeLen int) int {
	need := minRSOverlapBars
	if half := (tapeLen + 1) / 2; half < need {
		need = half
	}
	if need < 2 {
		need = 2
	}
	return need
}

// tapeCloseByTS maps preferred-series (or index) timestamps to closes.
func tapeCloseByTS(tape metrics.CompositeTape) map[int64]float64 {
	series := tape.PreferredSeries()
	if series.Len() >= 2 {
		all := series.All()
		out := make(map[int64]float64, len(all))
		for _, c := range all {
			cl := c.Close()
			if cl > 0 {
				out[c.Timestamp().Unix()] = cl
			}
		}
		if len(out) >= 2 {
			return out
		}
	}
	pts := tape.Index.VolumeWeightedPoints
	if len(pts) < 2 {
		pts = tape.Index.Points
	}
	out := make(map[int64]float64, len(pts))
	for _, p := range pts {
		if p.Value > 0 {
			out[p.Timestamp] = p.Value
		}
	}
	return out
}

// alignedOverlap returns spark and tape closes on the shared timestamp set,
// in spark chronological order. Both slices have the same length and calendar.
func alignedOverlap(sparkCloses []float64, sparkTS []int64, tape map[int64]float64) (sym, mkt []float64) {
	if len(sparkCloses) != len(sparkTS) || len(tape) == 0 {
		return nil, nil
	}
	for i, ts := range sparkTS {
		tc, ok := tape[ts]
		if !ok || tc <= 0 || sparkCloses[i] <= 0 {
			continue
		}
		sym = append(sym, sparkCloses[i])
		mkt = append(mkt, tc)
	}
	return sym, mkt
}

// symbolSkipper reports symbols that must not receive RS (stables / wrappers).
type symbolSkipper interface {
	Skip(sym string) bool
}

// applyRelativeStrength fills RS/Beta/RSRank from timestamp overlap with the
// tape. Rows below minOverlapRequired, excluded names, or bad prices stay
// unset. Returns the number of scored rows.
func applyRelativeStrength(results []RankedResult, tape map[int64]float64, skip symbolSkipper) int {
	if len(results) == 0 || len(tape) < 2 {
		return 0
	}
	need := minOverlapRequired(len(tape))
	scored := 0
	for i := range results {
		if skip != nil && skip.Skip(results[i].Symbol.String()) {
			continue
		}
		sym, mkt := alignedOverlap(results[i].Sparkline, results[i].sparkTS, tape)
		if len(sym) < need {
			continue
		}
		rs := math.Log(sym[len(sym)-1]/sym[0]) - math.Log(mkt[len(mkt)-1]/mkt[0])
		beta := olsBetaAligned(sym, mkt)
		results[i].RelativeStrength = &rs
		results[i].Beta = &beta
		scored++
	}
	assignRSRank(results)
	return scored
}

// rsZeroScoredTransient reports whether a usable tape with zero scored rows
// should skip Redis caching. Incomplete candle coverage always skips cache
// (missing ranking rows are unrelated to the RS floor). Capable-but-unaligned
// rows may recover; precision below the floor, all-excluded, and all-short
// history on a complete board are treated as stable.
func rsZeroScoredTransient(results []RankedResult, tape map[int64]float64, skip symbolSkipper, precision, universeN int) bool {
	// Incomplete board — do not cache even when precision cannot score RS.
	if universeN > 0 && len(results) < universeN {
		return true
	}
	need := minOverlapRequired(len(tape))
	if precision > 0 && precision < need {
		return false
	}
	eligible, capable := 0, 0
	for i := range results {
		if skip != nil && skip.Skip(results[i].Symbol.String()) {
			continue
		}
		eligible++
		if len(results[i].sparkTS) >= need {
			capable++
		}
	}
	if eligible == 0 {
		return false
	}
	return capable > 0
}

// olsBetaAligned is OLS of symbol log-returns on tape log-returns for two
// equal-length, timestamp-aligned close series. Consecutive overlap points
// are one return period even if the tape skipped intermediate bars (explicit
// choice — not time-weighted). Returns 0 when tape variance < 1e-12.
func olsBetaAligned(sym, mkt []float64) float64 {
	if len(sym) != len(mkt) || len(sym) < 2 {
		return 0
	}
	var sx, sy, sxx, sxy, count float64
	for i := 1; i < len(sym); i++ {
		if sym[i] <= 0 || sym[i-1] <= 0 || mkt[i] <= 0 || mkt[i-1] <= 0 {
			continue
		}
		x := math.Log(mkt[i] / mkt[i-1])
		y := math.Log(sym[i] / sym[i-1])
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
		count++
	}
	if count < 1 {
		return 0
	}
	meanX := sx / count
	varX := sxx/count - meanX*meanX
	if varX < 1e-12 {
		return 0
	}
	denom := count*sxx - sx*sx
	if math.Abs(denom) < 1e-18 {
		return 0
	}
	return (count*sxy - sx*sy) / denom
}

// assignRSRank sets RSRank among rows that have RelativeStrength set.
// Unscored rows keep RSRank nil.
func assignRSRank(results []RankedResult) {
	indices := make([]int, 0, len(results))
	for i := range results {
		if results[i].RelativeStrength != nil {
			indices = append(indices, i)
		}
	}
	n := len(indices)
	if n == 0 {
		return
	}
	sort.Slice(indices, func(a, b int) bool {
		ra := *results[indices[a]].RelativeStrength
		rb := *results[indices[b]].RelativeStrength
		if ra != rb {
			return ra > rb
		}
		return results[indices[a]].Symbol.String() < results[indices[b]].Symbol.String()
	})
	for rank, idx := range indices {
		var p float64
		if n <= 1 {
			p = 1.0
		} else {
			p = 1.0 - float64(rank)/float64(n-1)
		}
		results[idx].RSRank = &p
	}
}

// rsSortKey returns the descending sort metric for leaders/laggards.
// Unscored rows sort last (math.Inf(-1) under descending order).
func rsSortKey(r RankedResult, laggards bool) float64 {
	if r.RelativeStrength == nil {
		return math.Inf(-1)
	}
	if laggards {
		return -*r.RelativeStrength
	}
	return *r.RelativeStrength
}
