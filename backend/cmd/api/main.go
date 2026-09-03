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
	appbehavior "pano_chart/backend/application/behavior"
	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/application/market/regimehistory"
	"pano_chart/backend/application/market/transition"
	apprisk "pano_chart/backend/application/risk"
	"pano_chart/backend/application/setups"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"pano_chart/backend/infrastructure/events"
	"pano_chart/backend/infrastructure/feargreed"
	"pano_chart/backend/infrastructure/googleplay"
	"pano_chart/backend/infrastructure/market"
	"pano_chart/backend/infrastructure/news"
	"pano_chart/backend/infrastructure/overview"
	"pano_chart/backend/infrastructure/payment"
	"pano_chart/backend/infrastructure/rankings"
	"pano_chart/backend/infrastructure/snapshot"
	"pano_chart/backend/infrastructure/symbol_universe"

	appnotify "pano_chart/backend/application/notifications"
	appsocial "pano_chart/backend/application/social"
	infranotify "pano_chart/backend/infrastructure/notifications"
	infrasocial "pano_chart/backend/infrastructure/social"
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
		sidewaysAlgo = usecases.SidewaysAlgoV5 // default
	}
	var sidewaysCalc scoring.SymbolScoreCalculator
	switch sidewaysAlgo {
	case usecases.SidewaysAlgoV2:
		sidewaysCalc = &scoring.SidewaysV2ScoreCalculator{}
	case usecases.SidewaysAlgoV3:
		sidewaysCalc = &scoring.SidewaysV3ScoreCalculator{
			Config: scoring.DefaultSidewaysV3Config("1h"),
		}
	case usecases.SidewaysAlgoV4:
		sidewaysCalc = &scoring.SidewaysV4ScoreCalculator{}
	case usecases.SidewaysAlgoV5:
		sidewaysCalc = &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe("1h"),
		}
	default:
		sidewaysAlgo = usecases.SidewaysAlgoV5
		sidewaysCalc = &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe("1h"),
		}
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
	overviewCacheTTL := 3 * time.Minute // default
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
	rankingsCacheTTL := 3 * time.Minute // default
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

	// --- Payment infrastructure (SQLite-backed) ---
	paymentDBPath := os.Getenv("PAYMENT_DB_PATH")
	if paymentDBPath == "" {
		paymentDBPath = "./payment.sqlite"
	}
	paymentRepo, err := payment.NewSQLiteRepository(paymentDBPath)
	if err != nil {
		log.Fatalf("[main] FATAL: payment DB init failed: %v", err)
	}
	defer func() { _ = paymentRepo.Close() }()

	providerRegistry := usecases.NewPaymentProviderRegistry()

	// --- Google Play Billing provider ---
	gpPackage := os.Getenv("GOOGLE_PLAY_PACKAGE")
	gpSubID := os.Getenv("GOOGLE_PLAY_SUBSCRIPTION_ID")
	gpSAPath := os.Getenv("GOOGLE_PLAY_SERVICE_ACCOUNT_JSON")
	gpToken := os.Getenv("GOOGLE_PLAY_ACCESS_TOKEN")

	switch {
	case gpPackage != "" && gpSubID != "" && gpSAPath != "":
		// Preferred: service account with automatic token refresh.
		saClient, saErr := googleplay.NewServiceAccountClient(gpSAPath)
		if saErr != nil {
			log.Fatalf("[main] FATAL: Google Play service account init failed: %v", saErr)
		}
		gpProvider := googleplay.NewProvider(googleplay.Config{
			PackageName:    gpPackage,
			SubscriptionID: gpSubID,
		}, saClient)
		providerRegistry.Register(gpProvider)
		log.Printf("[main] Google Play provider registered via service account (pkg=%s, sub=%s)\n", gpPackage, gpSubID)

	case gpPackage != "" && gpSubID != "" && gpToken != "":
		// Fallback: raw access token (expires after ~1 h — for dev/testing).
		gpProvider := googleplay.NewProvider(googleplay.Config{
			PackageName:    gpPackage,
			SubscriptionID: gpSubID,
			AccessToken:    gpToken,
		}, nil)
		providerRegistry.Register(gpProvider)
		log.Printf("[main] Google Play provider registered with static token (pkg=%s, sub=%s)\n", gpPackage, gpSubID)

	default:
		log.Println("[main] Google Play provider not configured (missing env vars)")
	}

	subscriptionSvc := usecases.NewSubscriptionService(paymentRepo, paymentRepo)
	verifyPurchaseUC := usecases.NewVerifyPurchase(providerRegistry, subscriptionSvc)
	log.Printf("[main] Payment infrastructure initialized (db=%s)\n", paymentDBPath)

	// --- Market state service ---
	evalProvider := market.NewRankingsEvaluationProvider(rankingsUC)
	marketService := appmarket.NewMarketStateService(evalProvider)
	marketHandler := adhttp.NewMarketHandler(marketService)
	log.Println("[main] Market state service initialized")

	// --- Market composite index ---
	candleProvider := market.NewCompositeCandleProvider(
		cachedUniverse, candleRepo, exchangeInfoURL, tickerURL,
	)
	compositeService := metrics.NewCompositeIndexService(candleProvider, rankingWorkers)
	compositeCacheTTL := 3 * time.Minute
	compositeUC := market.NewRedisCachedComposite(compositeService, redisClient, compositeCacheTTL, "market_composite")
	compositeHandler := adhttp.NewMarketCompositeHandler(compositeUC)
	log.Println("[main] Market composite index service initialized")

	// --- Market regime detector ---
	metricsService := metrics.NewMetricsService(compositeService, candleProvider, evalProvider)

	// --- Regime history tracker (SQLite-backed) ---
	regimeHistoryDBPath := os.Getenv("PC_REGIME_HISTORY_DB")
	if regimeHistoryDBPath == "" {
		regimeHistoryDBPath = "./regime_history.sqlite"
	}
	regimeHistoryRepo, err := regimehistory.NewSQLiteRepository(regimeHistoryDBPath)
	if err != nil {
		log.Fatalf("Failed to open regime history DB: %v", err)
	}
	regimeTracker := regimehistory.NewTracker(regimeHistoryRepo)
	metricsService.SetObserver(regimeTracker)
	regimeHistoryService := regimehistory.NewService(regimeHistoryRepo)
	regimeHistoryHandler := adhttp.NewMarketRegimeHistoryHandler(regimeHistoryService)
	log.Printf("[main] Regime history tracker initialized (db=%s)\n", regimeHistoryDBPath)

	// --- Regime history backfill (runs once when DB is empty) ---
	backfiller := metrics.NewBackfiller(candleProvider, regimeTracker)
	for _, bfTF := range []string{"1h", "4h", "1d"} {
		hist, histErr := regimeHistoryService.GetHistory(bfTF, 1)
		if histErr != nil || len(hist.Periods) == 0 {
			log.Printf("[main] Backfilling regime history for %s...", bfTF)
			if bfErr := backfiller.Run(context.Background(), bfTF, 100); bfErr != nil {
				log.Printf("[main] Backfill %s failed: %v", bfTF, bfErr)
			} else {
				log.Printf("[main] Backfill %s complete", bfTF)
			}
		}
	}

	regimeHandler := adhttp.NewMarketRegimeHandler(metricsService)
	log.Println("[main] Market regime detector initialized")

	// --- Market transition probability engine ---
	transitionEngine := transition.NewTransitionEngine()
	transitionService := transition.NewTransitionService(metricsService, transitionEngine)
	transitionService.SetAgeProvider(regimeHistoryService)
	transitionHandler := adhttp.NewMarketTransitionHandler(transitionService)
	log.Println("[main] Market transition engine initialized")

	// --- Setup quality engine ---
	setupEngine := setups.NewEngine()
	setupService := setups.NewSetupService(candleRepo, symbolScorer, setupEngine)
	setupService.SetMarketProvider(metricsService)
	setupHandler := adhttp.NewSetupHandler(setupService)
	log.Println("[main] Setup quality engine initialized")

	// --- Fragility / risk engine ---
	riskEngine := apprisk.NewEngine()
	riskProvider := apprisk.NewCandleBasedDataProvider(candleRepo)
	riskService := apprisk.NewService(riskEngine, riskProvider)
	fragilityHandler := adhttp.NewFragilityHandler(riskService)
	setupService.SetFragilityProvider(riskService)

	// --- Behavior engine ---
	behaviorEngine := appbehavior.NewEngine()
	behaviorProvider := appbehavior.NewCandleBasedDataProvider(candleRepo, riskEngine)
	behaviorService := appbehavior.NewService(behaviorEngine, behaviorProvider)
	behaviorHandler := adhttp.NewBehaviorHandler(behaviorService)

	tokenRouter := adhttp.NewTokenRouter(setupHandler, fragilityHandler, behaviorHandler)
	log.Println("[main] Fragility risk engine initialized")
	log.Println("[main] Behavior engine initialized")

	// --- Social watcher service (RSS/Nitter) ---
	nitterBaseURL := os.Getenv("NITTER_BASE_URL")
	if nitterBaseURL == "" {
		nitterBaseURL = "http://127.0.0.1:8081"
	}
	socialCacheTTL := 90 * time.Second
	rssProvider := infrasocial.NewRSSProvider(nitterBaseURL, nil)

	socialDBPath := os.Getenv("SOCIAL_DB_PATH")
	if socialDBPath == "" {
		socialDBPath = "./social.sqlite"
	}
	socialAccountStore, err := infrasocial.NewSQLiteAccountStore(socialDBPath)
	if err != nil {
		log.Fatalf("[main] social account store: %v", err)
	}
	defer func() { _ = socialAccountStore.Close() }()

	socialSubStore, err := infrasocial.NewSQLiteSubscriptionStore(socialDBPath)
	if err != nil {
		log.Fatalf("[main] social subscription store: %v", err)
	}
	defer func() { _ = socialSubStore.Close() }()

	socialCache := appsocial.NewPostCache(socialCacheTTL)
	socialDispatcher := appsocial.NewDispatcher(256)
	socialService := appsocial.NewService(rssProvider, socialAccountStore, socialSubStore, socialCache)

	// Background watcher goroutine.
	socialWatcher := appsocial.NewWatcher(
		rssProvider, socialCache, socialAccountStore, socialSubStore,
		socialDispatcher, appsocial.DefaultWatcherConfig(),
	)
	socialCtx, socialCancel := context.WithCancel(context.Background())
	defer socialCancel()
	go socialWatcher.Run(socialCtx)
	log.Printf("[main] Social watcher started (nitter=%s, cache_ttl=%v)\n", nitterBaseURL, socialCacheTTL)

	// --- Push notifications (FCM) ---
	deviceDBPath := os.Getenv("DEVICE_DB_PATH")
	if deviceDBPath == "" {
		deviceDBPath = "./device_tokens.sqlite"
	}
	deviceStore, err := infrasocial.NewSQLiteDeviceStore(deviceDBPath)
	if err != nil {
		log.Fatalf("[main] device token store: %v", err)
	}
	defer func() { _ = deviceStore.Close() }()

	fcmCredsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	fcmProjectID := os.Getenv("FCM_PROJECT_ID")
	if fcmCredsPath != "" {
		fcmNotifier, err := infrasocial.NewFCMNotifier(fcmCredsPath, fcmProjectID)
		if err != nil {
			log.Printf("[main] FCM notifier init failed (push disabled): %v", err)
		} else {
			pushConsumer := appsocial.NewPushConsumer(
				socialDispatcher.Events(), socialSubStore, deviceStore, fcmNotifier,
			)
			go pushConsumer.Run(socialCtx)
			log.Println("[main] Push notification consumer started")
		}
	} else {
		log.Println("[main] GOOGLE_APPLICATION_CREDENTIALS not set — push notifications disabled")
	}

	// --- Notification config store (per-user preferences) ---
	notifConfigStore, err := infranotify.NewSQLiteConfigStore(deviceStore.DB())
	if err != nil {
		log.Fatalf("[main] notification config store: %v", err)
	}
	log.Println("[main] Notification config store initialized")

	// --- Notification engine (broadcast: market, setup, macro, news) ---
	if fcmCredsPath != "" {
		fcmForBroadcast, err := infrasocial.NewFCMNotifier(fcmCredsPath, fcmProjectID)
		if err != nil {
			log.Printf("[main] broadcast FCM init failed: %v", err)
		} else {
			broadcastSender := infranotify.NewBroadcastSender(deviceStore, fcmForBroadcast)
			notifyEngine := appnotify.NewEngine(broadcastSender, appnotify.DefaultEngineConfig())

			// Macro event provider adapter (wraps EventsUseCase).
			var macroProvider appnotify.EventProvider
			if eventsUC != nil {
				macroProvider = &eventsAdapter{uc: eventsUC}
			}

			setupScanAdapter := infranotify.NewSetupScanAdapter(setupService, rankingsUC)

			notifyScheduler := appnotify.NewScheduler(
				notifyEngine,
				metricsService,   // implements MarketProvider (CalculateRegime)
				setupScanAdapter, // scans top-ranked symbols for best setup
				macroProvider,
				appnotify.DefaultSchedulerConfig(),
			)
			notifyScheduler.SetConfigStore(notifConfigStore)
			notifyScheduler.SetSubscriptionChecker(subscriptionSvc)
			go notifyScheduler.Run(socialCtx)
			log.Println("[main] Notification engine + scheduler started")
		}
	}

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
	mux.Handle("/api/payments/verify", adhttp.NewVerifyPurchaseHandler(verifyPurchaseUC))
	mux.Handle("/api/subscription/status", adhttp.NewSubscriptionStatusHandler(subscriptionSvc))
	mux.Handle("/api/market/state", marketHandler)
	mux.Handle("/api/market/composite", compositeHandler)
	mux.Handle("/api/market/regime", regimeHandler)
	mux.Handle("/api/market/regime/history", regimeHistoryHandler)
	mux.Handle("/api/market/transition", transitionHandler)
	mux.Handle("/api/token/", tokenRouter)
	log.Println("[main] /api/market/state endpoint registered")
	log.Println("[main] /api/market/composite endpoint registered")
	log.Println("[main] /api/market/regime endpoint registered")
	log.Println("[main] /api/market/regime/history endpoint registered")
	log.Println("[main] /api/market/transition endpoint registered")
	log.Println("[main] /api/payments/verify and /api/subscription/status endpoints registered")

	// Social endpoints
	mux.Handle("/api/social/subscribe", adhttp.NewSocialSubscribeHandler(socialService))
	mux.Handle("/api/social/unsubscribe", adhttp.NewSocialUnsubscribeHandler(socialService))
	mux.Handle("/api/social/subscribe/settings", adhttp.NewSocialSubscribeSettingsHandler(socialService))
	mux.Handle("/api/social/feed", adhttp.NewSocialFeedHandler(socialService))
	mux.Handle("/api/social/accounts", adhttp.NewSocialAccountsHandler(socialService))
	log.Println("[main] /api/social/* endpoints registered")

	// Device registration endpoints (push notifications)
	mux.Handle("/api/device/register", adhttp.NewDeviceRegisterHandler(deviceStore))
	mux.Handle("/api/device/unregister", adhttp.NewDeviceUnregisterHandler(deviceStore))
	log.Println("[main] /api/device/* endpoints registered")

	// Notification config endpoint
	mux.Handle("/api/notification/config", adhttp.NewNotificationConfigHandler(notifConfigStore))
	log.Println("[main] /api/notification/config endpoint registered")

	// Volatility profile endpoint
	volPath := os.Getenv("VOL_OUTPUT")
	if volPath == "" {
		volPath = "volatility_1m.json"
	}
	mux.Handle("/api/volatility", adhttp.NewVolatilityHandler(volPath))
	log.Println("[main] /api/volatility endpoint registered")

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

// eventsAdapter adapts EventsUseCase to the notification scheduler's EventProvider.
type eventsAdapter struct {
	uc usecases.EventsUseCase
}

func (a *eventsAdapter) FetchEvents(ctx context.Context, from, to time.Time) ([]domain.Event, error) {
	return a.uc.Execute(ctx, usecases.GetEventsRequest{
		DateFrom: from,
		DateTo:   to,
		Country:  "United States",
	})
}
