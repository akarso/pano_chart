import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/chart/indicators.dart';

void main() {
  group('computeBehaviorIndicators', () {
    test('returns NaN-padded lists for short input', () {
      final result = computeBehaviorIndicators([], [], [], [], 20);
      expect(result.greed, isEmpty);
      expect(result.fear, isEmpty);
    });

    test('returns NaN for warmup indices', () {
      const n = 30;
      const window = 20;
      final closes = List.generate(n, (i) => 100.0 + i * 0.5);
      final highs = closes.map((c) => c + 2).toList();
      final lows = closes.map((c) => c - 2).toList();
      final volumes = List.filled(n, 1000.0);

      final result =
          computeBehaviorIndicators(closes, highs, lows, volumes, window);

      expect(result.greed.length, n);
      // First window-1 entries should be NaN.
      for (var i = 0; i < window - 1; i++) {
        expect(result.greed[i].isNaN, true, reason: 'greed[$i] should be NaN');
        expect(result.fear[i].isNaN, true, reason: 'fear[$i] should be NaN');
        expect(result.patience[i].isNaN, true,
            reason: 'patience[$i] should be NaN');
        expect(result.panic[i].isNaN, true, reason: 'panic[$i] should be NaN');
      }
      // From window-1 onward they should be valid.
      expect(result.greed[window - 1].isNaN, false);
    });

    test('all values are between 0 and 100', () {
      // Volatile series to exercise all code paths.
      final closes = <double>[
        100, 102, 99, 103, 97, 105, 101, 98, 104, 100,
        106, 99, 107, 95, 108, 102, 99, 110, 105, 103,
        120, 85, 130, 80, 100, 115, 90, 110, 95, 105,
      ].map((e) => e.toDouble()).toList();
      final highs = closes.map((c) => c + 3).toList();
      final lows = closes.map((c) => c - 3).toList();
      final volumes = List.generate(
          closes.length, (i) => 500.0 + (i % 5) * 200);

      final result =
          computeBehaviorIndicators(closes, highs, lows, volumes, 10);

      for (var i = 9; i < closes.length; i++) {
        expect(result.greed[i], greaterThanOrEqualTo(0),
            reason: 'greed[$i]');
        expect(result.greed[i], lessThanOrEqualTo(100),
            reason: 'greed[$i]');
        expect(result.fear[i], greaterThanOrEqualTo(0),
            reason: 'fear[$i]');
        expect(result.fear[i], lessThanOrEqualTo(100),
            reason: 'fear[$i]');
        expect(result.patience[i], greaterThanOrEqualTo(0),
            reason: 'patience[$i]');
        expect(result.patience[i], lessThanOrEqualTo(100),
            reason: 'patience[$i]');
        expect(result.panic[i], greaterThanOrEqualTo(0),
            reason: 'panic[$i]');
        expect(result.panic[i], lessThanOrEqualTo(100),
            reason: 'panic[$i]');
      }
    });

    test('monotonically rising prices produce greed > fear', () {
      const n = 40;
      // Strong upward trend: each candle closes higher.
      final closes = List.generate(n, (i) => 100.0 + i * 2.0);
      final highs = closes.map((c) => c + 1).toList();
      final lows = closes.map((c) => c - 1).toList();
      final volumes = List.filled(n, 1000.0);

      final result =
          computeBehaviorIndicators(closes, highs, lows, volumes, 20);

      // At the end of a strong trend, greed should exceed fear.
      final last = n - 1;
      expect(result.greed[last], greaterThan(result.fear[last]),
          reason: 'greed should exceed fear in uptrend');
    });

    test('high volatility increases fear and panic', () {
      const n = 40;
      // Wild swings: alternating +10/-10.
      final closes = List.generate(n, (i) => i.isEven ? 100.0 : 120.0);
      final highs = closes.map((c) => c + 5).toList();
      final lows = closes.map((c) => c - 5).toList();
      final volumes = List.filled(n, 1000.0);

      final volatile =
          computeBehaviorIndicators(closes, highs, lows, volumes, 20);

      // Calm series for comparison.
      final calmCloses = List.filled(n, 100.0);
      final calmHighs = calmCloses.map((c) => c + 0.5).toList();
      final calmLows = calmCloses.map((c) => c - 0.5).toList();
      final calm = computeBehaviorIndicators(
          calmCloses, calmHighs, calmLows, volumes, 20);

      final last = n - 1;
      expect(volatile.fear[last], greaterThan(calm.fear[last]),
          reason: 'volatile fear > calm fear');
      expect(volatile.panic[last], greaterThan(calm.panic[last]),
          reason: 'volatile panic > calm panic');
    });

    test('calm market produces higher patience', () {
      const n = 40;
      // Calm/compressed: tiny range.
      final closes = List.generate(n, (i) => 100.0 + i * 0.001);
      final highs = closes.map((c) => c + 0.01).toList();
      final lows = closes.map((c) => c - 0.01).toList();
      final volumes = List.filled(n, 1000.0);

      final calm =
          computeBehaviorIndicators(closes, highs, lows, volumes, 20);

      // Choppy series.
      final choppyCloses = List.generate(n, (i) => 100.0 + (i.isOdd ? 5 : -5));
      final choppyHighs = choppyCloses.map((c) => c + 3).toList();
      final choppyLows = choppyCloses.map((c) => c - 3).toList();
      final choppy = computeBehaviorIndicators(
          choppyCloses, choppyHighs, choppyLows, volumes, 20);

      final last = n - 1;
      expect(calm.patience[last], greaterThan(choppy.patience[last]),
          reason: 'calm patience > choppy patience');
    });

    test('window=2 minimal case still produces valid output', () {
      final closes = [100.0, 110.0, 105.0];
      final highs = [102.0, 112.0, 107.0];
      final lows = [98.0, 108.0, 103.0];
      final volumes = [500.0, 600.0, 550.0];

      final result =
          computeBehaviorIndicators(closes, highs, lows, volumes, 2);

      expect(result.greed[0].isNaN, true);
      expect(result.greed[1].isNaN, false);
      expect(result.greed[2].isNaN, false);
    });

    test('soft-normalise caps total to 150', () {
      // The backend caps g+f+p+pa at 1.5 (150 on 0–100 scale).
      const n = 40;
      final closes = List.generate(n, (i) => 100.0 + i * 2.0);
      final highs = closes.map((c) => c + 5).toList();
      final lows = closes.map((c) => c - 5).toList();
      final volumes = List.generate(n, (i) => 500.0 + i * 100);

      final result =
          computeBehaviorIndicators(closes, highs, lows, volumes, 10);

      for (var i = 9; i < n; i++) {
        final total = result.greed[i] +
            result.fear[i] +
            result.patience[i] +
            result.panic[i];
        expect(total, lessThanOrEqualTo(150.01),
            reason: 'total at $i = $total');
      }
    });
  });
}
