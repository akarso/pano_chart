import '../candles/api/candle_response.dart';
import 'volatility_model.dart';

/// Maps visible candles to their corresponding [VolatilityBucket] from the
/// full 1440-entry intraday profile.
///
/// Returns one bucket per candle, looked up by minute-of-day.  If no bucket
/// matches (empty data), a neutral bucket (normalized = 1.0) is
/// substituted.
List<VolatilityBucket> alignBucketsToCandles({
  required List<CandleDto> candles,
  required Map<int, VolatilityBucket> bucketsByMinute,
}) {
  const neutral = VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0);

  return candles.map((c) {
    final m = c.timestamp.hour * 60 + c.timestamp.minute;
    return bucketsByMinute[m] ?? neutral;
  }).toList();
}

/// Builds a lookup map from a list of [VolatilityBucket].
Map<int, VolatilityBucket> buildBucketLookup(List<VolatilityBucket> buckets) {
  return {for (final b in buckets) b.minute: b};
}
