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

    test('maps candle timestamps to buckets by local minute-of-day', () {
      // Create a candle, convert to local to find its local minute,
      // then verify alignment matches that bucket.
      final ts = DateTime.utc(2024, 1, 1, 10, 0);
      final localTs = ts.toLocal();
      final localMinute = localTs.hour * 60 + localTs.minute;

      final buckets = buildBucketLookup([
        VolatilityBucket(minute: localMinute, normalized: 1.3, spikeProb: 0.2),
      ]);

      final result = alignBucketsToCandles(
        candles: [_candle(ts)],
        bucketsByMinute: buckets,
      );

      expect(result.length, 1);
      expect(result[0], isNotNull);
      expect(result[0]!.normalized, 1.3);
      expect(result[0]!.spikeProb, 0.2);
    });

    test('returns null for unmatched minute', () {
      final candles = [
        _candle(DateTime.utc(2024, 1, 1, 5, 30)),
      ];

      final result = alignBucketsToCandles(
        candles: candles,
        bucketsByMinute: {}, // no buckets at all
      );

      expect(result.length, 1);
      expect(result[0], isNull);
    });

    test('empty candles returns empty list', () {
      final result = alignBucketsToCandles(
        candles: [],
        bucketsByMinute: {0: const VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)},
      );
      expect(result, isEmpty);
    });

    test('multiple candles at same local minute-of-day get same bucket', () {
      final ts1 = DateTime.utc(2024, 1, 1, 12, 0);
      final ts2 = DateTime.utc(2024, 1, 2, 12, 0);
      final localMinute = ts1.toLocal().hour * 60 + ts1.toLocal().minute;

      final bucket = VolatilityBucket(minute: localMinute, normalized: 2.0, spikeProb: 0.9);
      final lookup = buildBucketLookup([bucket]);

      final result = alignBucketsToCandles(
        candles: [_candle(ts1), _candle(ts2)],
        bucketsByMinute: lookup,
      );

      expect(result[0], isNotNull);
      expect(result[0]!.normalized, 2.0);
      expect(result[1], isNotNull);
      expect(result[1]!.normalized, 2.0);
    });

    test('uses local timezone, not UTC', () {
      // A candle at UTC midnight → local time may differ.
      final utcMidnight = DateTime.utc(2024, 6, 15, 0, 0);
      final localTs = utcMidnight.toLocal();
      final localMinute = localTs.hour * 60 + localTs.minute;

      // Bucket at the LOCAL minute should match.
      final bucket = VolatilityBucket(minute: localMinute, normalized: 1.5, spikeProb: 0.3);
      final lookup = buildBucketLookup([bucket]);

      final result = alignBucketsToCandles(
        candles: [_candle(utcMidnight)],
        bucketsByMinute: lookup,
      );

      expect(result[0], isNotNull);
      expect(result[0]!.normalized, 1.5);

      // A bucket at UTC minute 0 should NOT match (unless local == UTC).
      if (localMinute != 0) {
        final utcBucket = const VolatilityBucket(minute: 0, normalized: 9.9, spikeProb: 0.9);
        final utcLookup = buildBucketLookup([utcBucket]);
        final utcResult = alignBucketsToCandles(
          candles: [_candle(utcMidnight)],
          bucketsByMinute: utcLookup,
        );
        expect(utcResult[0], isNull);
      }
    });

    test('isDailyTimeframe aligns by day-of-week', () {
      // 2024-01-15 is a Monday → Dart weekday 1 → Go weekday 1
      final monday = DateTime.utc(2024, 1, 15, 0, 0);
      // 2024-01-21 is a Sunday → Dart weekday 7 → Go weekday 0
      final sunday = DateTime.utc(2024, 1, 21, 0, 0);

      final monBucket = const VolatilityBucket(minute: 1, normalized: 1.5, spikeProb: 0.3);
      final sunBucket = const VolatilityBucket(minute: 0, normalized: 0.7, spikeProb: 0.1);
      final lookup = buildBucketLookup([sunBucket, monBucket]);

      final result = alignBucketsToCandles(
        candles: [_candle(monday), _candle(sunday)],
        bucketsByMinute: lookup,
        isDailyTimeframe: true,
      );

      expect(result[0], isNotNull);
      expect(result[0]!.normalized, 1.5); // Monday bucket
      expect(result[1], isNotNull);
      expect(result[1]!.normalized, 0.7); // Sunday bucket
    });

    test('isDailyTimeframe returns null for missing day', () {
      // Only have Monday bucket (day 1), candle is on Tuesday (day 2)
      final tuesday = DateTime.utc(2024, 1, 16, 0, 0);
      final monBucket = const VolatilityBucket(minute: 1, normalized: 1.5, spikeProb: 0.3);
      final lookup = buildBucketLookup([monBucket]);

      final result = alignBucketsToCandles(
        candles: [_candle(tuesday)],
        bucketsByMinute: lookup,
        isDailyTimeframe: true,
      );

      expect(result[0], isNull);
    });
  });
}
