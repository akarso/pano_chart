package metrics

import (
	"context"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// DefaultMinSectorSymbols is the minimum contributing members for a published sector.
const DefaultMinSectorSymbols = 2

// emptyUniverseLogGap limits "empty universe" spam during outages (uncached misses).
const emptyUniverseLogGap = time.Minute

// SectorsCalculator is the port for sector composite computation (PR-098).
type SectorsCalculator interface {
	Calculate(ctx context.Context, timeframe string, limit int) (mkt.SectorIndexResult, error)
}

// emptyUniverseThrottle rate-limits empty-universe log lines per service instance.
type emptyUniverseThrottle struct {
	mu   sync.Mutex
	last time.Time
}

// SectorIndexService builds per-sector composites with RS vs a market tape
// assembled from the same bar fetch (one fan-out). RS uses overlapping
// timestamps with the market series (clamped window).
type SectorIndexService struct {
	composite    *CompositeIndexService
	catalog      *SectorCatalog
	minSymbols   int
	includeOther bool
	emptyLog     *emptyUniverseThrottle
}

// NewSectorIndexService constructs the service from a composite (provider+filter)
// and a sector catalog.
func NewSectorIndexService(
	composite *CompositeIndexService,
	catalog *SectorCatalog,
) *SectorIndexService {
	return &SectorIndexService{
		composite:  composite,
		catalog:    catalog,
		minSymbols: DefaultMinSectorSymbols,
		emptyLog:   &emptyUniverseThrottle{},
	}
}

// WithMinSymbols returns a copy with an overridden membership floor (tests).
func (s *SectorIndexService) WithMinSymbols(n int) *SectorIndexService {
	if s == nil {
		return nil
	}
	out := *s
	if n < 1 {
		n = 1
	}
	out.minSymbols = n
	return &out
}

// WithIncludeOther returns a copy that publishes the synthetic "other" bucket (tests).
func (s *SectorIndexService) WithIncludeOther(v bool) *SectorIndexService {
	if s == nil {
		return nil
	}
	out := *s
	out.includeOther = v
	return &out
}

// Calculate returns configured sectors sorted by RS desc (unavailable last).
func (s *SectorIndexService) Calculate(
	ctx context.Context,
	timeframe string,
	limit int,
) (mkt.SectorIndexResult, error) {
	empty := mkt.SectorIndexResult{Timeframe: timeframe, Sectors: nil}
	if s == nil || s.catalog == nil || s.composite == nil {
		return empty, nil
	}
	if limit <= 0 {
		limit = 200
	}

	tf, paths, err := s.composite.loadSymbolBars(ctx, timeframe, limit)
	if err != nil {
		return empty, err
	}
	if len(paths) == 0 {
		s.logEmptyUniverse(timeframe)
		return empty, nil
	}

	marketTape := assembleTape(timeframe, tf, paths, limit)

	universe := make([]string, 0, len(paths))
	for sym := range paths {
		universe = append(universe, sym)
	}
	sort.Strings(universe)

	sectors := s.configuredSectors(timeframe, tf, paths, universe, marketTape, limit)
	if s.includeOther {
		if other, ok := s.otherSector(timeframe, tf, paths, universe, marketTape, limit); ok {
			sectors = append(sectors, other)
		}
	}
	sortByRS(sectors)

	return mkt.SectorIndexResult{
		Timeframe:         timeframe,
		MarketSymbolCount: marketTape.Index.SymbolCount,
		Sectors:           sectors,
	}, nil
}

func (s *SectorIndexService) logEmptyUniverse(timeframe string) {
	th := s.emptyLog
	if th == nil {
		th = &emptyUniverseThrottle{}
	}
	th.mu.Lock()
	defer th.mu.Unlock()
	now := time.Now()
	if !th.last.IsZero() && now.Sub(th.last) < emptyUniverseLogGap {
		return
	}
	th.last = now
	log.Printf("[sectors] empty universe for timeframe=%s", timeframe)
}

func (s *SectorIndexService) configuredSectors(
	timeframe string,
	tf domain.Timeframe,
	paths map[string]*symbolBars,
	universe []string,
	marketTape CompositeTape,
	limit int,
) []mkt.SectorIndex {
	defs := s.catalog.Sectors()
	out := make([]mkt.SectorIndex, 0, len(defs))
	var belowMin, assembleFail int
	for _, def := range defs {
		members := s.catalog.Members(def.ID, universe)
		idx, reason := s.sectorFromMembers(
			timeframe, tf, paths, members, def.ID, def.Name, marketTape, limit,
		)
		switch reason {
		case sectorOK, sectorUnavailable:
			out = append(out, idx)
		case sectorAssembleFail:
			assembleFail++
		case sectorBelowMin:
			belowMin++
		}
	}
	if belowMin > 0 || assembleFail > 0 {
		log.Printf("[sectors] skipped %d below-min, %d assemble-failed", belowMin, assembleFail)
	}
	return out
}

func (s *SectorIndexService) otherSector(
	timeframe string,
	tf domain.Timeframe,
	paths map[string]*symbolBars,
	universe []string,
	marketTape CompositeTape,
	limit int,
) (mkt.SectorIndex, bool) {
	members := make([]string, 0)
	for _, sym := range universe {
		if s.catalog.ForSymbol(sym) == reservedSectorOther {
			members = append(members, sym)
		}
	}
	idx, reason := s.sectorFromMembers(
		timeframe, tf, paths, members, reservedSectorOther, "Other", marketTape, limit,
	)
	return idx, reason == sectorOK || reason == sectorUnavailable
}

type sectorBuildReason string

const (
	sectorOK           sectorBuildReason = "ok"
	sectorBelowMin     sectorBuildReason = "below_min"
	sectorAssembleFail sectorBuildReason = "assemble_failed"
	sectorUnavailable  sectorBuildReason = "rs_unavailable"
)

func (s *SectorIndexService) sectorFromMembers(
	timeframe string,
	tf domain.Timeframe,
	paths map[string]*symbolBars,
	members []string,
	id, name string,
	marketTape CompositeTape,
	limit int,
) (mkt.SectorIndex, sectorBuildReason) {
	part := partitionPaths(paths, members)
	if len(part) == 0 {
		return mkt.SectorIndex{}, sectorBelowMin
	}
	tape := assembleTape(timeframe, tf, part, limit)
	if tape.Index.SymbolCount < s.minSymbols {
		return mkt.SectorIndex{}, sectorBelowMin
	}
	sectorPts, marketPts, pathOK := matchingPathPoints(tape, marketTape)
	if !pathOK {
		return mkt.SectorIndex{}, sectorAssembleFail
	}
	win, aligned := alignedReturns(sectorPts, marketPts)
	if !aligned {
		// Own-series return is still meaningful; RS is not.
		ownRet := indexLogReturn(sectorPts)
		return mkt.SectorIndex{
			ID:          id,
			Name:        name,
			SymbolCount: tape.Index.SymbolCount,
			Points:      sectorPts,
			Return:      ownRet,
			RS:          0,
			RSAvailable: false,
		}, sectorUnavailable
	}
	return mkt.SectorIndex{
		ID:          id,
		Name:        name,
		SymbolCount: tape.Index.SymbolCount,
		Points:      win.clamped, // same window as return/rs; rebased to 100
		Return:      win.sectorRet,
		RS:          win.rs,
		RSAvailable: true,
	}, sectorOK
}

func partitionPaths(paths map[string]*symbolBars, members []string) map[string]*symbolBars {
	out := make(map[string]*symbolBars, len(members))
	for _, sym := range members {
		if p, ok := paths[sym]; ok {
			out[sym] = p
		}
	}
	return out
}

func sortByRS(sectors []mkt.SectorIndex) {
	sort.SliceStable(sectors, func(i, j int) bool {
		if sectors[i].RSAvailable != sectors[j].RSAvailable {
			return sectors[i].RSAvailable
		}
		if sectors[i].RSAvailable && sectors[i].RS != sectors[j].RS {
			return sectors[i].RS > sectors[j].RS
		}
		return sectors[i].ID < sectors[j].ID
	})
}

// matchingPathPoints forces the same composite path on both sides.
// VW only when both sides have it; otherwise median on both.
func matchingPathPoints(sector, market CompositeTape) (sectorPts, marketPts []mkt.IndexPoint, ok bool) {
	if sp, sok := pointsForPath(sector, "composite_volume_weighted"); sok {
		if mp, mok := pointsForPath(market, "composite_volume_weighted"); mok {
			return sp, mp, true
		}
	}
	sp, sok := pointsForPath(sector, "composite_median")
	mp, mok := pointsForPath(market, "composite_median")
	if sok && mok {
		return sp, mp, true
	}
	return nil, nil, false
}

// pointsForPath returns the named path with no silent fallback.
func pointsForPath(tape CompositeTape, path string) ([]mkt.IndexPoint, bool) {
	switch path {
	case "composite_volume_weighted":
		if len(tape.Index.VolumeWeightedPoints) >= 2 {
			return append([]mkt.IndexPoint(nil), tape.Index.VolumeWeightedPoints...), true
		}
		return nil, false
	default:
		if len(tape.Index.Points) >= 2 {
			return append([]mkt.IndexPoint(nil), tape.Index.Points...), true
		}
		return nil, false
	}
}

// alignedWindow is the clamped, rebased sector series plus overlap returns.
type alignedWindow struct {
	clamped   []mkt.IndexPoint
	sectorRet float64
	marketRet float64
	rs        float64
}

// alignedReturns computes sector and market log-returns over the intersection
// of their timestamps (clamped window). Points are rebased so the first shared
// stamp is 100 (same convention as composite series); return/rs are ratios and
// are unchanged by the rebase.
func alignedReturns(sectorPts, marketPts []mkt.IndexPoint) (alignedWindow, bool) {
	if len(sectorPts) < 2 || len(marketPts) < 2 {
		return alignedWindow{}, false
	}
	marketByTS := make(map[int64]float64, len(marketPts))
	for _, p := range marketPts {
		if p.Value > 0 {
			marketByTS[p.Timestamp] = p.Value
		}
	}

	clamped := make([]mkt.IndexPoint, 0, len(sectorPts))
	var mFirst, mLast float64
	for _, p := range sectorPts {
		if p.Value <= 0 {
			continue
		}
		mv, found := marketByTS[p.Timestamp]
		if !found {
			continue
		}
		if len(clamped) == 0 {
			mFirst = mv
		}
		mLast = mv
		clamped = append(clamped, p)
	}
	if len(clamped) < 2 || mFirst <= 0 {
		return alignedWindow{}, false
	}
	sectorRet := math.Log(clamped[len(clamped)-1].Value / clamped[0].Value)
	marketRet := math.Log(mLast / mFirst)
	rebaseIndexTo100(clamped)
	return alignedWindow{
		clamped:   clamped,
		sectorRet: sectorRet,
		marketRet: marketRet,
		rs:        sectorRet - marketRet,
	}, true
}

func rebaseIndexTo100(pts []mkt.IndexPoint) {
	if len(pts) == 0 || pts[0].Value <= 0 {
		return
	}
	base := pts[0].Value
	for i := range pts {
		pts[i].Value = pts[i].Value / base * 100
	}
}

func indexLogReturn(pts []mkt.IndexPoint) float64 {
	if len(pts) < 2 {
		return 0
	}
	first := pts[0].Value
	last := pts[len(pts)-1].Value
	if first <= 0 || last <= 0 {
		return 0
	}
	return math.Log(last / first)
}
