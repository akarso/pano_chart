import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/async/single_flight.dart';

void main() {
  group('singleFlight', () {
    test('concurrent calls share one underlying invocation', () async {
      var callCount = 0;
      final completer = Completer<void>();
      final guarded = singleFlight(() async {
        callCount++;
        await completer.future;
      });

      final f1 = guarded();
      final f2 = guarded();
      final f3 = guarded();

      // All three calls happened before the underlying call resolved —
      // only one invocation should have started.
      expect(callCount, 1);

      completer.complete();
      await Future.wait([f1, f2, f3]);

      expect(callCount, 1);
    });

    test('a call after completion starts a fresh invocation', () async {
      var callCount = 0;
      final guarded = singleFlight(() async {
        callCount++;
      });

      await guarded();
      await guarded();

      expect(callCount, 2);
    });

    test('all concurrent callers observe the same error', () async {
      final guarded = singleFlight(() async {
        throw Exception('boom');
      });

      final f1 = guarded();
      final f2 = guarded();

      await expectLater(f1, throwsException);
      await expectLater(f2, throwsException);
    });

    test('a later call after a failure starts a fresh invocation', () async {
      var callCount = 0;
      final guarded = singleFlight(() async {
        callCount++;
        if (callCount == 1) throw Exception('first fails');
      });

      await expectLater(guarded(), throwsException);
      await guarded();

      expect(callCount, 2);
    });

    test('simulates a burst of 401s sharing one reclaim', () async {
      // Mirrors main.dart's reclaimDeviceSecret usage: several concurrent
      // callers all lose the race except one, and all should see the
      // winner's result rather than each attempting their own claim.
      var claimAttempts = 0;
      String? secret;
      final reclaim = singleFlight(() async {
        claimAttempts++;
        await Future<void>.delayed(const Duration(milliseconds: 5));
        secret = 'fresh-secret';
      });

      await Future.wait([reclaim(), reclaim(), reclaim(), reclaim()]);

      expect(claimAttempts, 1);
      expect(secret, 'fresh-secret');
    });
  });
}
