package signal_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"pano_chart/backend/application/ports"
	appsignal "pano_chart/backend/application/signal"
	domainsignal "pano_chart/backend/domain/signal"
	infrasignal "pano_chart/backend/infrastructure/signal"
)

type memScorecardStore struct {
	rows []gradable
}

type gradable struct {
	kind, label, tf string
	score           float64
	success         bool
	fwd             float64
	rule            string
	emitted         time.Time
}

func (m *memScorecardStore) ScorecardAggregate(_ context.Context, kind, label, tf string, since time.Time) (ports.ScorecardAggregate, error) {
	var out ports.ScorecardAggregate
	for _, r := range m.rows {
		if r.kind != kind {
			continue
		}
		if tf != "" && r.tf != tf {
			continue
		}
		if !since.IsZero() && r.emitted.Before(since) {
			continue
		}
		if domainsignal.ExcludedFromHitRate(r.rule) {
			continue
		}
		if r.label == label {
			i := appsignal.DecileIndex(r.score)
			out.Buckets[i].N++
			out.Buckets[i].SumReturn += r.fwd
			out.Total++
			if r.success {
				out.Buckets[i].Hits++
				out.Hits++
			}
			continue
		}
		out.BaselineTotal++
		if r.success {
			out.BaselineHits++
		}
	}
	return out, nil
}

func (m *memScorecardStore) ScorecardSummary(_ context.Context, tf string, since time.Time) ([]ports.ScorecardGroup, error) {
	type key struct{ k, l string }
	acc := map[key]*ports.ScorecardGroup{}
	for _, r := range m.rows {
		if tf != "" && r.tf != tf {
			continue
		}
		if !since.IsZero() && r.emitted.Before(since) {
			continue
		}
		if domainsignal.ExcludedFromHitRate(r.rule) {
			continue
		}
		k := key{r.kind, r.label}
		g, ok := acc[k]
		if !ok {
			g = &ports.ScorecardGroup{Kind: r.kind, Label: r.label}
			acc[k] = g
		}
		g.Total++
		if r.success {
			g.Hits++
		}
	}
	out := make([]ports.ScorecardGroup, 0, len(acc))
	for _, g := range acc {
		out = append(out, *g)
	}
	return out, nil
}

func TestScorecardService_baselineExcludesLabel(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	since := now.Add(-30 * 24 * time.Hour)
	store := &memScorecardStore{}
	for i := 0; i < 20; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "trend_up", tf: "1h", score: 0.8,
			success: true, rule: domainsignal.RuleTrendUp, emitted: since.Add(time.Hour),
		})
	}
	for i := 0; i < 20; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "sideways", tf: "1h", score: 0.5,
			success: i < 10, rule: domainsignal.RuleRangeStay, emitted: since.Add(time.Hour),
		})
	}
	store.rows = append(store.rows, gradable{
		kind: "badge", label: "gain", tf: "1h", score: 0.9,
		success: false, rule: domainsignal.RuleUnsupported, emitted: since.Add(time.Hour),
	})

	svc := appsignal.NewScorecardService(store)
	svc.SetNow(func() time.Time { return now })
	card, err := svc.Get(context.Background(), "badge", "trend_up", "1h", "30d")
	if err != nil {
		t.Fatal(err)
	}
	if card.Total != 20 || card.Hits != 20 || card.HitRate != 1 {
		t.Fatalf("card=%+v", card)
	}
	if card.Baseline == nil || *card.Baseline != 0.5 {
		t.Fatalf("baseline want 0.5, got %v", card.Baseline)
	}
}

func TestScorecardService_soleLabelBaselineNil(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	store := &memScorecardStore{}
	for i := 0; i < 5; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "trend_up", tf: "1h", score: 0.8,
			success: true, rule: domainsignal.RuleTrendUp, emitted: since.Add(time.Hour),
		})
	}
	svc := appsignal.NewScorecardService(store)
	svc.SetNow(func() time.Time { return now })
	card, err := svc.Get(context.Background(), "badge", "trend_up", "1h", "1d")
	if err != nil {
		t.Fatal(err)
	}
	if card.Baseline != nil {
		t.Fatalf("sole label must not report baseline 0, got %v", *card.Baseline)
	}
}

func TestScorecardService_summaryTwoLabelsSameKind(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	store := &memScorecardStore{}
	for i := 0; i < 20; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "trend_up", tf: "1h", score: 0.8,
			success: true, rule: domainsignal.RuleTrendUp, emitted: since.Add(time.Hour),
		})
	}
	for i := 0; i < 20; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "sideways", tf: "1h", score: 0.5,
			success: i < 10, rule: domainsignal.RuleRangeStay, emitted: since.Add(time.Hour),
		})
	}
	svc := appsignal.NewScorecardService(store)
	svc.SetNow(func() time.Time { return now })
	sum, err := svc.Summary(context.Background(), "1h", "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Items) != 2 {
		t.Fatalf("summary=%+v", sum.Items)
	}
	byLabel := map[string]appsignal.SummaryRow{}
	for _, r := range sum.Items {
		byLabel[r.Label] = r
	}
	// trend_up baseline = sideways rate 10/20 = 0.5
	if byLabel["trend_up"].Baseline == nil || *byLabel["trend_up"].Baseline != 0.5 {
		t.Fatalf("trend_up baseline=%v", byLabel["trend_up"].Baseline)
	}
	// sideways baseline = trend_up rate 20/20 = 1
	if byLabel["sideways"].Baseline == nil || *byLabel["sideways"].Baseline != 1 {
		t.Fatalf("sideways baseline=%v", byLabel["sideways"].Baseline)
	}
}

func TestScorecardService_summaryTwoKinds(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	store := &memScorecardStore{}
	for i := 0; i < 5; i++ {
		store.rows = append(store.rows, gradable{
			kind: "badge", label: "trend_up", tf: "1h", score: 0.8,
			success: true, rule: domainsignal.RuleTrendUp, emitted: since.Add(time.Hour),
		})
		store.rows = append(store.rows, gradable{
			kind: "setup", label: "compression", tf: "1h", score: 0.7,
			success: i%2 == 0, rule: domainsignal.RuleCompressionExpand, emitted: since.Add(time.Hour),
		})
	}
	svc := appsignal.NewScorecardService(store)
	svc.SetNow(func() time.Time { return now })
	sum, err := svc.Summary(context.Background(), "1H", "1d")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Timeframe != "1h" {
		t.Fatalf("canonical tf=%q", sum.Timeframe)
	}
	if len(sum.Items) != 2 {
		t.Fatalf("summary=%+v", sum)
	}
}

func TestSQLite_SummaryBaselinesTwoLabels(t *testing.T) {
	dbPath := t.TempDir() + "/sum.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	emitted := now.Add(-time.Hour)

	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("up-%d", i)
		if err := repo.Append(ctx, domainsignal.Signal{
			ID: id, Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
			Score: 0.8, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkResolved(ctx, id, domainsignal.Outcome{
			SignalID: id, ResolvedAt: now, Success: true, Rule: domainsignal.RuleTrendUp,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("side-%d", i)
		if err := repo.Append(ctx, domainsignal.Signal{
			ID: id, Kind: domainsignal.KindBadge, Label: "sideways", Timeframe: "1h",
			Score: 0.5, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkResolved(ctx, id, domainsignal.Outcome{
			SignalID: id, ResolvedAt: now, Success: i < 10, Rule: domainsignal.RuleRangeStay,
		}); err != nil {
			t.Fatal(err)
		}
	}

	svc := appsignal.NewScorecardService(repo)
	svc.SetNow(func() time.Time { return now })
	sum, err := svc.Summary(ctx, "1h", "1d")
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]appsignal.SummaryRow{}
	for _, r := range sum.Items {
		byLabel[r.Label] = r
	}
	if byLabel["trend_up"].Baseline == nil || *byLabel["trend_up"].Baseline != 0.5 {
		t.Fatalf("trend_up baseline=%v items=%+v", byLabel["trend_up"].Baseline, sum.Items)
	}
	if byLabel["sideways"].Baseline == nil || *byLabel["sideways"].Baseline != 1 {
		t.Fatalf("sideways baseline=%v", byLabel["sideways"].Baseline)
	}
}

func TestSQLite_ScorecardIgnoresUnresolvedAndAdmin(t *testing.T) {
	dbPath := t.TempDir() + "/sc.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	emitted := now.Add(-time.Hour)

	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("u-%d", i)
		if err := repo.Append(ctx, domainsignal.Signal{
			ID: id, Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
			Score: 0.9, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Append(ctx, domainsignal.Signal{
		ID: "graded", Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
		Score: 0.85, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkResolved(ctx, "graded", domainsignal.Outcome{
		SignalID: "graded", ResolvedAt: now, Success: true, Rule: domainsignal.RuleTrendUp, ForwardReturn: 0.01,
	}); err != nil {
		t.Fatal(err)
	}
	for i, rule := range []string{
		domainsignal.RuleUnsupported,
		domainsignal.RuleInvalid,
		domainsignal.RuleInsufficientContext,
		domainsignal.RulePathUnavailable,
	} {
		id := fmt.Sprintf("admin-%d", i)
		if err := repo.Append(ctx, domainsignal.Signal{
			ID: id, Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
			Score: 0.5, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkResolved(ctx, id, domainsignal.Outcome{
			SignalID: id, ResolvedAt: now, Success: false, Rule: rule,
		}); err != nil {
			t.Fatal(err)
		}
	}

	agg, err := repo.ScorecardAggregate(ctx, "badge", "trend_up", "1h", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if agg.Total != 1 || agg.Hits != 1 {
		t.Fatalf("agg=%+v (unresolved/admin must not affect)", agg)
	}
	if agg.BaselineTotal != 0 {
		t.Fatalf("baseline=%d/%d", agg.BaselineHits, agg.BaselineTotal)
	}
}

func TestSQLite_ScorecardDecileBoundaries(t *testing.T) {
	dbPath := t.TempDir() + "/decile.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	emitted := now.Add(-time.Hour)

	cases := []struct {
		id    string
		score float64
	}{
		{"s005", 0.05},
		{"s095", 0.95},
		{"s100", 1.0},
		{"sNeg", -0.2},
	}
	for _, c := range cases {
		if err := repo.Append(ctx, domainsignal.Signal{
			ID: c.id, Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
			Score: c.score, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.MarkResolved(ctx, c.id, domainsignal.Outcome{
			SignalID: c.id, ResolvedAt: now, Success: true, Rule: domainsignal.RuleTrendUp,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Other label contributes only to baseline.
	if err := repo.Append(ctx, domainsignal.Signal{
		ID: "other", Kind: domainsignal.KindBadge, Label: "sideways", Timeframe: "1h",
		Score: 0.5, Price: 1, ATR: 1, EmittedAt: emitted, HorizonBars: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkResolved(ctx, "other", domainsignal.Outcome{
		SignalID: "other", ResolvedAt: now, Success: false, Rule: domainsignal.RuleRangeStay,
	}); err != nil {
		t.Fatal(err)
	}

	agg, err := repo.ScorecardAggregate(ctx, "badge", "trend_up", "1h", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if agg.Buckets[0].N != 2 || agg.Buckets[9].N != 2 || agg.Total != 4 {
		t.Fatalf("%+v", agg)
	}
	if agg.BaselineTotal != 1 || agg.BaselineHits != 0 {
		t.Fatalf("baseline=%d/%d", agg.BaselineHits, agg.BaselineTotal)
	}
}

func TestSQLite_AbsoluteSinceWindowsDoNotShareTotals(t *testing.T) {
	dbPath := t.TempDir() + "/abs.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()

	mid := time.Date(2026, 9, 21, 0, 5, 0, 0, time.UTC)
	resolved := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := repo.Append(ctx, domainsignal.Signal{
		ID: "mid", Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
		Score: 0.8, Price: 1, ATR: 1, EmittedAt: mid, HorizonBars: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkResolved(ctx, "mid", domainsignal.Outcome{
		SignalID: "mid", ResolvedAt: resolved, Success: true, Rule: domainsignal.RuleTrendUp,
	}); err != nil {
		t.Fatal(err)
	}

	svc := appsignal.NewScorecardService(repo)
	card01, err := svc.Get(ctx, "badge", "trend_up", "1h", "2026-09-21T00:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	card09, err := svc.Get(ctx, "badge", "trend_up", "1h", "2026-09-21T00:09:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if card01.Total != 1 {
		t.Fatalf("since=00:01 must include 00:05 row, total=%d", card01.Total)
	}
	if card09.Total != 0 {
		t.Fatalf("since=00:09 must exclude 00:05 row, total=%d", card09.Total)
	}
}

func TestRedis_AbsoluteSinceKeysDoNotShareCard(t *testing.T) {
	dbPath := t.TempDir() + "/abs-redis.sqlite"
	repo, err := infrasignal.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	mid := time.Date(2026, 9, 21, 0, 5, 0, 0, time.UTC)
	resolved := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := repo.Append(ctx, domainsignal.Signal{
		ID: "mid", Kind: domainsignal.KindBadge, Label: "trend_up", Timeframe: "1h",
		Score: 0.8, Price: 1, ATR: 1, EmittedAt: mid, HorizonBars: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkResolved(ctx, "mid", domainsignal.Outcome{
		SignalID: "mid", ResolvedAt: resolved, Success: true, Rule: domainsignal.RuleTrendUp,
	}); err != nil {
		t.Fatal(err)
	}

	svc := appsignal.NewScorecardService(repo)
	mem := &recordingRedis{data: map[string]string{}}
	cache := infrasignal.NewRedisCachedScorecard(svc, mem, "scorecards")

	c1, err := cache.Get(ctx, "badge", "trend_up", "1h", "2026-09-21T00:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := cache.Get(ctx, "badge", "trend_up", "1h", "2026-09-21T00:09:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if c1.Total == c2.Total {
		t.Fatalf("shared totals %d across absolute windows (keys=%v)", c1.Total, mem.keys())
	}
	if c1.Total != 1 || c2.Total != 0 {
		t.Fatalf("c1=%d c2=%d", c1.Total, c2.Total)
	}
	if len(mem.data) != 2 {
		t.Fatalf("want 2 redis keys, got %d: %v", len(mem.data), mem.keys())
	}
}

type recordingRedis struct {
	data map[string]string
}

func (r *recordingRedis) Get(_ context.Context, key string) (string, error) {
	v, ok := r.data[key]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (r *recordingRedis) keys() []string {
	out := make([]string, 0, len(r.data))
	for k := range r.data {
		out = append(out, k)
	}
	return out
}

func (r *recordingRedis) Set(_ context.Context, key, value string, _ time.Duration) error {
	if r.data == nil {
		r.data = map[string]string{}
	}
	r.data[key] = value
	return nil
}
