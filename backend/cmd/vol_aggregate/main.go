package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pano_chart/backend/application/market/metrics"
	vol "pano_chart/backend/infrastructure/volatility"
)

const atrPeriod = 14

func main() {
	symbolsFlag := flag.String("symbols", "", "comma-separated symbols (default: BTCUSDT)")
	outPrefix := flag.String("out", "", "path stem for per-sector files (same as VOL_SECTOR_PREFIX; e.g. /data/vol → /data/vol_l1.json); empty skips sector files")
	sectorsPath := flag.String("sectors", "", "sectors.yaml path (default: SECTORS_CONFIG_PATH / config lookup)")
	daysFlag := flag.Int("days", 150, "lookback days of 1m candles")
	dbPathFlag := flag.String("db", "", "sqlite candle cache path (default: VOL_DB_PATH)")
	marketOutFlag := flag.String("market-out", "", "market-wide profile path (default: VOL_OUTPUT)")
	marketSymFlag := flag.String("market-symbol", "BTCUSDT", "symbol whose profile is written as market-wide (must be in --symbols)")
	flag.Parse()

	symbols := parseSymbols(*symbolsFlag)
	if len(symbols) == 0 {
		symbols = []string{"BTCUSDT"}
	}

	days := *daysFlag
	if days <= 0 {
		days = 150
	}
	dbPath := *dbPathFlag
	if dbPath == "" {
		dbPath = envOrDefault("VOL_DB_PATH", "/var/www/pano_charts/volatility_candles.sqlite")
	}
	marketOut := *marketOutFlag
	if marketOut == "" {
		marketOut = envOrDefault("VOL_OUTPUT", "/var/www/pano_charts/volatility_1m.json")
	}
	marketSym := strings.ToUpper(strings.TrimSpace(*marketSymFlag))
	if marketSym == "" {
		marketSym = "BTCUSDT"
	}

	cache, err := vol.NewCandleCache(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cache: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = cache.Close() }()

	client := &http.Client{Timeout: 30 * time.Second}
	fetcher := vol.NewFetcher(client)
	ctx := context.Background()

	results := make(map[string]vol.FullResult, len(symbols))
	for _, symbol := range symbols {
		full, aerr := aggregateSymbol(ctx, cache, fetcher, symbol, days)
		if aerr != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", symbol, aerr)
			os.Exit(1)
		}
		results[symbol] = full
		fmt.Printf("Aggregated %s\n", symbol)
	}

	marketResult, ok := results[marketSym]
	if !ok {
		fmt.Fprintf(os.Stderr, "market-symbol %s not in --symbols %v\n", marketSym, symbols)
		os.Exit(1)
	}
	if err := vol.SaveFullResult(marketResult, marketOut); err != nil {
		fmt.Fprintf(os.Stderr, "save market: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Market-wide (%s): %s\n", marketSym, marketOut)

	prefix := strings.TrimSpace(*outPrefix)
	if prefix == "" {
		return
	}

	secPath := *sectorsPath
	if secPath == "" {
		secPath = metrics.SectorsPath()
	}
	catalog, err := metrics.LoadSectorCatalog(secPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sectors: %v\n", err)
		os.Exit(1)
	}

	bySector := map[string][]vol.FullResult{}
	for _, sym := range symbols {
		id := catalog.ForSymbol(sym)
		if id == "other" {
			fmt.Printf("skip %s: not in any configured sector\n", sym)
			continue
		}
		bySector[id] = append(bySector[id], results[sym])
	}

	for _, sec := range catalog.Sectors() {
		outFile, perr := metrics.SectorProfilePath(prefix, sec.ID)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "sector path %s: %v\n", sec.ID, perr)
			os.Exit(1)
		}
		group := bySector[sec.ID]
		if len(group) == 0 {
			// Stale profile from a prior run would keep serving old seasonality;
			// remove it so the API falls back to market-wide.
			if err := os.Remove(outFile); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "remove stale %s: %v\n", outFile, err)
				os.Exit(1)
			}
			if err == nil {
				fmt.Printf("Sector %s: removed stale profile (no symbols in this run)\n", sec.ID)
			}
			continue
		}
		merged := vol.AverageFullResults(group)
		if dir := filepath.Dir(outFile); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
				os.Exit(1)
			}
		}
		if err := vol.SaveFullResult(merged, outFile); err != nil {
			fmt.Fprintf(os.Stderr, "save %s: %v\n", outFile, err)
			os.Exit(1)
		}
		fmt.Printf("Sector %s (%d symbols): %s\n", sec.ID, len(group), outFile)
	}
}

func parseSymbols(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func aggregateSymbol(
	ctx context.Context,
	cache *vol.CandleCache,
	fetcher *vol.Fetcher,
	symbol string,
	days int,
) (vol.FullResult, error) {
	end := time.Now().UnixMilli()
	start := time.Now().AddDate(0, 0, -days).UnixMilli()

	maxCached, err := cache.MaxOpenTime(symbol)
	if err != nil {
		return vol.FullResult{}, fmt.Errorf("max open time: %w", err)
	}

	fetchStart := start
	if maxCached > 0 && maxCached+60000 > fetchStart {
		fetchStart = maxCached + 60000
	}

	if fetchStart < end {
		fmt.Printf("Fetching %s candles from %s ...\n",
			symbol,
			time.UnixMilli(fetchStart).UTC().Format("2006-01-02 15:04"),
		)
		current := fetchStart
		for current < end {
			next := current + 1000*60*1000
			candles, ferr := fetcher.FetchCandles(ctx, symbol, current, next)
			if ferr != nil {
				return vol.FullResult{}, fmt.Errorf("fetch: %w", ferr)
			}
			if len(candles) == 0 {
				// Listing may post-date the lookback start; skip empty early
				// windows and keep scanning toward `end`. Still pace requests
				// so a long empty prefix does not 429 and abort the run.
				current = next
				time.Sleep(200 * time.Millisecond)
				continue
			}
			if serr := cache.Store(symbol, candles); serr != nil {
				return vol.FullResult{}, fmt.Errorf("store: %w", serr)
			}
			current = candles[len(candles)-1].OpenTime + 60000
			time.Sleep(200 * time.Millisecond)
		}
	} else {
		fmt.Printf("%s: cache up to date, skipping fetch.\n", symbol)
	}

	candles, err := cache.Load(symbol, start, end)
	if err != nil {
		return vol.FullResult{}, fmt.Errorf("load: %w", err)
	}
	// ATR period needs atrPeriod prior bars; weekly slice starts at atrPeriod.
	minCandles := atrPeriod + 1
	if len(candles) < minCandles {
		return vol.FullResult{}, fmt.Errorf(
			"insufficient candles: got %d, need ≥ %d (ATR period %d + 1)",
			len(candles), minCandles, atrPeriod,
		)
	}

	result := vol.Aggregate(candles)
	intraday := vol.BuildAllTimeframes(result.Buckets)
	atr := vol.ComputeATR(candles, atrPeriod)
	weekly := vol.BuildWeekly(candles[atrPeriod:], atr[atrPeriod:])
	dailyBuckets := vol.DeriveDailyOfWeek(weekly)
	intraday = append(intraday, vol.TimeframeResult{
		Timeframe: vol.TF1d,
		Buckets:   dailyBuckets,
	})
	return vol.FullResult{Intraday: intraday, Weekly: weekly}, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
