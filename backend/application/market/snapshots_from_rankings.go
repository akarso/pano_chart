package market

import (
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// SnapshotsFromRankings converts RankedResult rows into EvaluationSnapshots,
// enriching each from its sparkline. Deduplicates by symbol (last wins).
// When computedAt is zero, ComputedAt stays 0 and Timestamp is left zero —
// used by on-demand providers that are not writing to the store.
func SnapshotsFromRankings(results []usecases.RankedResult, timeframe string, computedAt time.Time) []domain.EvaluationSnapshot {
	var atUnix int64
	if !computedAt.IsZero() {
		atUnix = computedAt.Unix()
	}

	bySym := make(map[string]domain.EvaluationSnapshot, len(results))
	order := make([]string, 0, len(results))
	for _, r := range results {
		sym := r.Symbol.String()
		spark := append([]float64(nil), r.Sparkline...)
		snap := domain.EvaluationSnapshot{
			Timestamp:         computedAt,
			Symbol:            sym,
			Timeframe:         timeframe,
			SidewaysScore:     r.Scores["Sideways Consistency"],
			TrendScore:        r.Scores["Trend Predictability"],
			CompressionScore:  r.Scores["Compression"],
			BreakoutUpScore:   r.Scores["Breakout Up"],
			BreakoutDownScore: r.Scores["Breakout Down"],
			Volume:            r.Volume,
			Sparkline:         spark,
			ComputedAt:        atUnix,
			AlgoVersion:       domain.AlgoVersion,
		}
		EnrichFromSparkline(&snap, spark)
		if _, seen := bySym[sym]; !seen {
			order = append(order, sym)
		}
		bySym[sym] = snap
	}

	out := make([]domain.EvaluationSnapshot, 0, len(order))
	for _, sym := range order {
		out = append(out, bySym[sym])
	}
	return out
}
