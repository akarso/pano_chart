package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/infrastructure/symbol_universe"
	"pano_chart/backend/domain/scoring"
)

func main() {
	addr := ":8080"
	binanceBase := os.Getenv("PC_BINANCE_BASE_URL")
	if binanceBase == "" {
		binanceBase = "https://api.binance.com/api/v3"
	}
	redisAddr := os.Getenv("PC_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	// --- Redis client wiring ---
	redisClient := symbol_universe.NewGoRedisClient(redisAddr)
	redisAdapter := infra.NewRedisMinimalAdapter(redisClient)

	// --- CandleRepository with Redis caching ---
	baseRepo := infra.NewFreeTierCandleRepository(binanceBase, nil)
	cacheTTL := 5 * time.Minute
	candleRepo := infra.NewRedisCandleRepository(redisAdapter, baseRepo, cacheTTL)

	// --- Dynamic Binance Universe and Volume Providers with Redis caching ---
	binanceHTTP := http.DefaultClient
	universe := symbol_universe.NewBinanceExchangeInfoUniverse(binanceHTTP, binanceBase+"/exchangeInfo", 50)
	cachedUniverse := symbol_universe.NewRedisCachedSymbolUniverse(
		universe, redisClient, 30*time.Minute, "symbol_universe:exchange_info",
	)
	volumeProvider := symbol_universe.NewBinance24hTickerVolumeProvider(binanceHTTP, binanceBase+"/ticker/24hr")
	cachedVolumeProvider := symbol_universe.NewRedisCachedVolumeProvider(
		volumeProvider, redisClient, 2*time.Minute, "binance:24h_volume",
	)

	// --- Use cases ---
	weights := []usecases.ScoreWeight{
		{Calculator: &scoring.SidewaysConsistencyScoreCalculator{}, Weight: 1.0},
		{Calculator: &scoring.TrendPredictabilityScoreCalculator{}, Weight: 1.0},
		{Calculator: &scoring.GainLossScoreCalculator{}, Weight: 1.0},
	}
	rankUC := usecases.NewVolumeSortedRankSymbols(cachedUniverse, cachedVolumeProvider, weights)
	getCandleUC := usecases.NewGetCandleSeries(candleRepo)

	// --- Handlers ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("/health write error: %v", err)
		}
	})
	mux.Handle("/api/v1/candles", adhttp.NewGetCandleSeriesHandler(getCandleUC))
	mux.Handle("/api/rankings", &adhttp.RankingsHandler{
		Ranker:     rankUC,
		CandleRepo: candleRepo,
		Symbols:    nil, // Not needed, dynamic universe used
	})

	fmt.Printf("Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
