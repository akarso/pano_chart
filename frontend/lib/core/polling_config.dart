/// Configurable constants for staggered auto-refresh polling.
///
/// These values control the auto-refresh cycle timing for the overview
/// sparkline grid and per-screen refresh intervals.

/// Delay in milliseconds between sequential flash-dot activations
/// during auto-refresh.
const int kStaggerDelayMs = 20;

/// Safety margin in milliseconds added after the stagger sequence
/// before scheduling the next auto-refresh cycle.
const int kStaggerMarginMs = 5000;

/// Computes the auto-refresh interval for the overview grid
/// based on loaded symbol count.
///
/// interval = ([kStaggerDelayMs] × [symbolCount]) + [kStaggerMarginMs]
Duration overviewAutoRefreshInterval(int symbolCount) {
  return Duration(
    milliseconds: (kStaggerDelayMs * symbolCount) + kStaggerMarginMs,
  );
}

/// Detail view and bubble map auto-refresh intervals per timeframe.
///
/// Shared because both screens use the same schedule.
const Map<String, Duration> kChartRefreshIntervals = {
  '1m': Duration(seconds: 10),
  '5m': Duration(minutes: 1),
  '15m': Duration(minutes: 3),
  '1h': Duration(minutes: 10),
  '4h': Duration(minutes: 15),
  '1d': Duration(hours: 1),
};

/// Duration after which macro events are re-fetched when the user
/// remains on a detail chart without navigating away.
const Duration kMacroEventsRefreshDuration = Duration(minutes: 15);
