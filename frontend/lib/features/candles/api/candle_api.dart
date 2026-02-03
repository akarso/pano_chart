import 'candle_request.dart';
import 'candle_response.dart';

/// Client port (interface) for fetching candle series.
/// No implementation or side-effects in this PR — transports come later.
abstract class CandleApi {
  /// Fetch candle series described by [request].
  /// Implementations must be asynchronous and side-effect free from the caller's perspective.
  Future<CandleSeriesResponse> fetchCandles(CandleRequest request);
}
