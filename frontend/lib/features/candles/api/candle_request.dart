/// Request object mapping 1:1 to the `/api/v1/candles` query parameters.
class CandleRequest {
  final String symbol;
  final String timeframe;
  final DateTime from;
  final DateTime to;

  /// All fields required. `from` and `to` must be UTC and `from` < `to`.
  CandleRequest(
      {required String symbol,
      required String timeframe,
      required DateTime from,
      required DateTime to})
      : symbol = symbol.trim(),
        timeframe = timeframe.trim(),
        from = from,
        to = to {
    if (this.symbol.isEmpty)
      throw ArgumentError.value(symbol, 'symbol', 'symbol is required');
    if (this.timeframe.isEmpty)
      throw ArgumentError.value(
          timeframe, 'timeframe', 'timeframe is required');
    if (!this.from.isUtc)
      throw ArgumentError.value(from, 'from', 'from must be UTC');
    if (!this.to.isUtc) throw ArgumentError.value(to, 'to', 'to must be UTC');
    if (!this.from.isBefore(this.to))
      throw ArgumentError.value(from, 'from', 'from must be before to');
  }
}
