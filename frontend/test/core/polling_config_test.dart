import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/polling_config.dart';

void main() {
  group('polling_config', () {
    group('overviewAutoRefreshInterval', () {
      test('uses formula: stagger * count + margin', () {
        // 150 symbols → 20ms * 150 + 5000ms = 8000ms
        final interval = overviewAutoRefreshInterval(150);
        expect(interval.inMilliseconds, 8000);
      });

      test('returns margin only when count is 0', () {
        final interval = overviewAutoRefreshInterval(0);
        expect(interval.inMilliseconds, kStaggerMarginMs);
      });

      test('small count produces short interval', () {
        final interval = overviewAutoRefreshInterval(10);
        // 20 * 10 + 5000 = 5200
        expect(interval.inMilliseconds, 5200);
      });
    });

    group('kChartRefreshIntervals', () {
      test('1m maps to 10 seconds', () {
        expect(kChartRefreshIntervals['1m'], const Duration(seconds: 10));
      });

      test('5m maps to 1 minute', () {
        expect(kChartRefreshIntervals['5m'], const Duration(minutes: 1));
      });

      test('15m maps to 3 minutes', () {
        expect(kChartRefreshIntervals['15m'], const Duration(minutes: 3));
      });

      test('1h maps to 10 minutes', () {
        expect(kChartRefreshIntervals['1h'], const Duration(minutes: 10));
      });

      test('4h maps to 15 minutes', () {
        expect(kChartRefreshIntervals['4h'], const Duration(minutes: 15));
      });

      test('1d maps to 1 hour', () {
        expect(kChartRefreshIntervals['1d'], const Duration(hours: 1));
      });

      test('covers all 6 timeframes', () {
        expect(kChartRefreshIntervals.keys.toSet(),
            {'1m', '5m', '15m', '1h', '4h', '1d'});
      });
    });

    group('constants', () {
      test('kStaggerDelayMs is 20', () {
        expect(kStaggerDelayMs, 20);
      });

      test('kStaggerMarginMs is 5000', () {
        expect(kStaggerMarginMs, 5000);
      });

      test('kMacroEventsRefreshDuration is 15 minutes', () {
        expect(kMacroEventsRefreshDuration, const Duration(minutes: 15));
      });
    });
  });
}
