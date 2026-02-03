import '../api/candle_api.dart';
import '../api/candle_request.dart';
import '../api/candle_response.dart';
import 'get_candle_series_input.dart';

/// Use case interface for fetching candle series.
abstract class GetCandleSeries {
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input);
}

/// Implementation that delegates to the provided [CandleApi].
class GetCandleSeriesImpl implements GetCandleSeries {
  final CandleApi _api;

  GetCandleSeriesImpl(this._api);

  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    final req = CandleRequest(
        symbol: input.symbol,
        timeframe: input.timeframe,
        from: input.from,
        to: input.to);
    return _api.fetchCandles(req);
  }
}
