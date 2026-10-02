import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/replay/replay_asof.dart';
import 'package:pano_chart_frontend/features/replay/replay_controller.dart';

void main() {
  group('alignAsOfToBar', () {
    test('floors to the open of the containing 4h bar', () {
      final raw = DateTime.utc(2025, 9, 16, 10, 17, 42);
      expect(alignAsOfToBar(raw, '4h'), DateTime.utc(2025, 9, 16, 8));
    });

    test('floors to the open of the containing 1h bar', () {
      final raw = DateTime.utc(2025, 9, 16, 10, 17);
      expect(alignAsOfToBar(raw, '1h'), DateTime.utc(2025, 9, 16, 10));
    });
  });

  group('clampAsOfToNow', () {
    test('does not advance past the open of the candle containing now', () {
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      final future = DateTime.utc(2025, 9, 17);
      expect(
        clampAsOfToNow(future, '1h', now: now),
        DateTime.utc(2025, 9, 16, 10),
      );
    });
  });

  group('asOfFor across timeframes', () {
    test('same instant re-aligns when the surface TF changes', () {
      final c = ReplayController(reloadDebounce: Duration.zero);
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      c.enter(timeframe: '1h', now: now);
      expect(c.asOfFor('1h', now: now), DateTime.utc(2025, 9, 16, 10));
      expect(c.asOfFor('4h', now: now), DateTime.utc(2025, 9, 16, 8));
      expect(
        c.asOfUnixFor('4h', now: now),
        DateTime.utc(2025, 9, 16, 8).millisecondsSinceEpoch ~/ 1000,
      );
    });

    test('setAsOf returns false when aligned unix is unchanged', () {
      final c = ReplayController(reloadDebounce: Duration.zero);
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      c.enter(timeframe: '4h', now: now);
      expect(c.setAsOf('1h', c.asOf!, now: now), isFalse);
      expect(c.asOfFor('1h', now: now), DateTime.utc(2025, 9, 16, 8));
    });
  });

  group('replayScorecardSince', () {
    test('is absolute RFC3339 and always ≤ asOf', () {
      final asOf = DateTime.utc(2025, 9, 16, 8);
      final since = DateTime.parse(replayScorecardSince(asOf));
      expect(since.isUtc, isTrue);
      expect(since.isBefore(asOf) || since.isAtSameMomentAs(asOf), isTrue);
      expect(asOf.difference(since), const Duration(days: 30));
    });
  });

  group('ReplayController', () {
    test('enter / stepBars / exit keep bar alignment', () {
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      final c = ReplayController(reloadDebounce: Duration.zero);
      c.enter(timeframe: '4h', now: now);
      expect(c.isActive, isTrue);
      expect(c.asOf, DateTime.utc(2025, 9, 16, 8));
      expect(
        c.asOfUnixFor('4h', now: now),
        DateTime.utc(2025, 9, 16, 8).millisecondsSinceEpoch ~/ 1000,
      );

      c.stepBars('4h', -1, now: now);
      expect(c.asOf, DateTime.utc(2025, 9, 16, 4));

      c.stepDays('4h', 1, now: now);
      expect(c.asOf, DateTime.utc(2025, 9, 16, 8));

      c.exit();
      expect(c.isActive, isFalse);
      expect(c.asOfUnixFor('4h'), isNull);
    });

    test('UI notifies immediately; reload listeners debounce scrub steps',
        () async {
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      final c = ReplayController(
        reloadDebounce: const Duration(milliseconds: 50),
      );
      var ui = 0;
      var reloads = 0;
      c.addListener(() => ui++);
      c.addReloadListener(() => reloads++);

      c.enter(timeframe: '1h', now: now);
      expect(ui, 1);
      expect(reloads, 1);

      c.stepBars('1h', -1, now: now);
      c.stepBars('1h', -1, now: now);
      expect(ui, 3, reason: 'banner/scrubber must track every step');
      expect(reloads, 1, reason: 'fetches stay debounced');
      expect(c.asOf, DateTime.utc(2025, 9, 16, 8));

      await Future<void>.delayed(const Duration(milliseconds: 80));
      expect(reloads, 2);

      c.exit();
      expect(ui, 4);
      expect(reloads, 3);
    });

    test('pulse surface suppresses Overview-style reload until release', () {
      final now = DateTime.utc(2025, 9, 16, 10, 17);
      final c = ReplayController(reloadDebounce: Duration.zero);
      var reloads = 0;
      c.addReloadListener(() {
        if (c.isPulseForeground) return;
        reloads++;
      });

      c.enter(timeframe: '1h', now: now);
      expect(reloads, 1);

      c.acquirePulseSurface();
      c.stepBars('1h', -1, now: now);
      expect(reloads, 1);

      c.releasePulseSurface();
      expect(reloads, 2, reason: 'release syncs covered surfaces once');
    });
  });

  group('formatReplayBanner', () {
    test('matches the ROADMAP banner shape', () {
      expect(
        formatReplayBanner(DateTime.utc(2025, 9, 16, 8, 30)),
        'Replay: Sep 16 08:30 UTC',
      );
    });
  });

  group('uriWithAsOf', () {
    test('appends asOf when set', () {
      final uri = Uri.parse('https://x/api/rankings?timeframe=1h');
      expect(
        uriWithAsOf(uri, 1726473600).queryParameters['asOf'],
        '1726473600',
      );
      expect(uriWithAsOf(uri, null), uri);
    });
  });
}
