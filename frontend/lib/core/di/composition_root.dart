import 'package:http/http.dart' as http;

import '../../features/candles/application/get_candle_series.dart';
import '../../features/candles/application/get_candle_series.dart' as impl;
import '../../features/candles/infrastructure/http_candle_api.dart';
import '../../features/overview/get_overview_impl.dart';
import '../../features/overview/http_overview_api.dart';
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

  /// Creates a wired OverviewViewModel backed by the real HTTP adapter.
  OverviewViewModel createOverviewViewModel() {
    final api = HttpOverviewApi(baseUrl: apiBaseUrl, client: httpClient);
    final getOverview = GetOverviewImpl(api);
    return OverviewViewModel(getOverview);
  }
}
