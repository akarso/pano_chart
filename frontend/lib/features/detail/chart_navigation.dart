import '../candles/application/get_candle_series_input.dart';

/// Shared constants and helpers for navigating to the detail chart.
///
/// Both the overview list and the bubble map must fetch identical candle
/// windows so the detail chart always has the same depth and indicator
/// accuracy regardless of entry point.

/// Number of candles the sparkline covers — used as `initialVisibleCount`.
const int kSparklineCandles = 30;

/// Extra candles fetched purely for indicator warmup (EMA, RSI, ATR).
/// These candles are hidden — the user cannot scroll past them.
const int kIndicatorWarmup = 50;

/// Total *visible* chart candles (scrollable).
const int kChartCandles = 600;

/// Returns the [Duration] of a single candle for the given timeframe string.
Duration candleDuration(String tf) {
  switch (tf) {
    case '1m':
      return const Duration(minutes: 1);
    case '5m':
      return const Duration(minutes: 5);
    case '15m':
      return const Duration(minutes: 15);
    case '1h':
      return const Duration(hours: 1);
    case '4h':
      return const Duration(hours: 4);
    case '1d':
      return const Duration(days: 1);
    default:
      return const Duration(hours: 1);
  }
}

/// Builds a [GetCandleSeriesInput] for the detail chart.
///
/// Fetches [kChartCandles] + [kIndicatorWarmup] candles at [timeframe],
/// ending at [now].
GetCandleSeriesInput buildDetailChartInput({
  required String symbol,
  required String timeframe,
  DateTime? now,
}) {
  final end = now ?? DateTime.now().toUtc();
  final totalCandles = kChartCandles + kIndicatorWarmup;
  final from = end.subtract(candleDuration(timeframe) * totalCandles);
  return GetCandleSeriesInput(
    symbol: symbol,
    timeframe: timeframe,
    from: from,
    to: end,
  );
}
