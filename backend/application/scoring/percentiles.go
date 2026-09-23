package scoring

import (
	"context"
	"errors"
	"math"
	"sort"

	"pano_chart/backend/application/ports"
)

// ErrNoSamples is returned when a calculator/timeframe has no retained scores.
var ErrNoSamples = errors.New("no score samples")

// Percentile is the fraction of retained samples less than or equal to score.
func Percentile(samples []float64, score float64) (float64, error) {
	if len(samples) == 0 {
		return 0, ErrNoSamples
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	n := sort.Search(len(sorted), func(i int) bool { return sorted[i] > score })
	return float64(n) / float64(len(sorted)), nil
}

// Distribution is the nearest-rank values at p5, p10, … p95.
func Distribution(samples []float64) ([]float64, error) {
	if len(samples) == 0 {
		return nil, ErrNoSamples
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	out := make([]float64, 0, 19)
	for p := 5; p <= 95; p += 5 {
		out = append(out, nearestRank(sorted, p))
	}
	return out, nil
}

func nearestRank(sorted []float64, percentile int) float64 {
	n := len(sorted)
	rank := int(math.Ceil(float64(percentile)/100*float64(n))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= n {
		rank = n - 1
	}
	return sorted[rank]
}

// DistributionService answers percentile queries from a sample reader.
type DistributionService struct {
	reader ports.ScoreSampleReader
}

// NewDistributionService constructs the query service.
func NewDistributionService(reader ports.ScoreSampleReader) *DistributionService {
	return &DistributionService{reader: reader}
}

// Percentile is the empirical CDF of score for one calculator and timeframe.
func (s *DistributionService) Percentile(ctx context.Context, calculator, tf string, score float64) (float64, error) {
	samples, err := s.reader.Scores(ctx, calculator, tf)
	if err != nil {
		return 0, err
	}
	return Percentile(samples, score)
}

// Distribution returns p5..p95 for one calculator and timeframe.
func (s *DistributionService) Distribution(ctx context.Context, calculator, tf string) ([]float64, error) {
	samples, err := s.reader.Scores(ctx, calculator, tf)
	if err != nil {
		return nil, err
	}
	return Distribution(samples)
}

// Calculators lists calculators that still have retained samples.
func (s *DistributionService) Calculators(ctx context.Context) ([]string, error) {
	return s.reader.Calculators(ctx)
}
