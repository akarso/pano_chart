package market_test

import (
	"testing"

	"pano_chart/backend/application/market/metrics"
)

func TestSectorProfilePath_MatchesWriterReaderConvention(t *testing.T) {
	path, err := metrics.SectorProfilePath("/data/vol", "L1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/data/vol_l1.json" {
		t.Fatalf("path=%q want /data/vol_l1.json", path)
	}
	again, err := metrics.SectorProfilePath("/data/vol", "l1")
	if err != nil || again != path {
		t.Fatalf("case-normalized round-trip failed: %v %q", err, again)
	}
}

func TestNormalizeSectorID_RejectsPathTraversal(t *testing.T) {
	for _, id := range []string{"../x", "a/b", "a\\b", "L1!", ""} {
		if _, err := metrics.NormalizeSectorID(id); err == nil {
			t.Fatalf("expected error for %q", id)
		}
	}
}
