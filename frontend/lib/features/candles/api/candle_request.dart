/// Request object mapping 1:1 to the `/api/v1/candles` query parameters.
class CandleRequest {
  final String symbol;
  final String timeframe;
  final DateTime from;
  final DateTime to;

  /// All fields required. `from` and `to` must be UTC and `from` < `to`.
  // ignore: prefer_initializing_formals
  CandleRequest(
      {required String symbol,
      required String timeframe,
      required this.from,
      required this.to})
      : symbol = symbol.trim(),
        timeframe = timeframe.trim() {
    if (this.symbol.isEmpty) {
      throw ArgumentError.value(symbol, 'symbol', 'symbol is required');
    }
    if (this.timeframe.isEmpty) {
      throw ArgumentError.value(
          timeframe, 'timeframe', 'timeframe is required');
    }
    if (!from.isUtc) {
      throw ArgumentError.value(from, 'from', 'from must be UTC');
    }
    if (!to.isUtc) {
      throw ArgumentError.value(to, 'to', 'to must be UTC');
    }
    if (!from.isBefore(to)) {
      throw ArgumentError.value(from, 'from', 'from must be before to');
    }
  }
}
