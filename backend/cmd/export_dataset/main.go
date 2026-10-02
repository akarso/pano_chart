// Command export_dataset writes a CSV of resolved setup signals for offline
// regime-model training (PR-109).
//
// regime_label is the four-way raw-score argmax stored as regime_code at
// emission — independent of scoring.regime_model (heuristic or learned).
// success is the separate outcome grade.
//
// Skips: unresolved rows; ExcludedFromHitRate outcome rules; rows missing
// regime_code or any DefaultRegimeFeatures key (pre-PR-109 / incomplete).
//
// Rows are loaded in pages (bounded memory). -max-rows caps eligible written
// rows; the scanner stops once that many eligible rows are written.
//
//	export_dataset -db ./signals.sqlite -out ./dataset.csv
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"pano_chart/backend/domain/scoring"
	domainsignal "pano_chart/backend/domain/signal"
	infrasignal "pano_chart/backend/infrastructure/signal"
)

const exportPageSize = 1000

func main() {
	dbPath := flag.String("db", envOr("PC_SIGNAL_DB", "./signals.sqlite"), "signals sqlite path")
	outPath := flag.String("out", "dataset.csv", "output CSV path")
	kind := flag.String("kind", "setup", "signal kind filter (default setup; empty = all kinds — incomplete features)")
	sinceDays := flag.Int("since-days", 0, "only signals emitted in the last N days (0 = all)")
	maxRows := flag.Int("max-rows", 0, "cap eligible written rows (0 = unlimited); DB is scanned in pages")
	flag.Parse()

	if *kind == "" {
		fmt.Fprintf(os.Stderr, "warning: -kind empty exports all kinds; transition/regime/badge rows often lack full feature keys — prefer -kind setup\n")
	}

	repo, err := infrasignal.NewSQLiteRepository(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = repo.Close() }()

	base := domainsignal.Filter{OldestFirst: true}
	if *kind != "" {
		base.Kind = domainsignal.Kind(*kind)
	}
	if *sinceDays > 0 {
		base.Since = time.Now().UTC().Add(-time.Duration(*sinceDays) * 24 * time.Hour)
	}

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create out: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	header := []string{
		"id", "kind", "symbol", "timeframe", "label", "score",
		"price", "atr", "emitted_at",
	}
	header = append(header, scoring.DefaultRegimeFeatures...)
	header = append(header, "regime_label", "success", "forward_return", "rule")
	if err := w.Write(header); err != nil {
		fmt.Fprintf(os.Stderr, "write header: %v\n", err)
		os.Exit(1)
	}

	scanned := 0
	written := 0
	skippedNoOutcome := 0
	skippedExcluded := 0
	skippedIncomplete := 0
	offset := 0

	for {
		filter := base
		filter.Limit = exportPageSize
		filter.Offset = offset
		rows, err := repo.Query(context.Background(), filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "query: %v\n", err)
			os.Exit(1)
		}
		if len(rows) == 0 {
			break
		}
		scanned += len(rows)
		stop := false
		for _, row := range rows {
			if row.Outcome == nil {
				skippedNoOutcome++
				continue
			}
			if domainsignal.ExcludedFromHitRate(row.Outcome.Rule) {
				skippedExcluded++
				continue
			}
			if !hasTrainingFeatures(row.Signal.Context) {
				skippedIncomplete++
				continue
			}
			if *maxRows > 0 && written >= *maxRows {
				stop = true
				break
			}
			if err := w.Write(recordFor(row)); err != nil {
				fmt.Fprintf(os.Stderr, "write row: %v\n", err)
				os.Exit(1)
			}
			written++
		}
		if stop || len(rows) < exportPageSize {
			break
		}
		offset += len(rows)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "flush: %v\n", err)
		os.Exit(1)
	}
	unlimited := *maxRows <= 0
	fmt.Printf("scanned=%d resolved_written=%d skipped_no_outcome=%d skipped_excluded=%d skipped_incomplete=%d out=%s kind=%q max_rows=%d unlimited=%v page_size=%d oldest_first=true\n",
		scanned, written, skippedNoOutcome, skippedExcluded, skippedIncomplete, *outPath, *kind, *maxRows, unlimited, exportPageSize)
}

// hasTrainingFeatures requires regime_code and every DefaultRegimeFeatures key.
func hasTrainingFeatures(ctx map[string]float64) bool {
	if ctx == nil {
		return false
	}
	if _, ok := ctx["regime_code"]; !ok {
		return false
	}
	for _, k := range scoring.DefaultRegimeFeatures {
		if _, ok := ctx[k]; !ok {
			return false
		}
	}
	return true
}

func recordFor(row domainsignal.SignalWithOutcome) []string {
	sig := row.Signal
	oc := row.Outcome
	ctx := sig.Context
	feat := func(k string) string {
		return fmtFloat(ctx[k])
	}
	success := "0"
	if oc.Success {
		success = "1"
	}
	out := []string{
		sig.ID,
		string(sig.Kind),
		sig.Symbol,
		sig.Timeframe,
		sig.Label,
		fmtFloat(sig.Score),
		fmtFloat(sig.Price),
		fmtFloat(sig.ATR),
		sig.EmittedAt.UTC().Format(time.RFC3339Nano),
	}
	for _, k := range scoring.DefaultRegimeFeatures {
		out = append(out, feat(k))
	}
	out = append(out, decodeRegimeCode(ctx["regime_code"]), success, fmtFloat(oc.ForwardReturn), oc.Rule)
	return out
}

func decodeRegimeCode(code float64) string {
	switch int(code + 0.5) {
	case 1:
		return "trend"
	case 2:
		return "compression"
	case 3:
		return "expansion"
	default:
		return "sideways"
	}
}

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
