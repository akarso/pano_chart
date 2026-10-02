package transition

import (
	"math"
	"sort"

	mkt "pano_chart/backend/domain/market"
)

// AgeBucket classifies how old a regime period was relative to the median
// duration of that regime (PR-107).
type AgeBucket int

const (
	// AgeYoung: duration < 25% of median.
	AgeYoung AgeBucket = iota
	// AgeMid: 25–75% of median.
	AgeMid
	// AgeMature: 75–150% of median.
	AgeMature
	// AgeStale: > 150% of median.
	AgeStale
	ageBucketCount
)

const (
	laplacePseudoCount   = 1.0
	minBlendSamples      = 30 // also the "confident" threshold for blending
	empiricalWeightCap   = 0.7
	empiricalWeightDenom = 100.0
)

// transitionTargets are the four regimes exposed as TransitionProbabilities.
var transitionTargets = []mkt.Regime{
	mkt.RegimeTrend,
	mkt.RegimeSideways,
	mkt.RegimeCompression,
	mkt.RegimeExpansion,
}

var fromIndex = map[mkt.Regime]int{
	mkt.RegimeTrend: 0, mkt.RegimeSideways: 1,
	mkt.RegimeCompression: 2, mkt.RegimeExpansion: 3,
}

// Matrix is an empirical from → (age bucket) → to probability table.
type Matrix struct {
	nFrom     map[mkt.Regime]int
	nBucket   map[mkt.Regime][ageBucketCount]int
	medianDur map[mkt.Regime]float64
	// probs only populated for buckets with raw count > 0.
	probs map[mkt.Regime][ageBucketCount]mkt.TransitionProbabilities
	// pooled is Laplace over all age buckets for from (when nFrom > 0).
	pooled map[mkt.Regime]mkt.TransitionProbabilities
}

// LookupResult is the empirical row actually used for blending.
type LookupResult struct {
	Probabilities mkt.TransitionProbabilities
	SampleSize    int  // raw counts for the bucket or pooled row used
	Pooled        bool // true when the all-age row was used
}

// BuildMatrix counts consecutive regime transitions grouped by the age bucket
// of the ending "from" period, applies Laplace smoothing (+1 per target) only
// to buckets that saw at least one transition, and builds a pooled all-age row.
func BuildMatrix(periods []mkt.RegimePeriod) Matrix {
	m := Matrix{
		nFrom:     make(map[mkt.Regime]int),
		nBucket:   make(map[mkt.Regime][ageBucketCount]int),
		medianDur: make(map[mkt.Regime]float64),
		probs:     make(map[mkt.Regime][ageBucketCount]mkt.TransitionProbabilities),
		pooled:    make(map[mkt.Regime]mkt.TransitionProbabilities),
	}

	segments := normalizeAndMerge(periods)
	nPeriods := 0
	for _, seg := range segments {
		nPeriods += len(seg.periods)
	}
	if nPeriods < 2 {
		return m
	}

	// Median from closed periods only (exclude trailing open current). Closed
	// stubs absorbed into an open continuation are kept via medianClosed.
	durs := make(map[mkt.Regime][]int)
	for _, seg := range segments {
		for r, xs := range seg.medianClosed {
			durs[r] = append(durs[r], xs...)
		}
		for i, p := range seg.periods {
			openTail := i == len(seg.periods)-1 && p.EndTimestamp == nil
			if openTail {
				continue
			}
			if p.DurationCandles > 0 {
				durs[p.Regime] = append(durs[p.Regime], p.DurationCandles)
			}
		}
	}
	for r, xs := range durs {
		m.medianDur[r] = medianInt(xs)
	}

	var counts [4][ageBucketCount][4]float64
	var pooledCounts [4][4]float64

	for _, seg := range segments {
		for i := 0; i+1 < len(seg.periods); i++ {
			from := seg.periods[i]
			to := seg.periods[i+1]
			if from.DurationCandles <= 0 {
				continue
			}
			fi, fok := fromIndex[from.Regime]
			ti, tok := fromIndex[to.Regime]
			if !fok || !tok {
				continue
			}
			b := ClassifyAge(from.DurationCandles, m.medianDur[from.Regime])
			counts[fi][b][ti]++
			pooledCounts[fi][ti]++
			m.nFrom[from.Regime]++
			nb := m.nBucket[from.Regime]
			nb[b]++
			m.nBucket[from.Regime] = nb
		}
	}

	for _, from := range transitionTargets {
		fi := fromIndex[from]
		nb := m.nBucket[from]
		var buckets [ageBucketCount]mkt.TransitionProbabilities
		for b := AgeBucket(0); b < ageBucketCount; b++ {
			if nb[b] == 0 {
				continue
			}
			buckets[b] = laplaceNormalizeCounts(counts[fi][b])
		}
		m.probs[from] = buckets
		if m.nFrom[from] > 0 {
			m.pooled[from] = laplaceNormalizeCounts(pooledCounts[fi])
		}
	}
	return m
}

type periodSegment struct {
	periods      []mkt.RegimePeriod
	medianClosed map[mkt.Regime][]int // closed stubs absorbed into an open row
}

// normalizeAndMerge maps silent/indecisive → sideways, splits on unknown
// regimes (no transitions across gaps), and merges consecutive equals within
// a segment so remapped churn does not invent sideways self-transitions.
// When a closed period is absorbed into a still-open continuation, its closed
// duration is recorded in medianClosed so the open-tail median skip does not
// drop that history.
func normalizeAndMerge(periods []mkt.RegimePeriod) []periodSegment {
	var segments []periodSegment
	var cur periodSegment
	flush := func() {
		if len(cur.periods) > 0 {
			segments = append(segments, cur)
			cur = periodSegment{}
		}
	}
	for _, p := range periods {
		r, ok := coreRegime(p.Regime)
		if !ok {
			flush()
			continue
		}
		p.Regime = r
		if len(cur.periods) > 0 && cur.periods[len(cur.periods)-1].Regime == p.Regime {
			last := &cur.periods[len(cur.periods)-1]
			if last.EndTimestamp != nil && p.EndTimestamp == nil {
				// Closed → open: keep closed duration for median before opening.
				if last.DurationCandles > 0 {
					if cur.medianClosed == nil {
						cur.medianClosed = make(map[mkt.Regime][]int)
					}
					cur.medianClosed[last.Regime] = append(cur.medianClosed[last.Regime], last.DurationCandles)
				}
			}
			last.DurationCandles += p.DurationCandles
			if p.EndTimestamp != nil {
				last.EndTimestamp = p.EndTimestamp
			} else {
				last.EndTimestamp = nil
			}
			continue
		}
		cur.periods = append(cur.periods, p)
	}
	flush()
	return segments
}

// ClassifyAge maps a period duration onto an AgeBucket given the regime median.
// Non-positive median → AgeMid (neutral).
func ClassifyAge(duration int, median float64) AgeBucket {
	if median <= 0 || duration < 0 {
		return AgeMid
	}
	r := float64(duration) / median
	switch {
	case r < 0.25:
		return AgeYoung
	case r < 0.75:
		return AgeMid
	case r <= 1.50:
		return AgeMature
	default:
		return AgeStale
	}
}

// TrailingMergedAge sums durations of the trailing run of periods that share
// the same core regime as the last period (silent/indecisive/sideways merge
// the same way as BuildMatrix). Used so live Lookup age matches how historical
// transitions were bucketed.
func TrailingMergedAge(periods []mkt.RegimePeriod) int {
	age, _ := TrailingMerged(periods)
	return age
}

// TrailingMerged returns the merged age and core regime of the trailing run.
func TrailingMerged(periods []mkt.RegimePeriod) (age int, regime mkt.Regime) {
	if len(periods) == 0 {
		return 0, ""
	}
	last, ok := coreRegime(periods[len(periods)-1].Regime)
	if !ok {
		return periods[len(periods)-1].DurationCandles, ""
	}
	sum := 0
	for i := len(periods) - 1; i >= 0; i-- {
		r, ok := coreRegime(periods[i].Regime)
		if !ok || r != last {
			break
		}
		sum += periods[i].DurationCandles
	}
	return sum, last
}

// WeightFromSamples is clamp(n/100, 0, 0.7).
func WeightFromSamples(n int) float64 {
	if n <= 0 {
		return 0
	}
	w := float64(n) / empiricalWeightDenom
	if w > empiricalWeightCap {
		return empiricalWeightCap
	}
	return w
}

// Lookup returns empirical probabilities for from at the age bucket implied by
// duration. Prefer the age bucket when it has ≥ minBlendSamples; otherwise
// prefer the pooled all-age row when that has ≥ minBlendSamples; otherwise
// return whichever sparse row exists (caller stays heuristic below the blend
// threshold). ok is false when from has no samples at all.
func (m Matrix) Lookup(from mkt.Regime, durationCandles int) (LookupResult, bool) {
	from, ok := coreRegime(from)
	if !ok || m.nFrom[from] == 0 {
		return LookupResult{}, false
	}
	b := ClassifyAge(durationCandles, m.medianDur[from])
	bucketN := m.nBucket[from][b]
	pooledN := m.nFrom[from]
	pooled, hasPooled := m.pooled[from]

	if bucketN >= minBlendSamples {
		return LookupResult{
			Probabilities: m.probs[from][b],
			SampleSize:    bucketN,
			Pooled:        false,
		}, true
	}
	if hasPooled && pooledN >= minBlendSamples {
		return LookupResult{
			Probabilities: pooled,
			SampleSize:    pooledN,
			Pooled:        true,
		}, true
	}
	if bucketN > 0 {
		return LookupResult{
			Probabilities: m.probs[from][b],
			SampleSize:    bucketN,
			Pooled:        false,
		}, true
	}
	if hasPooled {
		return LookupResult{
			Probabilities: pooled,
			SampleSize:    pooledN,
			Pooled:        true,
		}, true
	}
	return LookupResult{}, false
}

// Blend mixes empirical and heuristic probabilities:
// P = w×P_empirical + (1−w)×P_heuristic. w is clamped to [0, empiricalWeightCap].
func Blend(empirical, heuristic mkt.TransitionProbabilities, w float64) mkt.TransitionProbabilities {
	if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
		w = 0
	}
	if w > empiricalWeightCap {
		w = empiricalWeightCap
	}
	iw := 1 - w
	return mkt.TransitionProbabilities{
		Trend:       w*empirical.Trend + iw*heuristic.Trend,
		Sideways:    w*empirical.Sideways + iw*heuristic.Sideways,
		Compression: w*empirical.Compression + iw*heuristic.Compression,
		Expansion:   w*empirical.Expansion + iw*heuristic.Expansion,
	}
}

func coreRegime(r mkt.Regime) (mkt.Regime, bool) {
	switch r {
	case mkt.RegimeTrend, mkt.RegimeSideways, mkt.RegimeCompression, mkt.RegimeExpansion:
		return r, true
	case mkt.RegimeIndecisive, mkt.RegimeSilent:
		return mkt.RegimeSideways, true
	default:
		return "", false
	}
}

func laplaceNormalizeCounts(raw [4]float64) mkt.TransitionProbabilities {
	var sum float64
	out := raw
	for i := range out {
		out[i] += laplacePseudoCount
		sum += out[i]
	}
	return mkt.TransitionProbabilities{
		Trend:       out[0] / sum,
		Sideways:    out[1] / sum,
		Compression: out[2] / sum,
		Expansion:   out[3] / sum,
	}
}

func medianInt(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]int(nil), xs...)
	sort.Ints(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return float64(cp[mid])
	}
	return float64(cp[mid-1]+cp[mid]) / 2
}
