import 'package:http/http.dart' as http;

import '../../features/billing/api/subscription_api.dart';
import '../../features/billing/billing_manager.dart';
import '../../features/billing/infrastructure/http_subscription_api.dart';
import '../../features/billing/trial_manager.dart';
import '../../features/bubble_map/bubble_map_view_model.dart';
import '../../features/candles/application/get_candle_series.dart';
import '../../features/candles/application/get_candle_series.dart' as impl;
import '../../features/candles/infrastructure/http_candle_api.dart';
import '../../features/events/api/events_api.dart';
import '../../features/events/application/get_events.dart';
import '../../features/events/events_view_model.dart';
import '../../features/events/infrastructure/http_events_api.dart';
import '../../features/fear_greed/http_fear_greed_api.dart';
import '../../features/market_state/http_composite_index_api.dart';
import '../../features/market_state/http_market_state_api.dart';
import '../../features/news/api/news_api.dart';
import '../../features/news/application/get_news.dart';
import '../../features/news/infrastructure/http_news_api.dart';
import '../../features/news/news_view_model.dart';
import '../../features/overview/get_rankings_impl.dart';
import '../../features/overview/http_rankings_api.dart';
import '../../features/overview/overview_view_model.dart';

/// Composition root responsible for explicitly wiring dependencies.
class CompositionRoot {
  final String apiBaseUrl;
  final http.Client httpClient;
  final int stablecoinPadding;

  CompositionRoot({
    required this.apiBaseUrl,
    http.Client? httpClient,
    this.stablecoinPadding = 0,
  }) : httpClient = httpClient ?? http.Client();

  /// Creates a wired GetCandleSeries use case instance.
  GetCandleSeries createGetCandleSeries() {
    final api = HttpCandleApi(baseUrl: apiBaseUrl, client: httpClient);
    return impl.GetCandleSeriesImpl(api);
  }

  /// Creates a wired OverviewViewModel backed by the rankings API.
  OverviewViewModel createOverviewViewModel() {
    final api = HttpRankingsApi(baseUrl: apiBaseUrl, client: httpClient);
    final getRankings = GetRankingsImpl(api, pageSize: 30 + stablecoinPadding);
    return OverviewViewModel(getRankings);
  }

  /// Creates a wired EventsViewModel backed by the events API.
  EventsViewModel createEventsViewModel() {
    final api = HttpEventsApi(client: httpClient, baseUrl: apiBaseUrl);
    final getEvents = GetEventsImpl(api);
    return EventsViewModel(getEvents);
  }

  /// Creates a wired BubbleMapViewModel backed by the rankings API.
  BubbleMapViewModel createBubbleMapViewModel() {
    final api = HttpRankingsApi(baseUrl: apiBaseUrl, client: httpClient);
    final getRankings = GetRankingsImpl(api, pageSize: 50);
    return BubbleMapViewModel(getRankings);
  }

  /// Creates a wired FearGreedApi.
  FearGreedApi createFearGreedApi() {
    return HttpFearGreedApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired MarketStateApi.
  MarketStateApi createMarketStateApi() {
    return HttpMarketStateApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired CompositeIndexApi.
  CompositeIndexApi createCompositeIndexApi() {
    return HttpCompositeIndexApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired NewsViewModel backed by the news API.
  NewsViewModel createNewsViewModel() {
    final api = HttpNewsApi(client: httpClient, baseUrl: apiBaseUrl);
    final getNews = GetNewsImpl(api);
    return NewsViewModel(getNews);
  }

  /// Creates a [SubscriptionApi] for backend payment verification.
  SubscriptionApi createSubscriptionApi() {
    return HttpSubscriptionApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a [BillingManager] wired to the backend API.
  BillingManager createBillingManager({
    required String userId,
    TrialManager? trialManager,
  }) {
    return BillingManager(
      api: createSubscriptionApi(),
      userId: userId,
      trialManager: trialManager,
    );
  }
}
