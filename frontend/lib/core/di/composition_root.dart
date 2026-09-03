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
import '../../features/market_state/http_regime_api.dart';
import '../../features/market_state/http_regime_history_api.dart';
import '../../features/market_state/http_transition_api.dart';
import '../../features/detail/http_fragility_api.dart';
import '../../features/detail/http_behavior_api.dart';
import '../../features/detail/http_setup_api.dart';
import '../../features/volatility/http_volatility_api.dart';
import '../../features/news/api/news_api.dart';
import '../../features/news/application/get_news.dart';
import '../../features/news/infrastructure/http_news_api.dart';
import '../../features/news/news_view_model.dart';
import '../../features/notifications/api/notification_config_api.dart';
import '../../features/notifications/infrastructure/http_notification_config_api.dart';
import '../../features/overview/get_rankings_impl.dart';
import '../../features/overview/http_rankings_api.dart';
import '../../features/overview/overview_view_model.dart';
import '../../features/social/api/device_registration_api.dart';
import '../../features/social/api/social_api.dart';
import '../../features/social/infrastructure/http_device_registration_api.dart';
import '../../features/social/infrastructure/http_social_api.dart';
import '../../features/social/social_feed_view_model.dart';

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
  ///
  /// Eagerly loads all symbols in a single request (≈150 + stablecoin
  /// padding) so the full universe is available immediately at startup.
  OverviewViewModel createOverviewViewModel() {
    final api = HttpRankingsApi(baseUrl: apiBaseUrl, client: httpClient);
    final getRankings = GetRankingsImpl(api, pageSize: 150 + stablecoinPadding);
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

  /// Creates a wired RegimeApi.
  RegimeApi createRegimeApi() {
    return HttpRegimeApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired TransitionApi.
  TransitionApi createTransitionApi() {
    return HttpTransitionApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired RegimeHistoryApi.
  RegimeHistoryApi createRegimeHistoryApi() {
    return HttpRegimeHistoryApi(client: httpClient, baseUrl: apiBaseUrl);
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

  /// Creates a wired [SetupApi] for fetching setup quality scores.
  SetupApi createSetupApi() {
    return HttpSetupApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [FragilityApi] for fetching position crowding scores.
  FragilityApi createFragilityApi() {
    return HttpFragilityApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [BehaviorApi] for fetching retail behavior scores.
  BehaviorApi createBehaviorApi() {
    return HttpBehaviorApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [VolatilityApi] for fetching intraday activity profiles.
  VolatilityApi createVolatilityApi() {
    return HttpVolatilityApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [SocialApi].
  SocialApi createSocialApi() {
    return HttpSocialApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [DeviceRegistrationApi].
  DeviceRegistrationApi createDeviceRegistrationApi() {
    return HttpDeviceRegistrationApi(client: httpClient, baseUrl: apiBaseUrl);
  }

  /// Creates a wired [SocialFeedViewModel].
  SocialFeedViewModel createSocialFeedViewModel({required String userId}) {
    final api = createSocialApi();
    return SocialFeedViewModel(api, userId: userId);
  }

  /// Creates a wired [NotificationConfigApi].
  NotificationConfigApi createNotificationConfigApi() {
    return HttpNotificationConfigApi(client: httpClient, baseUrl: apiBaseUrl);
  }
}
