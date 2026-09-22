package signal

import (
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
)

func TestAggregateToScorecard_deciles(t *testing.T) {
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	var agg ports.ScorecardAggregate
	agg.Buckets[0] = ports.ScorecardBucketAgg{N: 10, Hits: 0, SumReturn: -0.1}
	agg.Buckets[9] = ports.ScorecardBucketAgg{N: 10, Hits: 10, SumReturn: 0.2}
	agg.Total = 20
	agg.Hits = 10
	card := AggregateToScorecard("badge", "trend_up", "1h", since, "30d", agg)
	if card.Total != 20 || card.Hits != 10 || math.Abs(card.HitRate-0.5) > 1e-9 {
		t.Fatalf("%+v", card)
	}
	if card.Buckets[0].N != 10 || card.Buckets[9].Hits != 10 {
		t.Fatalf("buckets=%+v", card.Buckets)
	}
	if math.Abs(card.Buckets[9].AvgReturn-0.02) > 1e-9 {
		t.Fatalf("avg=%v", card.Buckets[9].AvgReturn)
	}
	if card.SinceRaw != "30d" {
		t.Fatalf("sinceRaw=%q", card.SinceRaw)
	}
}

func TestDecileIndex(t *testing.T) {
	if DecileIndex(math.NaN()) != 0 {
		t.Fatal("nan")
	}
	if DecileIndex(-1) != 0 || DecileIndex(0) != 0 {
		t.Fatal("low")
	}
	if DecileIndex(1) != 9 || DecileIndex(1.5) != 9 {
		t.Fatal("high")
	}
	if DecileIndex(0.95) != 9 {
		t.Fatal("0.95")
	}
	if DecileIndex(0.15) != 1 {
		t.Fatal("0.15")
	}
}

func TestParseSinceDuration_strict(t *testing.T) {
	d, err := ParseSinceDuration("30d")
	if err != nil || d != 30*24*time.Hour {
		t.Fatalf("%v %v", d, err)
	}
	if _, err := ParseSinceDuration("30dgarbage"); err == nil {
		t.Fatal("expected reject trailing garbage")
	}
	if _, err := ParseSinceDuration("99999d"); err == nil {
		t.Fatal("expected reject huge days")
	}
	if _, err := ParseSinceDuration("3000000h"); err == nil {
		t.Fatal("expected reject huge hours")
	}
	win, err := ResolveSince("30d", time.Date(2026, 9, 21, 12, 0, 5, 0, time.UTC))
	if err != nil || win.CacheBucket != "30d" {
		t.Fatalf("%+v %v", win, err)
	}
	win2, err := ResolveSince("30d", time.Date(2026, 9, 21, 12, 0, 6, 0, time.UTC))
	if err != nil || win2.CacheBucket != win.CacheBucket {
		t.Fatal("relative since bucket must be stable across seconds")
	}
}

func TestResolveSince_absoluteExactKeyMatchesSQL(t *testing.T) {
	raw := "2026-09-21T00:09:00Z"
	win, err := ResolveSince(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 21, 0, 9, 0, 0, time.UTC)
	if !win.Since.Equal(want) {
		t.Fatalf("sql since=%v", win.Since)
	}
	if win.CacheBucket != "abs:"+want.Format(time.RFC3339Nano) {
		t.Fatalf("cache bucket=%q (must include exact instant)", win.CacheBucket)
	}
	winEarly, err := ResolveSince("2026-09-21T00:01:00Z", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if winEarly.CacheBucket == win.CacheBucket {
		t.Fatal("00:01 and 00:09 must not share a cache key")
	}
	frac, err := ResolveSince("2026-09-21T00:09:00.5Z", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !frac.Since.Equal(time.Date(2026, 9, 21, 0, 9, 0, 500000000, time.UTC)) {
		t.Fatalf("fractional since=%v", frac.Since)
	}
}

func TestBaselineRate_nilVsZero(t *testing.T) {
	if BaselineRate(0, 0) != nil {
		t.Fatal("empty comparison must be nil")
	}
	p := BaselineRate(0, 10)
	if p == nil || *p != 0 {
		t.Fatalf("zero rate with N>0: %v", p)
	}
	p = BaselineRate(5, 10)
	if p == nil || *p != 0.5 {
		t.Fatalf("%v", p)
	}
}

func TestNormalizeKind(t *testing.T) {
	k, err := NormalizeKind("Badge")
	if err != nil || k != "badge" {
		t.Fatalf("%q %v", k, err)
	}
	if _, err := NormalizeKind("other"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestNormalizeTimeframe(t *testing.T) {
	tf, err := NormalizeTimeframe("1H")
	if err != nil || tf != "1h" {
		t.Fatalf("%q %v", tf, err)
	}
	tf, err = NormalizeTimeframe("")
	if err != nil || tf != "" {
		t.Fatalf("empty: %q %v", tf, err)
	}
	if _, err := NormalizeTimeframe("99z"); err == nil {
		t.Fatal("expected reject")
	}
}
