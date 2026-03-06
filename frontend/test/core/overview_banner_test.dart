import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/overview_banner.dart';

void main() {
  group('isDataStale', () {
    test('returns false when within threshold', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 1));
      expect(isDataStale('1m', loadedAt, now), isFalse);
    });

    test('returns true when past threshold for 1m', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 1, seconds: 31));
      expect(isDataStale('1m', loadedAt, now), isTrue);
    });

    test('returns false at exact threshold boundary for 5m', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 2, seconds: 29));
      expect(isDataStale('5m', loadedAt, now), isFalse);
    });

    test('returns true past threshold for 5m', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 2, seconds: 30));
      expect(isDataStale('5m', loadedAt, now), isTrue);
    });

    test('returns true for 15m after 5min', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 5));
      expect(isDataStale('15m', loadedAt, now), isTrue);
    });

    test('returns true for 1h after 10min', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 10));
      expect(isDataStale('1h', loadedAt, now), isTrue);
    });

    test('returns false for 1h before 10min', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 9, seconds: 59));
      expect(isDataStale('1h', loadedAt, now), isFalse);
    });

    test('returns true for 4h after 1 hour', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(hours: 1));
      expect(isDataStale('4h', loadedAt, now), isTrue);
    });

    test('returns true for 1d after 1 hour', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(hours: 1));
      expect(isDataStale('1d', loadedAt, now), isTrue);
    });

    test('unknown timeframe defaults to 5 minutes', () {
      final now = DateTime(2026, 3, 6, 12, 0);
      final loadedAt = now.subtract(const Duration(minutes: 5));
      expect(isDataStale('99x', loadedAt, now), isTrue);
    });
  });

  group('StalenessTracker', () {
    test('initial kind is none', () {
      final tracker = StalenessTracker();
      expect(tracker.kind, OverviewBannerKind.none);
      tracker.stop();
    });

    test('markOffline sets kind to offline', () {
      final tracker = StalenessTracker();
      OverviewBannerKind? notified;
      tracker.onChanged = () => notified = tracker.kind;
      tracker.markOffline();
      expect(tracker.kind, OverviewBannerKind.offline);
      expect(notified, OverviewBannerKind.offline);
      tracker.stop();
    });

    test('markOnline after markOffline sets kind to none', () {
      final tracker = StalenessTracker();
      tracker.markOffline();
      expect(tracker.kind, OverviewBannerKind.offline);
      tracker.markOnline();
      expect(tracker.kind, OverviewBannerKind.none);
      tracker.stop();
    });

    test('offline takes priority over stale', () {
      final tracker = StalenessTracker();
      tracker.markLoaded();
      // Even if data becomes stale, offline should take priority.
      tracker.markOffline();
      expect(tracker.kind, OverviewBannerKind.offline);
      tracker.stop();
    });
  });
}
