import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:pano_chart_frontend/core/app_lifecycle_manager.dart';

void main() {
  group('AppLifecycleManager', () {
    late AppLifecycleManager manager;

    setUp(() {
      manager = AppLifecycleManager();
    });

    test('starts in foreground state', () {
      expect(manager.isPaused, isFalse);
    });

    test('fires onPause for all registered pausables on paused', () {
      var pauseCount = 0;
      manager.addPausable(Pausable(
        onPause: () => pauseCount++,
        onResume: () {},
      ));
      manager.addPausable(Pausable(
        onPause: () => pauseCount++,
        onResume: () {},
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.paused);

      expect(pauseCount, 2);
      expect(manager.isPaused, isTrue);
    });

    test('fires onResume for all registered pausables on resumed', () {
      var resumeCount = 0;
      manager.addPausable(Pausable(
        onPause: () {},
        onResume: () => resumeCount++,
      ));

      // Pause first, then resume.
      manager.didChangeAppLifecycleState(AppLifecycleState.paused);
      manager.didChangeAppLifecycleState(AppLifecycleState.resumed);

      expect(resumeCount, 1);
      expect(manager.isPaused, isFalse);
    });

    test('does not double-fire onPause on consecutive paused states', () {
      var pauseCount = 0;
      manager.addPausable(Pausable(
        onPause: () => pauseCount++,
        onResume: () {},
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.paused);
      manager.didChangeAppLifecycleState(AppLifecycleState.inactive);

      expect(pauseCount, 1);
    });

    test('does not fire onResume when already in foreground', () {
      var resumeCount = 0;
      manager.addPausable(Pausable(
        onPause: () {},
        onResume: () => resumeCount++,
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.resumed);

      expect(resumeCount, 0);
    });

    test('inactive triggers pause', () {
      var paused = false;
      manager.addPausable(Pausable(
        onPause: () => paused = true,
        onResume: () {},
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.inactive);

      expect(paused, isTrue);
    });

    test('detached triggers pause', () {
      var paused = false;
      manager.addPausable(Pausable(
        onPause: () => paused = true,
        onResume: () {},
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.detached);

      expect(paused, isTrue);
    });

    test('removePausable stops receiving callbacks', () {
      var pauseCount = 0;
      final p = Pausable(
        onPause: () => pauseCount++,
        onResume: () {},
      );
      manager.addPausable(p);
      manager.removePausable(p);

      manager.didChangeAppLifecycleState(AppLifecycleState.paused);

      expect(pauseCount, 0);
    });

    test('addPausable fires onPause immediately when already paused', () {
      manager.didChangeAppLifecycleState(AppLifecycleState.paused);

      var paused = false;
      manager.addPausable(Pausable(
        onPause: () => paused = true,
        onResume: () {},
      ));

      expect(paused, isTrue);
    });

    test('hidden state does not trigger pause or resume', () {
      var pauseCount = 0;
      var resumeCount = 0;
      manager.addPausable(Pausable(
        onPause: () => pauseCount++,
        onResume: () => resumeCount++,
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.hidden);

      expect(pauseCount, 0);
      expect(resumeCount, 0);
    });

    test('full cycle: pause → resume → pause', () {
      final log = <String>[];
      manager.addPausable(Pausable(
        onPause: () => log.add('pause'),
        onResume: () => log.add('resume'),
      ));

      manager.didChangeAppLifecycleState(AppLifecycleState.paused);
      manager.didChangeAppLifecycleState(AppLifecycleState.resumed);
      manager.didChangeAppLifecycleState(AppLifecycleState.inactive);

      expect(log, ['pause', 'resume', 'pause']);
    });
  });
}
