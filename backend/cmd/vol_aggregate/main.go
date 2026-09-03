package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	vol "pano_chart/backend/infrastructure/volatility"
)

func main() {
	symbol := "BTCUSDT"
	days := 150
	dbPath := envOrDefault("VOL_DB_PATH", "/var/www/pano_charts/volatility_candles.sqlite")
	outFile := envOrDefault("VOL_OUTPUT", "/var/www/pano_charts/volatility_1m.json")

	// --- SQLite candle cache ---
	cache, err := vol.NewCandleCache(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cache: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = cache.Close() }()

	// --- Determine fetch window ---
	end := time.Now().UnixMilli()
	start := time.Now().AddDate(0, 0, -days).UnixMilli()

	// Resume from the latest cached candle if any.
	maxCached, err := cache.MaxOpenTime(symbol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "max open time: %v\n", err)
		os.Exit(1)
	}

	fetchStart := start
	if maxCached > 0 && maxCached+60000 > fetchStart {
		fetchStart = maxCached + 60000 // next minute after last cached
	}

	// --- Fetch missing candles ---
	client := &http.Client{Timeout: 30 * time.Second}
	fetcher := vol.NewFetcher(client)
	ctx := context.Background()

	if fetchStart < end {
		fmt.Printf("Fetching %s candles from %s ...\n",
			symbol,
			time.UnixMilli(fetchStart).UTC().Format("2006-01-02 15:04"),
		)

		current := fetchStart
		for current < end {
			next := current + 1000*60*1000 // 1000-minute window

			candles, ferr := fetcher.FetchCandles(ctx, symbol, current, next)
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "fetch: %v\n", ferr)
				os.Exit(1)
			}
			if len(candles) == 0 {
				break
			}

			if serr := cache.Store(symbol, candles); serr != nil {
				fmt.Fprintf(os.Stderr, "store: %v\n", serr)
				os.Exit(1)
			}

			current = candles[len(candles)-1].OpenTime + 60000
			time.Sleep(200 * time.Millisecond) // Binance rate-limit courtesy
		}
	} else {
		fmt.Println("Cache is up to date, skipping fetch.")
	}

	// --- Load full window from cache ---
	count, _ := cache.Count(symbol)
	fmt.Printf("Cached candles: %d\n", count)

	candles, err := cache.Load(symbol, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d candles for aggregation.\n", len(candles))

	// --- Aggregate ---
	fmt.Println("Aggregating...")
	result := vol.Aggregate(candles)
	fmt.Printf("Produced %d non-empty minute buckets.\n", len(result.Buckets))

	// --- Derive multi-timeframe ---
	fmt.Println("Deriving multi-timeframe buckets...")
	intraday := vol.BuildAllTimeframes(result.Buckets)
	for _, tf := range intraday {
		fmt.Printf("  %s: %d buckets\n", tf.Timeframe, len(tf.Buckets))
	}

	// --- Weekly seasonality ---
	fmt.Println("Computing weekly seasonality...")
	const atrPeriod = 14
	atr := vol.ComputeATR(candles, atrPeriod)
	weekly := vol.BuildWeekly(candles[atrPeriod:], atr[atrPeriod:])
	fmt.Printf("Weekly buckets: %d\n", len(weekly.Buckets))

	// --- Day-of-week (1d) from weekly data ---
	dailyBuckets := vol.DeriveDailyOfWeek(weekly)
	intraday = append(intraday, vol.TimeframeResult{
		Timeframe: vol.TF1d,
		Buckets:   dailyBuckets,
	})
	fmt.Printf("  1d (day-of-week): %d buckets\n", len(dailyBuckets))

	// --- Save JSON ---
	full := vol.FullResult{
		Intraday: intraday,
		Weekly:   weekly,
	}
	if err := vol.SaveFullResult(full, outFile); err != nil {
		fmt.Fprintf(os.Stderr, "save: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Done. Output: %s\n", outFile)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
