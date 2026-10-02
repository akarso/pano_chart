// Command export_dataset writes a CSV of resolved setup signals for offline
// regime-model training (PR-109).
//
// IMPORTANT: Collect training CSVs while scoring.regime_model is heuristic.
// regime_label comes from regime_code at emission (result.Regime). Under a
// learned model that label is the model's own prediction — not ground truth.
//
// Features come from signal.Context (setupRegimeFeatures at emit). atr_pct is
// taken only from Context (Wilder TrueATR/price); missing → empty cell (no
// SimpleATR backfill from the atr column). success is the outcome grade —
// train OVR structure heads on regime_label, not success.
//
// Default: kind=setup, oldest-first. Use -max-rows to cap memory on large DBs
// (0 = unlimited).
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

func main() {
	dbPath := flag.String("db", envOr("PC_SIGNAL_DB", "./signals.sqlite"), "signals sqlite path")
	outPath := flag.String("out", "dataset.csv", "output CSV path")
	kind := flag.String("kind", "setup", "signal kind filter (default setup; empty = all kinds — incomplete features)")
	sinceDays := flag.Int("since-days", 0, "only signals emitted in the last N days (0 = all)")
	maxRows := flag.Int("max-rows", 0, "cap rows loaded from DB (0 = unlimited; loads full join into memory)")
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

	// Limit < 0 → unlimited; OldestFirst → ASC for training splits.
	limit := -1
	if *maxRows > 0 {
		limit = *maxRows
	}
	filter := domainsignal.Filter{Limit: limit, OldestFirst: true}
	if *kind != "" {
		filter.Kind = domainsignal.Kind(*kind)
	}
	if *sinceDays > 0 {
		filter.Since = time.Now().UTC().Add(-time.Duration(*sinceDays) * 24 * time.Hour)
	}

	rows, err := repo.Query(context.Background(), filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query: %v\n", err)
		os.Exit(1)
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

	scanned := len(rows)
	written := 0
	skippedNoOutcome := 0
	for _, row := range rows {
		if row.Outcome == nil {
			skippedNoOutcome++
			continue
		}
		rec := recordFor(row)
		if err := w.Write(rec); err != nil {
			fmt.Fprintf(os.Stderr, "write row: %v\n", err)
			os.Exit(1)
		}
		written++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "flush: %v\n", err)
		os.Exit(1)
	}
	unlimited := *maxRows <= 0
	fmt.Printf("scanned=%d resolved_written=%d skipped_no_outcome=%d out=%s kind=%q max_rows=%d unlimited=%v oldest_first=true\n",
		scanned, written, skippedNoOutcome, *outPath, *kind, *maxRows, unlimited)
	fmt.Fprintf(os.Stderr, "note: collect training CSVs under scoring.regime_model=heuristic so regime_label is independent of a learned classifier\n")
}

func recordFor(row domainsignal.SignalWithOutcome) []string {
	sig := row.Signal
	oc := row.Outcome
	ctx := sig.Context
	if ctx == nil {
		ctx = map[string]float64{}
	}
	feat := func(k string) string {
		v, ok := ctx[k]
		if !ok {
			return ""
		}
		return fmtFloat(v)
	}
	success := "0"
	if oc.Success {
		success = "1"
	}
	regimeLabel := ""
	if code, ok := ctx["regime_code"]; ok {
		regimeLabel = decodeRegimeCode(code)
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
	out = append(out, regimeLabel, success, fmtFloat(oc.ForwardReturn), oc.Rule)
	return out
}

func decodeRegimeCode(code float64) string {
	switch int(code + 0.5) {
	case 1:
		return "trend"
	case 2:
		return "compression"
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
