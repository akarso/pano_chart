package scoring_test

import (
	"context"
	"errors"
	"testing"

	appscoring "pano_chart/backend/application/scoring"
)

func TestPercentile_KnownSampleSet(t *testing.T) {
	samples := []float64{10, 20, 30, 40, 50}

	cases := []struct {
		score float64
		want  float64
	}{
		{30, 0.6},
		{25, 0.4},
		{5, 0},
		{50, 1},
	}
	for _, tc := range cases {
		got, err := appscoring.Percentile(samples, tc.score)
		if err != nil {
			t.Fatalf("score %v: %v", tc.score, err)
		}
		if got != tc.want {
			t.Fatalf("Percentile(%v)=%v, want %v", tc.score, got, tc.want)
		}
	}
}

func TestPercentile_Empty(t *testing.T) {
	_, err := appscoring.Percentile(nil, 1)
	if !errors.Is(err, appscoring.ErrNoSamples) {
		t.Fatalf("err=%v", err)
	}
}

func TestDistribution_NearestRank(t *testing.T) {
	samples := make([]float64, 20)
	for i := range samples {
		samples[i] = float64(i + 1)
	}
	got, err := appscoring.Distribution(samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 19 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0] != 1 || got[9] != 10 || got[18] != 19 {
		t.Fatalf("p5=%v p50=%v p95=%v", got[0], got[9], got[18])
	}
}

func TestDistribution_DoesNotMutateInput(t *testing.T) {
	samples := []float64{3, 1, 2}
	if _, err := appscoring.Distribution(samples); err != nil {
		t.Fatal(err)
	}
	if samples[0] != 3 || samples[1] != 1 || samples[2] != 2 {
		t.Fatalf("input mutated: %v", samples)
	}
}

type fakeReader struct {
	scores []float64
	names  []string
	err    error
}

func (f fakeReader) Scores(context.Context, string, string) ([]float64, error) {
	return f.scores, f.err
}

func (f fakeReader) Calculators(context.Context) ([]string, error) {
	return f.names, nil
}

func TestDistributionService_PercentileAndDistribution(t *testing.T) {
	svc := appscoring.NewDistributionService(fakeReader{scores: []float64{10, 20, 30, 40, 50}})
	got, err := svc.Percentile(context.Background(), "sideways", "1h", 30)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.6 {
		t.Fatalf("percentile=%v", got)
	}
	dist, err := svc.Distribution(context.Background(), "sideways", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(dist) != 19 {
		t.Fatalf("len=%d", len(dist))
	}
}

func TestDistributionService_PropagatesReaderError(t *testing.T) {
	boom := errors.New("db")
	svc := appscoring.NewDistributionService(fakeReader{err: boom})
	_, err := svc.Percentile(context.Background(), "sideways", "1h", 1)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}
