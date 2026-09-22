package signal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pano_chart/backend/application/ports"
	domainsignal "pano_chart/backend/domain/signal"
)

const scoreBucketExpr = `
	CASE
		WHEN s.score != s.score THEN 0
		WHEN s.score <= 0 THEN 0
		WHEN s.score >= 1 THEN 9
		ELSE CAST(s.score * 10 AS INTEGER)
	END`

func excludedRulesSQL() (clause string, args []interface{}) {
	rules := domainsignal.ExcludedHitRateRules()
	ph := make([]string, len(rules))
	args = make([]interface{}, len(rules))
	for i, r := range rules {
		ph[i] = "?"
		args[i] = r
	}
	return "o.rule NOT IN (" + strings.Join(ph, ",") + ")", args
}

// ScorecardAggregate implements ports.ScorecardStore.
// One statement: target-label deciles (bucket 0..9) and other-label baseline (bucket -1).
func (r *SQLiteRepository) ScorecardAggregate(
	ctx context.Context, kind, label, tf string, since time.Time,
) (ports.ScorecardAggregate, error) {
	var out ports.ScorecardAggregate
	excl, exclArgs := excludedRulesSQL()
	// label twice: is-target CASE + forward_return CASE
	args := []interface{}{label, label, kind}
	conds := []string{"s.kind = ?", excl}
	args = append(args, exclArgs...)
	if tf != "" {
		conds = append(conds, "s.timeframe = ?")
		args = append(args, tf)
	}
	if !since.IsZero() {
		conds = append(conds, "s.emitted_at >= ?")
		args = append(args, since.UTC().UnixNano())
	}
	q := fmt.Sprintf(`
		SELECT
			CASE WHEN s.label = ? THEN (%s) ELSE -1 END AS bucket,
			COUNT(*),
			SUM(CASE WHEN o.success != 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN s.label = ? THEN o.forward_return ELSE 0 END)
		FROM signals s
		INNER JOIN outcomes o ON o.signal_id = s.id
		WHERE %s
		GROUP BY bucket`, scoreBucketExpr, strings.Join(conds, " AND "))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return out, fmt.Errorf("scorecard aggregate: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bucket, n, hits int
		var sumRet float64
		if err := rows.Scan(&bucket, &n, &hits, &sumRet); err != nil {
			return out, fmt.Errorf("scorecard aggregate scan: %w", err)
		}
		if bucket < 0 {
			out.BaselineTotal += n
			out.BaselineHits += hits
			continue
		}
		if bucket > 9 {
			bucket = 9
		}
		out.Buckets[bucket].N += n
		out.Buckets[bucket].Hits += hits
		out.Buckets[bucket].SumReturn += sumRet
		out.Total += n
		out.Hits += hits
	}
	return out, rows.Err()
}

// ScorecardSummary implements ports.ScorecardStore.
func (r *SQLiteRepository) ScorecardSummary(
	ctx context.Context, tf string, since time.Time,
) ([]ports.ScorecardGroup, error) {
	excl, exclArgs := excludedRulesSQL()
	args := append([]interface{}{}, exclArgs...)
	conds := []string{excl}
	if tf != "" {
		conds = append(conds, "s.timeframe = ?")
		args = append(args, tf)
	}
	if !since.IsZero() {
		conds = append(conds, "s.emitted_at >= ?")
		args = append(args, since.UTC().UnixNano())
	}
	q := fmt.Sprintf(`
		SELECT s.kind, s.label, COUNT(*),
		       SUM(CASE WHEN o.success != 0 THEN 1 ELSE 0 END)
		FROM signals s
		INNER JOIN outcomes o ON o.signal_id = s.id
		WHERE %s
		GROUP BY s.kind, s.label
		ORDER BY s.kind ASC, s.label ASC`, strings.Join(conds, " AND "))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("scorecard summary: %w", err)
	}
	defer rows.Close()
	var out []ports.ScorecardGroup
	for rows.Next() {
		var g ports.ScorecardGroup
		if err := rows.Scan(&g.Kind, &g.Label, &g.Total, &g.Hits); err != nil {
			return nil, fmt.Errorf("scorecard summary scan: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
