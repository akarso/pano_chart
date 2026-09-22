package signal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	domainsignal "pano_chart/backend/domain/signal"
)

const (
	defaultSince = 30 * 24 * time.Hour
	maxSinceDays = 3650 // ~10y; avoids Duration overflow
)

var (
	maxSinceHours   = maxSinceDays * 24
	maxSinceMinutes = maxSinceHours * 60
	maxSinceSeconds = maxSinceMinutes * 60
)

// Allowed scorecard kinds (ROADMAP vocabulary).
var allowedKinds = map[string]struct{}{
	string(domainsignal.KindBadge):      {},
	string(domainsignal.KindSetup):      {},
	string(domainsignal.KindRegime):     {},
	string(domainsignal.KindTransition): {},
}

// ErrValidation is a client input error (maps to HTTP 400).
var ErrValidation = errors.New("scorecard validation")

// Bucket is one score decile on a scorecard.
type Bucket struct {
	Lo        float64 `json:"lo"`
	Hi        float64 `json:"hi"`
	N         int     `json:"n"`
	Hits      int     `json:"hits"`
	HitRate   float64 `json:"hitRate"`
	AvgReturn float64 `json:"avgReturn"`
}

// Scorecard is the graded reliability of one (kind, label, timeframe) slice.
// Baseline is nil when there is no comparison set (sole label / empty others);
// a pointer to 0 means the comparison set graded N>0 with zero hits.
type Scorecard struct {
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Timeframe string    `json:"timeframe"`
	Since     time.Time `json:"since"`
	SinceRaw  string    `json:"sinceRaw,omitempty"`
	Total     int       `json:"total"`
	Hits      int       `json:"hits"`
	HitRate   float64   `json:"hitRate"`
	Buckets   []Bucket  `json:"buckets"`
	Baseline  *float64  `json:"baseline"`
}

// SummaryRow is one chip line for the UI.
type SummaryRow struct {
	Kind      string   `json:"kind"`
	Label     string   `json:"label"`
	HitRate   float64  `json:"hitRate"`
	Baseline  *float64 `json:"baseline"`
	N         int      `json:"n"`
	Timeframe string   `json:"timeframe,omitempty"`
}

// SummaryResult is the frozen summary payload (since matches the aggregate window).
type SummaryResult struct {
	Timeframe string       `json:"timeframe"`
	Since     time.Time    `json:"since"`
	SinceRaw  string       `json:"sinceRaw,omitempty"`
	Items     []SummaryRow `json:"items"`
}

// SinceWindow is a parsed since parameter: SQL bound and cache key agree.
type SinceWindow struct {
	CacheBucket string    // redis / singleflight key token
	Since       time.Time // lower bound for SQL (same instant as absolute CacheBucket)
	DisplayRaw  string    // client-facing sinceRaw
}

// ScorecardService aggregates resolved signals into hit-rate scorecards.
// Baseline is computed in the same aggregate statement as the deciles;
// Redis holds the whole card for 10m so hit rate and baseline share one window.
type ScorecardService struct {
	store ports.ScorecardStore
	now   func() time.Time
}

// NewScorecardService constructs the service. store must be non-nil for Get.
func NewScorecardService(store ports.ScorecardStore) *ScorecardService {
	return &ScorecardService{
		store: store,
		now:   time.Now,
	}
}

// SetNow overrides the clock (tests).
func (s *ScorecardService) SetNow(fn func() time.Time) {
	if s != nil && fn != nil {
		s.now = fn
	}
}

// Get returns the scorecard for kind/label/timeframe.
// sinceRaw is a relative token ("30d") or RFC3339; empty → "30d".
func (s *ScorecardService) Get(ctx context.Context, kind, label, tf, sinceRaw string) (Scorecard, error) {
	if s == nil || s.store == nil {
		return Scorecard{}, fmt.Errorf("scorecard: store unavailable")
	}
	kind, err := NormalizeKind(kind)
	if err != nil {
		return Scorecard{}, err
	}
	label = strings.TrimSpace(label)
	tf, err = NormalizeTimeframe(tf)
	if err != nil {
		return Scorecard{}, err
	}
	if label == "" {
		return Scorecard{}, fmt.Errorf("%w: label required", ErrValidation)
	}
	if strings.ContainsAny(label, "|\x00") {
		return Scorecard{}, fmt.Errorf("%w: invalid label", ErrValidation)
	}
	now := s.now().UTC()
	win, err := ResolveSince(sinceRaw, now)
	if err != nil {
		return Scorecard{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	agg, err := s.store.ScorecardAggregate(ctx, kind, label, tf, win.Since)
	if err != nil {
		return Scorecard{}, err
	}
	card := AggregateToScorecard(kind, label, tf, win.Since, win.DisplayRaw, agg)
	card.Baseline = BaselineRate(agg.BaselineHits, agg.BaselineTotal)
	return card, nil
}

// Summary returns one row per (kind, label) for timeframe.
// Baselines are derived from the same GROUP BY result (no per-label re-query).
func (s *ScorecardService) Summary(ctx context.Context, tf, sinceRaw string) (SummaryResult, error) {
	if s == nil || s.store == nil {
		return SummaryResult{}, fmt.Errorf("scorecard: store unavailable")
	}
	tf, err := NormalizeTimeframe(tf)
	if err != nil {
		return SummaryResult{}, err
	}
	now := s.now().UTC()
	win, err := ResolveSince(sinceRaw, now)
	if err != nil {
		return SummaryResult{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	groups, err := s.store.ScorecardSummary(ctx, tf, win.Since)
	if err != nil {
		return SummaryResult{}, err
	}

	type kindTot struct{ hits, total int }
	byKind := map[string]kindTot{}
	for _, g := range groups {
		if _, err := NormalizeKind(g.Kind); err != nil {
			continue
		}
		t := byKind[g.Kind]
		t.hits += g.Hits
		t.total += g.Total
		byKind[g.Kind] = t
	}

	items := make([]SummaryRow, 0, len(groups))
	for _, g := range groups {
		if _, err := NormalizeKind(g.Kind); err != nil {
			continue
		}
		kt := byKind[g.Kind]
		otherHits := kt.hits - g.Hits
		otherTotal := kt.total - g.Total
		hitRate := 0.0
		if g.Total > 0 {
			hitRate = float64(g.Hits) / float64(g.Total)
		}
		items = append(items, SummaryRow{
			Kind:      g.Kind,
			Label:     g.Label,
			HitRate:   hitRate,
			Baseline:  BaselineRate(otherHits, otherTotal),
			N:         g.Total,
			Timeframe: tf,
		})
	}
	return SummaryResult{
		Timeframe: tf,
		Since:     win.Since,
		SinceRaw:  win.DisplayRaw,
		Items:     items,
	}, nil
}

// BaselineRate returns nil when total==0 (no comparison set); otherwise hits/total.
func BaselineRate(hits, total int) *float64 {
	if total <= 0 {
		return nil
	}
	r := float64(hits) / float64(total)
	return &r
}

// AggregateToScorecard maps SQL aggregates onto the API scorecard shape.
func AggregateToScorecard(kind, label, tf string, since time.Time, sinceRaw string, agg ports.ScorecardAggregate) Scorecard {
	card := Scorecard{
		Kind:      kind,
		Label:     label,
		Timeframe: tf,
		Since:     since,
		SinceRaw:  sinceRaw,
		Total:     agg.Total,
		Hits:      agg.Hits,
		Buckets:   emptyDeciles(),
	}
	if agg.Total > 0 {
		card.HitRate = float64(agg.Hits) / float64(agg.Total)
	}
	for i := range card.Buckets {
		b := &card.Buckets[i]
		src := agg.Buckets[i]
		b.N = src.N
		b.Hits = src.Hits
		if src.N > 0 {
			b.HitRate = float64(src.Hits) / float64(src.N)
			b.AvgReturn = src.SumReturn / float64(src.N)
		}
	}
	return card
}

func emptyDeciles() []Bucket {
	b := make([]Bucket, 10)
	for i := 0; i < 10; i++ {
		b[i] = Bucket{Lo: float64(i) / 10, Hi: float64(i+1) / 10}
	}
	return b
}

// DecileIndex maps a score into 0..9. NaN and non-finite → 0.
func DecileIndex(score float64) int {
	if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 {
		return 0
	}
	if score >= 1 {
		return 9
	}
	i := int(score * 10)
	if i > 9 {
		return 9
	}
	if i < 0 {
		return 0
	}
	return i
}

// NormalizeKind allowlists scorecard kinds.
func NormalizeKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if _, ok := allowedKinds[kind]; !ok {
		return "", fmt.Errorf("%w: unsupported kind %q", ErrValidation, kind)
	}
	return kind, nil
}

// NormalizeTimeframe returns the canonical TF string, or "" for “all timeframes”.
func NormalizeTimeframe(tf string) (string, error) {
	tf = strings.TrimSpace(tf)
	if tf == "" {
		return "", nil
	}
	canon, err := domain.NewTimeframe(tf)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return canon.String(), nil
}

// ResolveSince parses sinceRaw into a SQL window and a matching cache bucket.
// Relative forms keep the token as the cache key (every caller means the same
// duration). Absolute RFC3339 uses the exact UTC instant for both SQL and key.
func ResolveSince(raw string, now time.Time) (SinceWindow, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "30d"
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		exact := t.UTC()
		token := "abs:" + exact.Format(time.RFC3339Nano)
		return SinceWindow{
			CacheBucket: token,
			Since:       exact,
			DisplayRaw:  exact.Format(time.RFC3339Nano),
		}, nil
	}
	d, token, err := ParseSinceDurationToken(raw)
	if err != nil {
		return SinceWindow{}, err
	}
	return SinceWindow{
		CacheBucket: token,
		Since:       now.UTC().Add(-d),
		DisplayRaw:  token,
	}, nil
}

// ParseSinceDuration parses values like "30d", "7d", "24h", "1h". Empty → 30d.
func ParseSinceDuration(s string) (time.Duration, error) {
	d, _, err := ParseSinceDurationToken(s)
	return d, err
}

// ParseSinceDurationToken also returns the normalized token used in cache keys.
func ParseSinceDurationToken(s string) (time.Duration, string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return defaultSince, "30d", nil
	}
	if strings.HasSuffix(s, "d") {
		num := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(num)
		if err != nil || days <= 0 || days > maxSinceDays || num != strconv.Itoa(days) {
			return 0, "", fmt.Errorf("invalid since %q", s)
		}
		token := strconv.Itoa(days) + "d"
		return time.Duration(days) * 24 * time.Hour, token, nil
	}
	for _, u := range []struct {
		suf string
		d   time.Duration
		max int
	}{
		{"h", time.Hour, maxSinceHours},
		{"m", time.Minute, maxSinceMinutes},
		{"s", time.Second, maxSinceSeconds},
	} {
		if !strings.HasSuffix(s, u.suf) {
			continue
		}
		num := strings.TrimSuffix(s, u.suf)
		n, err := strconv.Atoi(num)
		if err != nil || n <= 0 || n > u.max || num != strconv.Itoa(n) {
			return 0, "", fmt.Errorf("invalid since %q", s)
		}
		token := strconv.Itoa(n) + u.suf
		return time.Duration(n) * u.d, token, nil
	}
	return 0, "", fmt.Errorf("invalid since %q", s)
}
