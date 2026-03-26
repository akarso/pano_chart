package market

import (
	"context"
	"math"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RankingsEvaluationProvider adapts the RankingsUseCase to the
// application-layer EvaluationProvider interface.
//
// It executes the rankings pipeline for the requested timeframe and
// converts each RankedResult into an EvaluationSnapshot using the
// component scores already computed by the ranking engine.
type RankingsEvaluationProvider struct {
	rankings usecases.RankingsUseCase
}

// NewRankingsEvaluationProvider constructs the adapter.
func NewRankingsEvaluationProvider(r usecases.RankingsUseCase) *RankingsEvaluationProvider {
	return &RankingsEvaluationProvider{rankings: r}
}

// GetLatestEvaluations implements market.EvaluationProvider.
func (p *RankingsEvaluationProvider) GetLatestEvaluations(timeframe string) ([]domain.EvaluationSnapshot, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return nil, err
	}

	results, err := p.rankings.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return nil, err
	}

	snapshots := make([]domain.EvaluationSnapshot, 0, len(results))
	for _, r := range results {
		snap := domain.EvaluationSnapshot{
			Symbol:            r.Symbol.String(),
			Timeframe:         timeframe,
			SidewaysScore:     r.Scores["Sideways Consistency"],
			TrendScore:        r.Scores["Trend Predictability"],
			CompressionScore:  r.Scores["Compression"],
			BreakoutUpScore:   r.Scores["Breakout Up"],
			BreakoutDownScore: r.Scores["Breakout Down"],
			Volume:            r.Volume,
		}
		EnrichFromSparkline(&snap, r.Sparkline)
		snapshots = append(snapshots, snap)
	}

	return snapshots, nil
}

// EnrichFromSparkline derives Bias, Price, ATR, RecentHigh, RecentLow, and
// RecentReturn from the close-price sparkline already stored on the
// RankedResult.  This allows the health computation in the market state
// service to function correctly.
func EnrichFromSparkline(snap *domain.EvaluationSnapshot, sparkline []float64) {
	n := len(sparkline)
	if n < 2 {
		return
	}

	snap.Price = sparkline[n-1]

	// Bias from overall direction.
	if sparkline[n-1] > sparkline[0] {
		snap.Bias = "up"
	} else if sparkline[n-1] < sparkline[0] {
		snap.Bias = "down"
	} else {
		snap.Bias = "neutral"
	}

	// Recent extremes and ATR proxy from sparkline.
	hi, lo := sparkline[0], sparkline[0]
	var atrSum float64
	for i := 0; i < n; i++ {
		if sparkline[i] > hi {
			hi = sparkline[i]
		}
		if sparkline[i] < lo {
			lo = sparkline[i]
		}
		if i > 0 {
			atrSum += math.Abs(sparkline[i] - sparkline[i-1])
		}
	}
	snap.RecentHigh = hi
	snap.RecentLow = lo
	snap.ATR = atrSum / float64(n-1) // average absolute candle-to-candle move

	// RecentReturn in ATR units (matches ComputeTrendHealth expectations).
	if snap.ATR > 0 {
		snap.RecentReturn = (sparkline[n-1] - sparkline[0]) / snap.ATR
	}
}
