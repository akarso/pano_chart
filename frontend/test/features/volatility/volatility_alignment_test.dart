import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_alignment.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';

void main() {
  group('buildBucketLookup', () {
    test('creates map keyed by minute', () {
      final buckets = [
        const VolatilityBucket(minute: 0, normalized: 1.2, spikeProb: 0.1),
        const VolatilityBucket(minute: 60, normalized: 0.8, spikeProb: 0.5),
        const VolatilityBucket(minute: 120, normalized: 1.0, spikeProb: 0.0),
      ];
      final map = buildBucketLookup(buckets);
      expect(map.length, 3);
      expect(map[0]!.normalized, 1.2);
      expect(map[60]!.spikeProb, 0.5);
      expect(map[120]!.minute, 120);
      expect(map[30], isNull);
    });

    test('empty list produces empty map', () {
      expect(buildBucketLookup([]), isEmpty);
    });
  });

  group('alignBucketsToCandles', () {
    CandleDto _candle(DateTime ts) => CandleDto(
          timestamp: ts,
          open: 1,
          high: 2,
          low: 0.5,
          close: 1.5,
          volume: 100,
        );

    test('maps candle timestamps to buckets by minute-of-day', () {
      final buckets = buildBucketLookup([
        const VolatilityBucket(minute: 600, normalized: 1.3, spikeProb: 0.2),
        const VolatilityBucket(minute: 660, normalized: 0.7, spikeProb: 0.8),
      ]);

      final candles = [
        _candle(DateTime.utc(2024, 1, 1, 10, 0)), // minute 600
        _candle(DateTime.utc(2024, 1, 1, 11, 0)), // minute 660
      ];

      final result = alignBucketsToCandles(
        candles: candles,
        bucketsByMinute: buckets,
      );

      expect(result.length, 2);
      expect(result[0].normalized, 1.3);
      expect(result[1].spikeProb, 0.8);
    });

    test('returns neutral bucket for unmatched minute', () {
      final candles = [
        _candle(DateTime.utc(2024, 1, 1, 5, 30)), // minute 330
      ];

      final result = alignBucketsToCandles(
        candles: candles,
        bucketsByMinute: {}, // no buckets at all
      );

      expect(result.length, 1);
      expect(result[0].normalized, 1.0);
      expect(result[0].spikeProb, 0.0);
    });

    test('empty candles returns empty list', () {
      final result = alignBucketsToCandles(
        candles: [],
        bucketsByMinute: {0: const VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)},
      );
      expect(result, isEmpty);
    });

    test('multiple candles at same minute-of-day get same bucket', () {
      final bucket = const VolatilityBucket(minute: 720, normalized: 2.0, spikeProb: 0.9);
      final lookup = buildBucketLookup([bucket]);

      final candles = [
        _candle(DateTime.utc(2024, 1, 1, 12, 0)), // minute 720, day 1
        _candle(DateTime.utc(2024, 1, 2, 12, 0)), // minute 720, day 2
      ];

      final result = alignBucketsToCandles(
        candles: candles,
        bucketsByMinute: lookup,
      );

      expect(result[0].normalized, 2.0);
      expect(result[1].normalized, 2.0);
    });
  });
}
