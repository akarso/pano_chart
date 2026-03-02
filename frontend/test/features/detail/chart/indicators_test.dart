import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/chart/indicators.dart';

void main() {
  group('computeEma', () {
    test('returns empty for empty input', () {
      expect(computeEma([], 3), isEmpty);
    });

    test('returns NaN padding for period-1 indices then SMA seed', () {
      // 3-period EMA on [2, 4, 6] → first 2 are NaN, third is SMA(2,4,6)=4
      final result = computeEma([2, 4, 6], 3);
      expect(result.length, 3);
      expect(result[0].isNaN, true);
      expect(result[1].isNaN, true);
      expect(result[2], closeTo(4.0, 1e-9));
    });

    test('warm-up pads correctly for period > length', () {
      final result = computeEma([10, 20], 5);
      expect(result.length, 2);
      expect(result[0].isNaN, true);
      expect(result[1].isNaN, true);
    });

    test('known EMA values for 3-period', () {
      // Values: [10, 11, 12, 13, 14]
      // SMA(10,11,12) = 11 → EMA[2] = 11
      // k = 2/(3+1) = 0.5
      // EMA[3] = 13*0.5 + 11*0.5 = 12
      // EMA[4] = 14*0.5 + 12*0.5 = 13
      final result = computeEma([10, 11, 12, 13, 14], 3);
      expect(result[2], closeTo(11.0, 1e-9));
      expect(result[3], closeTo(12.0, 1e-9));
      expect(result[4], closeTo(13.0, 1e-9));
    });

    test('single element with period 1 returns same value', () {
      final result = computeEma([42.0], 1);
      expect(result.length, 1);
      expect(result[0], closeTo(42.0, 1e-9));
    });

    test('constant values yield constant EMA', () {
      final result = computeEma(List.filled(10, 5.0), 3);
      for (var i = 2; i < 10; i++) {
        expect(result[i], closeTo(5.0, 1e-9));
      }
    });
  });

  group('computeRsi', () {
    test('returns empty for empty input', () {
      expect(computeRsi([], 14), isEmpty);
    });

    test('returns NaN for first period indices', () {
      final result = computeRsi(List.generate(20, (i) => i * 1.0), 14);
      for (var i = 0; i < 14; i++) {
        expect(result[i].isNaN, true, reason: 'index $i should be NaN');
      }
      // Index 14 is the first valid RSI value
      expect(result[14].isNaN, false);
    });

    test('monotonically increasing prices → RSI ≈ 100', () {
      // Strictly increasing: all gains, no losses
      final prices = List.generate(30, (i) => 100.0 + i);
      final result = computeRsi(prices, 14);
      final last = result.last;
      expect(last, closeTo(100.0, 0.01));
    });

    test('monotonically decreasing prices → RSI ≈ 0', () {
      final prices = List.generate(30, (i) => 100.0 - i);
      final result = computeRsi(prices, 14);
      final last = result.last;
      expect(last, closeTo(0.0, 0.01));
    });

    test('alternating prices → RSI near 50', () {
      // Same-magnitude up/down moves
      final prices = <double>[];
      for (var i = 0; i < 40; i++) {
        prices.add(i.isEven ? 100.0 : 110.0);
      }
      final result = computeRsi(prices, 14);
      final last = result.last;
      expect(last, closeTo(50.0, 5.0)); // roughly mid-range
    });

    test('RSI values are between 0 and 100', () {
      final prices = [
        100, 102, 99, 103, 97, 105, 101, 98, 104, 100,
        106, 99, 107, 95, 108, 102, 99, 110, 105, 103,
      ].map((e) => e.toDouble()).toList();
      final result = computeRsi(prices, 5);
      for (var i = 6; i < result.length; i++) {
        if (!result[i].isNaN) {
          expect(result[i], greaterThanOrEqualTo(0.0));
          expect(result[i], lessThanOrEqualTo(100.0));
        }
      }
    });
  });

  group('computeAtr', () {
    test('returns empty for empty input', () {
      expect(computeAtr([], [], [], 14), isEmpty);
    });

    test('returns NaN for first period indices', () {
      final n = 20;
      final highs = List.generate(n, (i) => 110.0 + i);
      final lows = List.generate(n, (i) => 90.0 + i);
      final closes = List.generate(n, (i) => 100.0 + i);
      final result = computeAtr(highs, lows, closes, 5);
      // First index is always NaN (no previous close)
      expect(result[0].isNaN, true);
      // Indices 1..4 are NaN (warm-up)
      for (var i = 1; i < 5; i++) {
        expect(result[i].isNaN, true, reason: 'index $i should be NaN');
      }
    });

    test('constant range produces constant ATR', () {
      // H-L always 10, no gaps → TR always 10 → ATR always 10
      final n = 20;
      final highs = List.filled(n, 110.0);
      final lows = List.filled(n, 100.0);
      final closes = List.filled(n, 105.0);
      final result = computeAtr(highs, lows, closes, 5);
      for (var i = 5; i < n; i++) {
        expect(result[i], closeTo(10.0, 0.01));
      }
    });

    test('ATR values are positive', () {
      final prices = [
        100, 102, 99, 103, 97, 105, 101, 98, 104, 100,
        106, 99, 107, 95, 108, 102, 99, 110, 105, 103,
      ].map((e) => e.toDouble()).toList();
      final highs = prices.map((p) => p + 3).toList();
      final lows = prices.map((p) => p - 2).toList();
      final result = computeAtr(highs, lows, prices, 5);
      for (var i = 5; i < result.length; i++) {
        if (!result[i].isNaN) {
          expect(result[i], greaterThan(0.0));
        }
      }
    });

    test('known ATR computation for period 2', () {
      // Candle 0: H=12, L=8, C=10 → TR=4 (no prev close)
      // Candle 1: H=14, L=9, C=11 → TR=max(5, |14-10|, |9-10|)=max(5,4,1)=5
      // Candle 2: H=13, L=10, C=12 → TR=max(3, |13-11|, |10-11|)=max(3,2,1)=3
      // ATR[2] = SMA of TR[1..2] = (5+3)/2 = 4
      // (first ATR after warm-up at index = period)
      final result = computeAtr(
        [12.0, 14.0, 13.0],
        [8.0, 9.0, 10.0],
        [10.0, 11.0, 12.0],
        2,
      );
      expect(result[0].isNaN, true);
      expect(result[1].isNaN, true);
      expect(result[2], closeTo(4.0, 1e-9));
    });
  });
}
