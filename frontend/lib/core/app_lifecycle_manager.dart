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

  /// Whether the app is currently in the background.
  bool get isPaused => _isPaused;

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

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    switch (state) {
      case AppLifecycleState.paused:
      case AppLifecycleState.inactive:
      case AppLifecycleState.detached:
        if (!_isPaused) {
          _isPaused = true;
          for (final p in _pausables) {
            p.onPause();
          }
        }
        break;
      case AppLifecycleState.resumed:
        if (_isPaused) {
          _isPaused = false;
          for (final p in _pausables) {
            p.onResume();
          }
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
