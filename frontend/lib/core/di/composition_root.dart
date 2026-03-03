import 'package:http/http.dart' as http;

import '../../features/bubble_map/bubble_map_view_model.dart';
import '../../features/candles/application/get_candle_series.dart';
import '../../features/candles/application/get_candle_series.dart' as impl;
import '../../features/candles/infrastructure/http_candle_api.dart';
import '../../features/events/api/events_api.dart';
import '../../features/events/application/get_events.dart';
import '../../features/events/events_view_model.dart';
import '../../features/events/infrastructure/http_events_api.dart';
import '../../features/fear_greed/http_fear_greed_api.dart';
import '../../features/overview/get_rankings_impl.dart';
import '../../features/overview/http_rankings_api.dart';
import '../../features/overview/overview_view_model.dart';

/// Composition root responsible for explicitly wiring dependencies.
class CompositionRoot {
  final String apiBaseUrl;
  final http.Client httpClient;

  CompositionRoot({required this.apiBaseUrl, http.Client? httpClient})
      : httpClient = httpClient ?? http.Client();

  /// Creates a wired GetCandleSeries use case instance.
  GetCandleSeries createGetCandleSeries() {
    final api = HttpCandleApi(baseUrl: apiBaseUrl, client: httpClient);
    return impl.GetCandleSeriesImpl(api);
  }

  /// Creates a wired OverviewViewModel backed by the rankings API.
  OverviewViewModel createOverviewViewModel() {
    final api = HttpRankingsApi(baseUrl: apiBaseUrl, client: httpClient);
    final getRankings = GetRankingsImpl(api);
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
}
