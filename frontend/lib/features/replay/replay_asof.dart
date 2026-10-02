import '../detail/chart_navigation.dart';

/// Shared footnote under the replay banner (Overview + Pulse).
const String kReplayBannerFootnote =
    'MTF off · live-only cards hidden · scorecards = window start';

/// Aligns [utc] down to the open of the candle containing it for [timeframe].
///
/// Matches backend `candleBoundary` / PR-112b scrubber contract: scrubber
/// steps emit bar-aligned unix seconds. Backend replay includes bars with
/// `open+tf ≤ asOf`, so the forming bar whose open equals this aligned
/// instant is excluded.
DateTime alignAsOfToBar(DateTime utc, String timeframe) {
  final d = candleDuration(timeframe);
  final ms = utc.toUtc().millisecondsSinceEpoch;
  final step = d.inMilliseconds;
  if (step <= 0) return utc.toUtc();
  final aligned = ms - (ms % step);
  return DateTime.fromMillisecondsSinceEpoch(aligned, isUtc: true);
}

/// Caps [asOf] at the open of the candle containing [now] (UTC).
///
/// That open is the latest legal scrub position: the in-progress bar is
/// not included by backend `open+tf ≤ asOf` filtering.
DateTime clampAsOfToNow(DateTime asOf, String timeframe, {DateTime? now}) {
  final n = (now ?? DateTime.now()).toUtc();
  final latest = alignAsOfToBar(n, timeframe);
  final a = alignAsOfToBar(asOf.toUtc(), timeframe);
  return a.isAfter(latest) ? latest : a;
}

/// Absolute RFC3339 lower bound for scorecard `since` under replay:
/// `asOf − 30d`, always ≤ [asOf].
///
/// This is **not** a full point-in-time scorecard: without backend
/// `until=asOf`, outcomes after [asOf] can still count. Prefer treating
/// chips as a best-effort window start until PR-112c (or similar).
String replayScorecardSince(DateTime asOf) {
  final start = asOf.toUtc().subtract(const Duration(days: 30));
  return start.toIso8601String();
}

/// Formats the persistent replay banner label.
String formatReplayBanner(DateTime asOf) {
  final u = asOf.toUtc();
  const months = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  final mon = months[u.month - 1];
  final dd = u.day.toString().padLeft(2, '0');
  final hh = u.hour.toString().padLeft(2, '0');
  final mm = u.minute.toString().padLeft(2, '0');
  return 'Replay: $mon $dd $hh:$mm UTC';
}

/// Appends `asOf=<unix>` when [asOf] is set.
Uri uriWithAsOf(Uri uri, int? asOf) {
  if (asOf == null) return uri;
  final q = Map<String, String>.from(uri.queryParameters);
  q['asOf'] = '$asOf';
  return uri.replace(queryParameters: q);
}
