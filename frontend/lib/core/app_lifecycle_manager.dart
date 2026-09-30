import 'package:flutter/widgets.dart';

/// Centralized lifecycle observer that pauses and resumes registered
/// callbacks when the app transitions between foreground and background.
///
/// Screens with periodic timers register via [addPausable] and unregister
/// via [removePausable].  When the OS reports the app as paused /
/// inactive, all registered [onPause] callbacks fire.  When resumed,
/// all [onResume] callbacks fire.
///
/// Components that must keep running in the background (e.g. social feed
/// polling for notifications) should simply NOT register here.
class AppLifecycleManager with WidgetsBindingObserver {
  final List<Pausable> _pausables = [];
  bool _isPaused = false;

  /// Set when the app has been [AppLifecycleState.paused] or
  /// [AppLifecycleState.detached] since the last resume. A notification
  /// shade only passes through [AppLifecycleState.inactive].
  bool _sawBackground = false;

  /// True while [Pausable.onResume] is running if the app actually left
  /// the foreground. False for an inactive → resumed shade pull.
  bool resumeFollowsBackground = false;

  /// Whether the app is currently in the background.
  bool get isPaused => _isPaused;

  /// Number of currently-registered pausables — for tests verifying a
  /// component moved its registration on reparenting rather than leaking
  /// it on the old manager or duplicating it.
  @visibleForTesting
  int get pausableCount => _pausables.length;

  /// Call once at startup (after [WidgetsFlutterBinding.ensureInitialized]).
  void init() {
    WidgetsBinding.instance.addObserver(this);
  }

  /// Call if/when the manager should be torn down.
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _pausables.clear();
  }

  /// Register a pausable component.  If the app is already paused the
  /// [Pausable.onPause] callback fires immediately so the newcomer
  /// starts in the correct state.
  void addPausable(Pausable p) {
    _pausables.add(p);
    if (_isPaused) p.onPause();
  }

  /// Unregister a pausable component (typically in [State.dispose]).
  void removePausable(Pausable p) {
    _pausables.remove(p);
  }

  void _pauseAll() {
    if (_isPaused) return;
    _isPaused = true;
    for (final p in List<Pausable>.of(_pausables)) {
      p.onPause();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    switch (state) {
      case AppLifecycleState.paused:
      case AppLifecycleState.detached:
        _sawBackground = true;
        _pauseAll();
        break;
      case AppLifecycleState.inactive:
        _pauseAll();
        break;
      case AppLifecycleState.resumed:
        if (_isPaused) {
          _isPaused = false;
          resumeFollowsBackground = _sawBackground;
          _sawBackground = false;
          for (final p in List<Pausable>.of(_pausables)) {
            p.onResume();
          }
          resumeFollowsBackground = false;
        }
        break;
      case AppLifecycleState.hidden:
        break;
    }
  }
}

/// A component whose background work can be paused and resumed.
class Pausable {
  final VoidCallback onPause;
  final VoidCallback onResume;

  const Pausable({required this.onPause, required this.onResume});
}

/// InheritedWidget that provides the [AppLifecycleManager] to descendants.
class AppLifecycleScope extends InheritedWidget {
  final AppLifecycleManager manager;

  const AppLifecycleScope({
    Key? key,
    required this.manager,
    required Widget child,
  }) : super(key: key, child: child);

  static AppLifecycleManager? of(BuildContext context) {
    return context
        .dependOnInheritedWidgetOfExactType<AppLifecycleScope>()
        ?.manager;
  }

  @override
  bool updateShouldNotify(AppLifecycleScope oldWidget) =>
      manager != oldWidget.manager;
}
