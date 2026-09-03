import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/auto_refresh_timer.dart';

void main() {
  group('AutoRefreshTimer', () {
    test('fires after interval', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 5),
          onTick: () async => ticks++,
        );
        timer.start();

        async.elapse(const Duration(seconds: 4));
        expect(ticks, 0);

        async.elapse(const Duration(seconds: 1));
        expect(ticks, 1);

        timer.dispose();
      });
    });

    test('fires repeatedly', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 3),
          onTick: () async => ticks++,
        );
        timer.start();

        async.elapse(const Duration(seconds: 9));
        expect(ticks, 3);

        timer.dispose();
      });
    });

    test('stop prevents further ticks', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 2),
          onTick: () async => ticks++,
        );
        timer.start();

        async.elapse(const Duration(seconds: 2));
        expect(ticks, 1);

        timer.stop();
        async.elapse(const Duration(seconds: 10));
        expect(ticks, 1);

        timer.dispose();
      });
    });

    test('restart resets interval', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 10),
          onTick: () async => ticks++,
        );
        timer.start();

        async.elapse(const Duration(seconds: 5));
        timer.restart(interval: const Duration(seconds: 2));

        async.elapse(const Duration(seconds: 2));
        expect(ticks, 1);

        timer.dispose();
      });
    });

    test('dispose prevents further ticks', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 1),
          onTick: () async => ticks++,
        );
        timer.start();
        timer.dispose();

        async.elapse(const Duration(seconds: 10));
        expect(ticks, 0);
      });
    });

    test('start is no-op if disposed', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 1),
          onTick: () async => ticks++,
        );
        timer.dispose();
        timer.start();

        async.elapse(const Duration(seconds: 10));
        expect(ticks, 0);
      });
    });

    test('start is no-op when already running', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 2),
          onTick: () async => ticks++,
        );
        timer.start();
        timer.start(); // should be no-op

        async.elapse(const Duration(seconds: 4));
        expect(ticks, 2);

        timer.dispose();
      });
    });

    test('updateInterval changes next tick interval', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 5),
          onTick: () async => ticks++,
        );
        timer.start();

        async.elapse(const Duration(seconds: 5));
        expect(ticks, 1);

        // updateInterval takes effect at the *next* reschedule, so the
        // already-pending 5s timer must fire first.
        timer.updateInterval(const Duration(seconds: 1));
        async.elapse(const Duration(seconds: 5));
        expect(ticks, 2);

        // Now the 1s interval is active.
        async.elapse(const Duration(seconds: 1));
        expect(ticks, 3);

        timer.dispose();
      });
    });

    test('swallows errors from onTick and continues', () {
      FakeAsync().run((async) {
        var ticks = 0;
        final timer = AutoRefreshTimer(
          interval: const Duration(seconds: 1),
          onTick: () async {
            ticks++;
            if (ticks == 1) throw Exception('boom');
          },
        );
        timer.start();

        async.elapse(const Duration(seconds: 3));
        expect(ticks, 3);

        timer.dispose();
      });
    });

    test('isRunning reflects state correctly', () {
      final timer = AutoRefreshTimer(
        interval: const Duration(seconds: 1),
        onTick: () async {},
      );

      expect(timer.isRunning, false);
      timer.start();
      expect(timer.isRunning, true);
      timer.stop();
      expect(timer.isRunning, false);
      timer.start();
      expect(timer.isRunning, true);
      timer.dispose();
      expect(timer.isRunning, false);
    });
  });
}
