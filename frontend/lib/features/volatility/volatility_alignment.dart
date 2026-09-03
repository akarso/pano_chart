import '../candles/api/candle_response.dart';
import 'volatility_model.dart';

/// Maps candles to their corresponding [VolatilityBucket] from the
/// volatility profile, using the user's **local** timezone.
///
/// For intraday timeframes the lookup key is minute-of-day (0–1439).
/// For the 1d timeframe the lookup key is day-of-week where
/// Sunday = 0. For daily candles the Dart `DateTime.weekday`
/// (Monday=1 … Sunday=7) is mapped to the Go convention (Sunday=0 … Saturday=6).
///
/// Returns one entry per candle (null when no bucket matches).
List<VolatilityBucket?> alignBucketsToCandles({
  required List<CandleDto> candles,
  required Map<int, VolatilityBucket> bucketsByMinute,
  bool isDailyTimeframe = false,
}) {
  return candles.map((c) {
    final local = c.timestamp.toLocal();
    if (isDailyTimeframe) {
      // Dart weekday: Monday=1 … Sunday=7
      // Backend day-of-week: Sunday=0 … Saturday=6
      final dow = local.weekday % 7; // Mon=1,…,Sat=6,Sun=7→0
      return bucketsByMinute[dow];
    }
    final m = local.hour * 60 + local.minute;
    return bucketsByMinute[m];
  }).toList();
}

/// Builds a lookup map from a list of [VolatilityBucket].
Map<int, VolatilityBucket> buildBucketLookup(List<VolatilityBucket> buckets) {
  return {for (final b in buckets) b.minute: b};
}
