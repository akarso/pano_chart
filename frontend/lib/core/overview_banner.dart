import 'dart:async';
import 'package:flutter/material.dart';

/// Staleness thresholds per timeframe.
///
/// After this duration since last successful load, the stale banner shows.
const Map<String, Duration> kStaleThresholds = {
  '1m': Duration(minutes: 1, seconds: 30),
  '5m': Duration(minutes: 2, seconds: 30),
  '15m': Duration(minutes: 5),
  '1h': Duration(minutes: 10),
  '4h': Duration(hours: 1),
  '1d': Duration(hours: 1),
};

/// Determines whether data for [timeframe] loaded at [loadedAt] is stale
/// relative to [now].
bool isDataStale(String timeframe, DateTime loadedAt, DateTime now) {
  final threshold = kStaleThresholds[timeframe] ?? const Duration(minutes: 5);
  return now.difference(loadedAt) >= threshold;
}

/// The kind of informational banner to show at the top of the overview.
enum OverviewBannerKind {
  /// No banner.
  none,

  /// Stale data — user should pull to refresh.
  stale,

  /// Offline — serving cached content.
  offline,
}

/// A slim, non-overlapping banner shown at the top of the overview page.
///
/// Only one banner is visible at a time. [OverviewBannerKind.offline] takes
/// priority over [OverviewBannerKind.stale].
class OverviewBanner extends StatelessWidget {
  final OverviewBannerKind kind;

  const OverviewBanner({super.key, required this.kind});

  @override
  Widget build(BuildContext context) {
    if (kind == OverviewBannerKind.none) return const SizedBox.shrink();

    final isOffline = kind == OverviewBannerKind.offline;
    final text = isOffline
        ? 'No connection — showing cached data'
        : 'Stale content, pull down to refresh';
    final color = isOffline ? const Color(0xFFB71C1C) : const Color(0xFF4A4A00);
    final icon = isOffline ? Icons.cloud_off : Icons.access_time;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      color: color.withAlpha(200),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(icon, color: Colors.white70, size: 14),
          const SizedBox(width: 6),
          Flexible(
            child: Text(
              text,
              style: const TextStyle(
                color: Colors.white70,
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }
}

/// Mixin (or standalone helper) that manages a periodic [Timer] to check
/// for data staleness and emits the appropriate [OverviewBannerKind].
class StalenessTracker {
  DateTime? _lastLoadedAt;
  String _timeframe = '1h';
  bool _isOffline = false;
  Timer? _timer;
  OverviewBannerKind _kind = OverviewBannerKind.none;
  VoidCallback? onChanged;

  OverviewBannerKind get kind => _kind;

  /// Call after a successful data load.
  void markLoaded() {
    _lastLoadedAt = DateTime.now();
    _isOffline = false;
    _update();
  }

  /// Call when the load failed and we're serving cached data.
  void markOffline() {
    _isOffline = true;
    _update();
  }

  /// Call when connectivity is restored and fresh data is loaded.
  void markOnline() {
    _isOffline = false;
    _lastLoadedAt = DateTime.now();
    _update();
  }

  /// Update the current timeframe (affects staleness threshold).
  void setTimeframe(String tf) {
    _timeframe = tf;
    _update();
  }

  /// Start the periodic staleness check. Call in initState.
  void start() {
    _timer?.cancel();
    _timer = Timer.periodic(const Duration(seconds: 10), (_) => _update());
  }

  /// Stop the timer. Call in dispose.
  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  void _update() {
    final previous = _kind;
    if (_isOffline) {
      _kind = OverviewBannerKind.offline;
    } else if (_lastLoadedAt != null &&
        isDataStale(_timeframe, _lastLoadedAt!, DateTime.now())) {
      _kind = OverviewBannerKind.stale;
    } else {
      _kind = OverviewBannerKind.none;
    }
    if (_kind != previous) {
      onChanged?.call();
    }
  }
}
