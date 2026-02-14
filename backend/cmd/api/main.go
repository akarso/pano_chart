package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/infrastructure/symbol_universe"
)

func main() {
	addr := ":8080"
	binanceBase := os.Getenv("PC_BINANCE_BASE_URL")
	if binanceBase == "" {
		binanceBase = symbol_universe.DefaultBinanceAPIBaseURL
	}
	exchangeInfoURL, tickerURL := symbol_universe.BuildBinanceURLs(binanceBase)
	redisAddr := os.Getenv("PC_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// --- Parse sparkline precision at startup ---
	sparklinePrecision := 30 // default
	if precStr := os.Getenv("OVERVIEW_SPARKLINE_PRECISION"); precStr != "" {
		if prec, err := strconv.Atoi(precStr); err == nil && prec > 0 {
			if prec > 200 {
				prec = 200 // clamp to max
			}
			sparklinePrecision = prec
		}
	}
	fmt.Printf("[main] Sparkline precision: %d\n", sparklinePrecision)

	// --- Redis client wiring ---
	redisClient := symbol_universe.NewGoRedisClient(redisAddr)
	redisAdapter := infra.NewRedisMinimalAdapter(redisClient)

	// --- CandleRepository with Redis caching ---
	baseRepo := infra.NewFreeTierCandleRepository(binanceBase, nil)
	cacheTTL := 5 * time.Minute
	candleRepo := infra.NewRedisCandleRepository(redisAdapter, baseRepo, cacheTTL)

	// --- Dynamic Binance Universe and Volume Providers with Redis caching ---
	binanceHTTP := http.DefaultClient
	universe := symbol_universe.NewBinanceExchangeInfoUniverse(binanceHTTP, 50)
	cachedUniverse := symbol_universe.NewRedisCachedSymbolUniverse(
		universe, redisClient, 30*time.Minute, "symbol_universe:exchange_info",
	)
	volumeProvider := symbol_universe.NewBinance24hTickerVolumeProvider(binanceHTTP, tickerURL)
	cachedVolumeProvider := symbol_universe.NewRedisCachedVolumeProvider(
		volumeProvider, redisClient, 2*time.Minute, "binance:24h_volume",
	)

	// --- Use cases ---
	weights := []usecases.ScoreWeight{
		{Calculator: &scoring.SidewaysConsistencyScoreCalculator{}, Weight: 1.0},
		{Calculator: &scoring.TrendPredictabilityScoreCalculator{}, Weight: 1.0},
		{Calculator: &scoring.GainLossScoreCalculator{}, Weight: 1.0},
	}
	rankUC := usecases.NewVolumeSortedRankSymbols(cachedUniverse, cachedVolumeProvider, weights, exchangeInfoURL, tickerURL)
	symbolScorer := usecases.NewWeightedSymbolScorer(weights)
	getCandleUC := usecases.NewGetCandleSeries(candleRepo)
	getSymbolDetailUC := usecases.NewGetSymbolDetail(
		candleRepo,
		symbolScorer,
		cachedUniverse,
		exchangeInfoURL,
		tickerURL,
		usecases.DefaultSymbolDetailLimit,
		usecases.MaxSymbolDetailLimit,
	)

	// --- State snapshot before handler registration ---
	fmt.Printf("[main] ==== STATE SNAPSHOT BEFORE HANDLER REGISTRATION ====\n")
	ctx := context.Background()

	// Test universe
	univ, err := cachedUniverse.Symbols(ctx, exchangeInfoURL, tickerURL)
	if err != nil {
		fmt.Printf("[main] Universe error: %v\n", err)
	} else {
		fmt.Printf("[main] Universe size: %d\n", len(univ))
		if len(univ) > 0 {
			fmt.Printf("[main] Universe sample (first 5):\n")
			for i := 0; i < 5 && i < len(univ); i++ {
				fmt.Printf("[main]   [%d] %s\n", i, univ[i].String())
			}
		}
	}

	// Test volume provider
	vols, err := cachedVolumeProvider.Volumes(ctx)
	if err != nil {
		fmt.Printf("[main] Volume provider error: %v\n", err)
	} else {
		fmt.Printf("[main] Volume map size: %d\n", len(vols))
		if len(univ) > 0 && len(vols) > 0 {
			// Check if sample universe symbols exist in volume map
			foundCount := 0
			for i := 0; i < 5 && i < len(univ); i++ {
				if vol, ok := vols[univ[i].String()]; ok {
					fmt.Printf("[main]   %s: volume=%.2f\n", univ[i].String(), vol)
					foundCount++
				}
			}
			if foundCount == 0 {
				fmt.Printf("[main]   WARNING: First 5 universe symbols NOT found in volume map!\n")
			}
		}
	}

	// Test ranker
	fmt.Printf("[main] Ranker type: %T\n", rankUC)
	fmt.Printf("[main] Ranker Universe provider: %v\n", rankUC.Universe())
	fmt.Printf("[main] Ranker Volumes field: %v\n", rankUC.Volumes)
	fmt.Printf("[main] ==== END STATE SNAPSHOT ====\n")

	// --- Overview use case ---
	getOverviewUC := usecases.NewGetOverview(rankUC, candleRepo, sparklinePrecision, 5)

	// --- Handlers ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("/health write error: %v", err)
		}
	})
	mux.Handle("/api/v1/candles", adhttp.NewGetCandleSeriesHandler(getCandleUC))
	mux.Handle("/api/rankings", &adhttp.RankingsHandler{
		Ranker:          rankUC,
		CandleRepo:      candleRepo,
		Symbols:         nil, // Not needed, dynamic universe used
		ExchangeInfoURL: exchangeInfoURL,
		TickerURL:       tickerURL,
	})
	mux.Handle("/api/overview", adhttp.NewOverviewHandler(getOverviewUC))
	mux.Handle("/api/symbol/", adhttp.NewSymbolDetailHandler(getSymbolDetailUC))

	fmt.Printf("Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
