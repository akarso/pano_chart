import 'dart:async';

import 'package:flutter/foundation.dart';

import '../detail/chart_navigation.dart';
import 'replay_asof.dart';

/// Shared Pro replay state for Market Pulse and the rankings grid (PR-112b).
///
/// Holds one logical UTC instant. Surfaces must call [asOfFor] /
/// [asOfUnixFor] with **their** timeframe so `?asOf=` is bar-aligned for
/// that TF.
///
/// **Listeners:** [ChangeNotifier] (this class) fires **immediately** on
/// every scrub so banner / scrubber stay live. [addReloadListener] fires
/// immediately on [enter] / [exit], and **debounced** (default 400ms) on
/// scrub steps so Overview/Pulse fetches coalesce against the replay rate
/// limit.
///
/// While a Pulse route holds [acquirePulseSurface], Overview reload
/// listeners should no-op (see [isPulseForeground]).
class ReplayController extends ChangeNotifier {
  ReplayController({this.reloadDebounce = const Duration(milliseconds: 400)});

  /// Delay before [addReloadListener] callbacks after scrubber steps.
  final Duration reloadDebounce;

  DateTime? _instant;
  Timer? _reloadDebounce;
  final List<VoidCallback> _reloadListeners = [];
  int _pulseSurfaceCount = 0;

  /// Raw scrub instant (UTC). Prefer [asOfFor] for display and requests.
  DateTime? get asOf => _instant;

  bool get isActive => _instant != null;

  /// True while one or more Market Pulse routes own scrubbing.
  bool get isPulseForeground => _pulseSurfaceCount > 0;

  /// Bar-aligned asOf for [timeframe] (null when live).
  DateTime? asOfFor(String timeframe, {DateTime? now}) {
    final a = _instant;
    if (a == null) return null;
    return clampAsOfToNow(a, timeframe, now: now);
  }

  /// Unix seconds for API `?asOf=`, aligned to [timeframe]; null when live.
  int? asOfUnixFor(String timeframe, {DateTime? now}) {
    final a = asOfFor(timeframe, now: now);
    if (a == null) return null;
    return a.millisecondsSinceEpoch ~/ 1000;
  }

  /// Register a fetch/reload callback (debounced on scrub, immediate on
  /// enter/exit). Prefer this over [addListener] for API reloads.
  void addReloadListener(VoidCallback listener) {
    _reloadListeners.add(listener);
  }

  void removeReloadListener(VoidCallback listener) {
    _reloadListeners.remove(listener);
  }

  /// Pulse route entered — Overview must not reload on every scrub.
  void acquirePulseSurface() {
    _pulseSurfaceCount++;
  }

  /// Pulse route left. When the last surface releases, reload listeners
  /// fire immediately so Overview can sync once.
  void releasePulseSurface() {
    if (_pulseSurfaceCount == 0) return;
    _pulseSurfaceCount--;
    if (_pulseSurfaceCount == 0) {
      _notifyReloadImmediate();
    }
  }

  /// Enter replay at [at] (default: now), aligned to [timeframe].
  void enter({required String timeframe, DateTime? at, DateTime? now}) {
    final n = now ?? DateTime.now();
    final seed = at ?? n;
    _cancelReloadDebounce();
    _instant = clampAsOfToNow(seed, timeframe, now: n);
    notifyListeners();
    _notifyReloadImmediate();
  }

  void exit() {
    if (_instant == null) return;
    _cancelReloadDebounce();
    _instant = null;
    notifyListeners();
    _notifyReloadImmediate();
  }

  /// Updates the logical instant (clamped to [timeframe]). Returns whether
  /// the stored instant changed. When unchanged, callers that still need a
  /// reload (e.g. Overview TF change) must trigger it themselves.
  bool setAsOf(
    String timeframe,
    DateTime value, {
    DateTime? now,
    bool immediateReload = false,
  }) {
    final next = clampAsOfToNow(value, timeframe, now: now);
    if (_instant == next) return false;
    _instant = next;
    notifyListeners(); // UI always immediate
    if (immediateReload || reloadDebounce <= Duration.zero) {
      _notifyReloadImmediate();
    } else {
      _scheduleReload();
    }
    return true;
  }

  /// Step by [delta] bars (±1 typically) on [timeframe]'s grid.
  void stepBars(String timeframe, int delta, {DateTime? now}) {
    if (_instant == null || delta == 0) return;
    final current = clampAsOfToNow(_instant!, timeframe, now: now);
    final step = candleDuration(timeframe) * delta.abs();
    final next = delta > 0 ? current.add(step) : current.subtract(step);
    setAsOf(timeframe, next, now: now);
  }

  /// Step by [days] calendar days, then re-align to [timeframe].
  void stepDays(String timeframe, int days, {DateTime? now}) {
    if (_instant == null || days == 0) return;
    final current = clampAsOfToNow(_instant!, timeframe, now: now);
    final next = current.add(Duration(days: days));
    setAsOf(timeframe, next, now: now);
  }

  void _scheduleReload() {
    _reloadDebounce?.cancel();
    _reloadDebounce = Timer(reloadDebounce, _notifyReloadImmediate);
  }

  void _notifyReloadImmediate() {
    _cancelReloadDebounce();
    for (final listener in List<VoidCallback>.of(_reloadListeners)) {
      listener();
    }
  }

  void _cancelReloadDebounce() {
    _reloadDebounce?.cancel();
    _reloadDebounce = null;
  }

  @override
  void dispose() {
    _cancelReloadDebounce();
    _reloadListeners.clear();
    super.dispose();
  }
}
