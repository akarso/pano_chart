import 'dart:async';

/// A fire-and-reschedule auto-refresh timer.
///
/// After each tick the timer waits for the [onTick] callback to complete
/// before scheduling the next one.  This prevents overlapping refresh
/// cycles when the data fetch is slow.
///
/// Usage:
///
/// ```dart
/// final timer = AutoRefreshTimer(
///   interval: const Duration(seconds: 8),
///   onTick: () async => _doRefresh(),
/// );
/// timer.start();
/// // later …
/// timer.dispose();
/// ```
class AutoRefreshTimer {
  Duration _interval;
  final Future<void> Function() onTick;

  Timer? _timer;
  bool _disposed = false;
  bool _running = false;

  AutoRefreshTimer({
    required Duration interval,
    required this.onTick,
  }) : _interval = interval;

  /// Whether the timer is currently running (scheduled for next tick).
  bool get isRunning => _running;

  /// The current interval between ticks.
  Duration get interval => _interval;

  /// Start the timer.  If already running, this is a no-op.
  void start() {
    if (_disposed || _running) return;
    _running = true;
    _schedule();
  }

  /// Stop the timer without disposing.  Can be restarted with [start].
  void stop() {
    _running = false;
    _timer?.cancel();
    _timer = null;
  }

  /// Restart with an optionally new [interval].
  void restart({Duration? interval}) {
    if (interval != null) _interval = interval;
    stop();
    start();
  }

  /// Update the interval *without* restarting.  The new interval takes
  /// effect at the next reschedule.
  void updateInterval(Duration interval) {
    _interval = interval;
  }

  /// Stop and release resources.  The timer cannot be restarted after
  /// this call.
  void dispose() {
    _disposed = true;
    stop();
  }

  void _schedule() {
    if (_disposed || !_running) return;
    _timer = Timer(_interval, _fire);
  }

  Future<void> _fire() async {
    if (_disposed || !_running) return;
    try {
      await onTick();
    } catch (_) {
      // Swallow — the caller's callback should handle its own errors.
    }
    _schedule();
  }
}
