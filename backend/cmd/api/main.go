package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rs/cors"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/infrastructure/events"
	"pano_chart/backend/infrastructure/feargreed"
	"pano_chart/backend/infrastructure/news"
	"pano_chart/backend/infrastructure/overview"
	"pano_chart/backend/infrastructure/rankings"
	"pano_chart/backend/infrastructure/snapshot"
	"pano_chart/backend/infrastructure/symbol_universe"
)

func main() {
	// Load YAML configuration at startup
	scoring.MustLoadConfig(scoring.ConfigPath())

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
	sparklinePrecision := 110 // default
	if precStr := os.Getenv("OVERVIEW_SPARKLINE_PRECISION"); precStr != "" {
		if prec, err := strconv.Atoi(precStr); err == nil && prec > 0 {
			if prec > 200 {
				prec = 200 // clamp to max
			}
			sparklinePrecision = prec
		}
	}

	// --- Parse ranking worker count ---
	rankingWorkers := 20 // default
	if wStr := os.Getenv("RANKING_WORKERS"); wStr != "" {
		if w, err := strconv.Atoi(wStr); err == nil && w > 0 {
			if w > 64 {
				w = 64 // clamp to sane max
			}
			rankingWorkers = w
		}
	}

	// --- HTTP client with connection pooling for Binance API ---
	binanceTransport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}
	binanceClient := &http.Client{
		Transport: binanceTransport,
		Timeout:   10 * time.Second,
	}

	// --- Redis client wiring ---
	redisClient := symbol_universe.NewGoRedisClient(redisAddr)
	redisAdapter := infra.NewRedisMinimalAdapter(redisClient)

	// --- CandleRepository with Redis caching ---
	baseRepo := infra.NewFreeTierCandleRepository(binanceBase, binanceClient, nil)
	cacheTTL := 5 * time.Minute
	candleRepo := infra.NewRedisCandleRepository(redisAdapter, baseRepo, cacheTTL)

	// --- Dynamic Binance Universe and Volume Providers with Redis caching ---
	binanceHTTP := http.DefaultClient
	universe := symbol_universe.NewBinanceExchangeInfoUniverse(binanceHTTP, 153)
	cachedUniverse := symbol_universe.NewRedisCachedSymbolUniverse(
		universe, redisClient, 30*time.Minute, "symbol_universe:exchange_info",
	)
	volumeProvider := symbol_universe.NewBinance24hTickerVolumeProvider(binanceHTTP, tickerURL)
	cachedVolumeProvider := symbol_universe.NewRedisCachedVolumeProvider(
		volumeProvider, redisClient, 2*time.Minute, "binance:24h_volume",
	)

	// --- Sideways algorithm selection ---
	sidewaysAlgo := usecases.SidewaysAlgoMode(os.Getenv("SIDEWAYS_ALGO"))
	if sidewaysAlgo == "" {
		sidewaysAlgo = usecases.SidewaysAlgoV1 // default
	}
	var sidewaysCalc scoring.SymbolScoreCalculator
	switch sidewaysAlgo {
	case usecases.SidewaysAlgoV2:
		sidewaysCalc = &scoring.SidewaysV2ScoreCalculator{}
	case usecases.SidewaysAlgoV3:
		sidewaysCalc = &scoring.SidewaysV3ScoreCalculator{
			Config: scoring.DefaultSidewaysV3Config("1h"),
		}
	default:
		sidewaysAlgo = usecases.SidewaysAlgoV1
		sidewaysCalc = &scoring.SidewaysConsistencyScoreCalculator{}
	}

	// --- Use cases ---
	weights := []usecases.ScoreWeight{
		{Calculator: sidewaysCalc, Weight: 1.0},
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

	// --- Overview use case ---
	getOverviewUC := usecases.NewGetOverview(rankUC, candleRepo, sparklinePrecision, 5)

	// --- Overview cache TTL ---
	overviewCacheTTL := 45 * time.Second // default
	if ttlStr := os.Getenv("OVERVIEW_CACHE_TTL_SECONDS"); ttlStr != "" {
		if secs, err := strconv.Atoi(ttlStr); err == nil {
			if secs < 5 {
				secs = 5
			}
			if secs > 300 {
				secs = 300
			}
			overviewCacheTTL = time.Duration(secs) * time.Second
		}
	}

	// Wrap with Redis cache decorator
	overviewUC := overview.NewRedisCachedOverview(getOverviewUC, redisClient, overviewCacheTTL, "overview")

	// --- Evaluation snapshot logger (async, buffered) ---
	snapshotLogger := snapshot.NewAsyncChannelLogger(1000, 50, 5*time.Second, func(batch []domain.EvaluationSnapshot) {
		for _, s := range batch {
			log.Printf("[snapshot] %s %s sideways=%.4f trend=%.4f price=%.2f atr=%.6f algo=%s",
				s.Symbol, s.Timeframe, s.SidewaysScore, s.TrendScore, s.Price, s.ATR, s.AlgoVersion)
		}
	})
	defer snapshotLogger.Stop()

	// --- Rankings v2 use case ---
	getRankingsUC := usecases.NewGetRankings(
		cachedUniverse,
		rankUC,
		cachedVolumeProvider,
		candleRepo,
		exchangeInfoURL,
		tickerURL,
		sparklinePrecision,
		sidewaysAlgo,
		weights,
		rankingWorkers,
		snapshotLogger,
	)

	// --- Rankings cache TTL ---
	rankingsCacheTTL := 60 * time.Second // default
	if ttlStr := os.Getenv("RANKINGS_CACHE_TTL_SECONDS"); ttlStr != "" {
		if secs, err := strconv.Atoi(ttlStr); err == nil {
			if secs < 10 {
				secs = 10
			}
			if secs > 300 {
				secs = 300
			}
			rankingsCacheTTL = time.Duration(secs) * time.Second
		}
	}

	// Wrap with Redis cache decorator
	rankingsUC := rankings.NewRedisCachedRankings(getRankingsUC, redisClient, rankingsCacheTTL, "rankings")

	// --- Events use case ---
	configPath := scoring.ConfigPath()
	ffAPIKey, err := events.LoadFinanceFlowAPIKey(configPath)
	if err != nil {
		log.Printf("[main] WARNING: FinanceFlow API key not found: %v (events endpoint disabled)", err)
	}
	var eventsUC usecases.EventsUseCase
	if ffAPIKey != "" {
		ffClient := events.NewFinanceFlowClient(ffAPIKey, "", nil)
		eventsUC = usecases.NewGetEvents(ffClient)
	}

	// --- Fear & Greed use case (cached 6h) ---
	fearGreedFetcher := feargreed.NewFetcher(nil, "")
	fearGreedUC := feargreed.NewRedisCachedFearGreed(fearGreedFetcher, redisClient, 6*time.Hour)

	// --- News use case (filesystem-based, cached 5min) ---
	newsDir := os.Getenv("NEWS_DIR")
	if newsDir == "" {
		newsDir = "./news"
	}
	newsRepo := news.NewFsNewsRepository(newsDir, 5*time.Minute)
	newsUC := usecases.NewGetNews(newsRepo)

	// --- Handlers ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("/health write error: %v", err)
		}
	})
	mux.Handle("/api/v1/candles", adhttp.NewGetCandleSeriesHandler(getCandleUC))
	mux.Handle("/api/rankings", adhttp.NewRankingsV2Handler(rankingsUC))
	mux.Handle("/api/overview", adhttp.NewOverviewHandler(overviewUC))
	mux.Handle("/api/symbol/", adhttp.NewSymbolDetailHandler(getSymbolDetailUC))
	mux.Handle("/api/v1/fear-greed", adhttp.NewFearGreedHandler(fearGreedUC))
	mux.Handle("/api/news", adhttp.NewNewsHandler(newsUC))
	mux.Handle("/api/news/", adhttp.NewNewsHandler(newsUC))
	log.Println("[main] /api/news endpoint registered")
	if eventsUC != nil {
		mux.Handle("/api/v1/events", adhttp.NewEventsHandler(eventsUC))
		log.Println("[main] /api/v1/events endpoint registered")
	}

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	})
	handler := c.Handler(mux)

	fmt.Printf("Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}
