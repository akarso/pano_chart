package http

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"pano_chart/backend/application/market/metrics"
	vol "pano_chart/backend/infrastructure/volatility"
)

// volatilityResultSource is the minimal surface VolatilitySeasonalityProvider
// needs from VolatilityHandler — kept as a small interface (rather than a
// concrete *VolatilityHandler field) so tests can fake it without spinning
// up a real handler/JSON file.
type volatilityResultSource interface {
	CurrentResult() (*vol.FullResult, error)
}

// SectorProfileHandler is a reloadable sector JSON source (production:
// *VolatilityHandler; tests inject fakes via SetSectorHandlerFactory).
type SectorProfileHandler interface {
	CurrentResult() (*vol.FullResult, error)
	Reload() error
}

// sectorEntry caches a loaded sector handler or a negative miss so Evaluate
// does not probe disk on every request (PR-108).
type sectorEntry struct {
	handler SectorProfileHandler // non-nil when file was loadable
	missing bool                 // true when file was absent/unloadable at last probe
}

// VolatilitySeasonalityProvider implements setups.SeasonalityProvider by
// looking up the current moment's historical spike probability from the
// same precomputed volatility profile VolatilityHandler serves — see
// PR-082. The reported probability is the max of two independent seasonal
// reads: the current UTC minute-of-day (intraday) and the current
// minute-of-week (day-of-week — e.g. weekend lull, Monday-open effects).
//
// PR-108: optional sectorPrefix (path stem, e.g. "/data/vol") loads
// metrics.SectorProfilePath(prefix, sector) files; missing/corrupt sector
// files fall back to the market-wide source after a warn log. Misses are
// negative-cached until ReloadSectors. Sparse-minute spike misses keep the
// loaded handler cached and only fall back for that request.
type VolatilitySeasonalityProvider struct {
	source       volatilityResultSource
	sectorPrefix string // stem: "/data/vol" → "/data/vol_l1.json"
	now          func() time.Time
	logf         func(format string, args ...any)
	// newSectorHandler builds a reloadable source for a profile path (tests).
	newSectorHandler func(path string) SectorProfileHandler

	mu      sync.Mutex
	sectors map[string]*sectorEntry
}

// NewVolatilitySeasonalityProvider constructs the provider over an existing
// *VolatilityHandler (or any type satisfying volatilityResultSource).
func NewVolatilitySeasonalityProvider(source volatilityResultSource) *VolatilitySeasonalityProvider {
	return &VolatilitySeasonalityProvider{
		source:  source,
		now:     time.Now,
		logf:    log.Printf,
		sectors: make(map[string]*sectorEntry),
		newSectorHandler: func(path string) SectorProfileHandler {
			return NewVolatilityHandler(path)
		},
	}
}

// NewVolatilitySeasonalityProviderWithClock is NewVolatilitySeasonalityProvider
// with an injectable clock, so tests can pin "now" instead of racing
// time.Now() against a minute boundary — CR follow-up.
func NewVolatilitySeasonalityProviderWithClock(source volatilityResultSource, now func() time.Time) *VolatilitySeasonalityProvider {
	p := NewVolatilitySeasonalityProvider(source)
	p.now = now
	return p
}

// SetSectorPrefix configures the path stem shared with vol_aggregate --out
// (e.g. "/data/vol" → "/data/vol_l1.json" via SectorProfilePath). Empty
// disables sector lookup.
func (p *VolatilitySeasonalityProvider) SetSectorPrefix(prefix string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sectorPrefix = strings.TrimSpace(prefix)
	p.sectors = make(map[string]*sectorEntry)
}

// SectorPrefix returns the configured stem (tests / startup diagnostics).
func (p *VolatilitySeasonalityProvider) SectorPrefix() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sectorPrefix
}

// SetLogger overrides the warn logger (tests).
func (p *VolatilitySeasonalityProvider) SetLogger(fn func(string, ...any)) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if fn == nil {
		p.logf = log.Printf
		return
	}
	p.logf = fn
}

// SetSectorHandlerFactory overrides how sector profile paths are opened (tests).
func (p *VolatilitySeasonalityProvider) SetSectorHandlerFactory(fn func(path string) SectorProfileHandler) {
	if p == nil || fn == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.newSectorHandler = fn
}

// ReloadSectors reloads every loaded sector handler and clears negative
// misses so newly written files are discovered — same cadence as the
// market-wide volatilityReloadLoop (PR-108).
func (p *VolatilitySeasonalityProvider) ReloadSectors() {
	if p == nil {
		return
	}
	p.mu.Lock()
	logf := p.logf
	type item struct {
		id string
		h  SectorProfileHandler
	}
	var toReload []item
	for id, e := range p.sectors {
		if e == nil || e.missing || e.handler == nil {
			delete(p.sectors, id)
			continue
		}
		toReload = append(toReload, item{id: id, h: e.handler})
	}
	p.mu.Unlock()

	if logf == nil {
		logf = log.Printf
	}
	var drop []string
	for _, it := range toReload {
		if err := it.h.Reload(); err != nil {
			logf("[seasonality] sector %s reload failed (%v); dropping cache entry", it.id, err)
			drop = append(drop, it.id)
		}
	}
	if len(drop) == 0 {
		return
	}
	p.mu.Lock()
	for _, id := range drop {
		delete(p.sectors, id)
	}
	p.mu.Unlock()
}

// CurrentSpikeProbability implements setups.SeasonalityProvider (market-wide).
func (p *VolatilitySeasonalityProvider) CurrentSpikeProbability(_ context.Context, _ string) (float64, error) {
	return p.spikeFrom(p.source, "market")
}

// CurrentSpikeProbabilityFor prefers a sector profile when present; otherwise
// falls back to the market-wide curve (missing file, "other", empty sector,
// or per-request spike lookup failure — the latter keeps the handler cached).
func (p *VolatilitySeasonalityProvider) CurrentSpikeProbabilityFor(ctx context.Context, sector, timeframe string) (float64, error) {
	id, err := metrics.NormalizeSectorID(sector)
	if err != nil {
		p.warnf("invalid sector id %q (%v); using market-wide", sector, err)
		return p.CurrentSpikeProbability(ctx, timeframe)
	}
	if id == "other" {
		return p.CurrentSpikeProbability(ctx, timeframe)
	}
	src, ok := p.sectorSource(id)
	if !ok {
		return p.CurrentSpikeProbability(ctx, timeframe)
	}
	v, err := p.spikeFrom(src, "sector:"+id)
	if err != nil {
		// Keep the loaded handler: sparse buckets / minute misses are expected
		// after intersection averaging; only fall back for this request.
		p.warnf("sector %s spike lookup failed (%v); falling back to market-wide", id, err)
		return p.CurrentSpikeProbability(ctx, timeframe)
	}
	return v, nil
}

func (p *VolatilitySeasonalityProvider) warnf(format string, args ...any) {
	p.mu.Lock()
	logf := p.logf
	p.mu.Unlock()
	if logf == nil {
		logf = log.Printf
	}
	logf("[seasonality] "+format, args...)
}

func (p *VolatilitySeasonalityProvider) sectorSource(sector string) (volatilityResultSource, bool) {
	p.mu.Lock()
	prefix := p.sectorPrefix
	if prefix == "" {
		p.mu.Unlock()
		return nil, false
	}
	if e, ok := p.sectors[sector]; ok {
		h := e.handler
		missing := e.missing || h == nil
		p.mu.Unlock()
		if missing {
			return nil, false
		}
		return h, true
	}
	factory := p.newSectorHandler
	logf := p.logf
	p.mu.Unlock()

	if factory == nil {
		factory = func(path string) SectorProfileHandler { return NewVolatilityHandler(path) }
	}
	if logf == nil {
		logf = log.Printf
	}

	path, err := metrics.SectorProfilePath(prefix, sector)
	if err != nil {
		p.storeSectorMiss(sector)
		return nil, false
	}

	h := factory(path)
	if _, err := h.CurrentResult(); err != nil {
		logf("[seasonality] sector profile %s unavailable (%v); market-wide until reload", path, err)
		p.storeSectorMiss(sector)
		return nil, false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Another goroutine may have won the probe.
	if e, ok := p.sectors[sector]; ok {
		if e.missing || e.handler == nil {
			return nil, false
		}
		return e.handler, true
	}
	p.sectors[sector] = &sectorEntry{handler: h}
	return h, true
}

func (p *VolatilitySeasonalityProvider) storeSectorMiss(sector string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.sectors[sector]; ok {
		return
	}
	p.sectors[sector] = &sectorEntry{missing: true}
}

func (p *VolatilitySeasonalityProvider) spikeFrom(source volatilityResultSource, label string) (float64, error) {
	if source == nil {
		return 0, fmt.Errorf("volatility seasonality: nil source (%s)", label)
	}
	result, err := source.CurrentResult()
	if err != nil {
		return 0, fmt.Errorf("volatility seasonality: %w", err)
	}

	var intraday []vol.BucketResult
	for _, entry := range result.Intraday {
		if entry.Timeframe == vol.TF1m {
			intraday = entry.Buckets
			break
		}
	}
	if len(intraday) == 0 {
		return 0, fmt.Errorf("volatility seasonality: no 1m buckets available")
	}

	nowFn := p.now
	if nowFn == nil {
		nowFn = time.Now
	}
	t := nowFn().UTC()
	minuteOfDay := t.Hour()*60 + t.Minute()

	spikeProb, ok := lookupBucketSpikeProb(intraday, minuteOfDay)
	if !ok {
		return 0, fmt.Errorf("volatility seasonality: no bucket for minute-of-day %d", minuteOfDay)
	}

	minuteOfWeek := int(t.Weekday())*1440 + minuteOfDay
	if weeklySpikeProb, ok := lookupWeeklyBucketSpikeProb(result.Weekly.Buckets, minuteOfWeek); ok && weeklySpikeProb > spikeProb {
		spikeProb = weeklySpikeProb
	}

	return spikeProb, nil
}

const maxMinuteSearchRadius = 15

func lookupBucketSpikeProb(buckets []vol.BucketResult, minuteOfDay int) (float64, bool) {
	if minuteOfDay >= 0 && minuteOfDay < len(buckets) && buckets[minuteOfDay].MinuteOfDay == minuteOfDay {
		return buckets[minuteOfDay].SpikeProb, true
	}

	byMinute := make(map[int]float64, len(buckets))
	for _, b := range buckets {
		byMinute[b.MinuteOfDay] = b.SpikeProb
	}
	if v, ok := byMinute[minuteOfDay]; ok {
		return v, true
	}
	for radius := 1; radius <= maxMinuteSearchRadius; radius++ {
		for _, delta := range [2]int{radius, -radius} {
			candidate := ((minuteOfDay+delta)%1440 + 1440) % 1440
			if v, ok := byMinute[candidate]; ok {
				return v, true
			}
		}
	}
	return 0, false
}

func lookupWeeklyBucketSpikeProb(buckets []vol.WeeklyBucket, minuteOfWeek int) (float64, bool) {
	const minutesPerWeek = 7 * 1440

	if minuteOfWeek >= 0 && minuteOfWeek < len(buckets) && buckets[minuteOfWeek].MinuteOfWeek == minuteOfWeek {
		return buckets[minuteOfWeek].SpikeProb, true
	}

	byMinute := make(map[int]float64, len(buckets))
	for _, b := range buckets {
		byMinute[b.MinuteOfWeek] = b.SpikeProb
	}
	if v, ok := byMinute[minuteOfWeek]; ok {
		return v, true
	}
	for radius := 1; radius <= maxMinuteSearchRadius; radius++ {
		for _, delta := range [2]int{radius, -radius} {
			candidate := ((minuteOfWeek+delta)%minutesPerWeek + minutesPerWeek) % minutesPerWeek
			if v, ok := byMinute[candidate]; ok {
				return v, true
			}
		}
	}
	return 0, false
}
