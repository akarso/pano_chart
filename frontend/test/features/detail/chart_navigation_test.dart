import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/chart_navigation.dart';

void main() {
  group('candleDuration', () {
    test('returns correct durations for all known timeframes', () {
      expect(candleDuration('1m'), const Duration(minutes: 1));
      expect(candleDuration('5m'), const Duration(minutes: 5));
      expect(candleDuration('15m'), const Duration(minutes: 15));
      expect(candleDuration('1h'), const Duration(hours: 1));
      expect(candleDuration('4h'), const Duration(hours: 4));
      expect(candleDuration('1d'), const Duration(days: 1));
    });

    test('defaults to 1h for unknown timeframe', () {
      expect(candleDuration('3d'), const Duration(hours: 1));
      expect(candleDuration(''), const Duration(hours: 1));
    });
  });

  group('shared constants', () {
    test('kSparklineCandles is 30', () {
      expect(kSparklineCandles, 30);
    });

    test('kIndicatorWarmup is 50', () {
      expect(kIndicatorWarmup, 50);
    });

    test('kChartCandles is 600', () {
      expect(kChartCandles, 600);
    });

    test('kTimeframes contains the six supported timeframes', () {
      expect(kTimeframes, ['1m', '5m', '15m', '1h', '4h', '1d']);
    });
  });

  group('buildDetailChartInput', () {
    test('returns input with correct symbol and timeframe', () {
      final input = buildDetailChartInput(
        symbol: 'BTCUSDT',
        timeframe: '1h',
      );
      expect(input.symbol, 'BTCUSDT');
      expect(input.timeframe, '1h');
    });

    test('fetches kChartCandles + kIndicatorWarmup candles', () {
      final now = DateTime.utc(2026, 3, 1, 12, 0);
      final input = buildDetailChartInput(
        symbol: 'ETHUSDT',
        timeframe: '1h',
        now: now,
      );

      // 650 1h candles = 650 hours
      final expectedFrom = now.subtract(const Duration(hours: 650));
      expect(input.from, expectedFrom);
      expect(input.to, now);
    });

    test('1m timeframe yields correct time window', () {
      final now = DateTime.utc(2026, 3, 1, 12, 0);
      final input = buildDetailChartInput(
        symbol: 'X',
        timeframe: '1m',
        now: now,
      );

      final expectedFrom = now.subtract(const Duration(minutes: 650));
      expect(input.from, expectedFrom);
    });

    test('4h timeframe yields correct time window', () {
      final now = DateTime.utc(2026, 3, 1, 12, 0);
      final input = buildDetailChartInput(
        symbol: 'X',
        timeframe: '4h',
        now: now,
      );

      final expectedFrom = now.subtract(const Duration(hours: 4 * 650));
      expect(input.from, expectedFrom);
    });

    test('1d timeframe yields correct time window', () {
      final now = DateTime.utc(2026, 3, 1, 12, 0);
      final input = buildDetailChartInput(
        symbol: 'X',
        timeframe: '1d',
        now: now,
      );

      final expectedFrom = now.subtract(const Duration(days: 650));
      expect(input.from, expectedFrom);
    });

    test('uses current time when now is not provided', () {
      final before = DateTime.now().toUtc();
      final input = buildDetailChartInput(
        symbol: 'X',
        timeframe: '1h',
      );
      final after = DateTime.now().toUtc();

      // input.to should be between before and after
      expect(
        input.to.isAfter(before) || input.to.isAtSameMomentAs(before),
        isTrue,
      );
      expect(
        input.to.isBefore(after) || input.to.isAtSameMomentAs(after),
        isTrue,
      );
    });
  });
}
