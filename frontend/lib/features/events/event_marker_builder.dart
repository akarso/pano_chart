import '../../domain/event.dart';
import '../candles/api/candle_response.dart';
import 'chart_event_overlay_painter.dart';
import 'event_filter.dart';

/// Computes [EventMarker] positions by aligning events to the nearest
/// candle in a candle series. Events outside the visible range are excluded.
///
/// [candleWidth] is the pixel width per candle (same value used by the
/// chart renderer).
List<EventMarker> buildEventMarkers({
  required CandleSeriesResponse series,
  required List<Event> events,
  required EventFilterLevel filterLevel,
  required double candleWidth,
}) {
  final candles = series.candles;
  if (candles.isEmpty || events.isEmpty) return const [];

  // Pre-filter by impact
  final filtered = events.where((e) => filterLevel.allows(e.impact)).toList();
  if (filtered.isEmpty) return const [];

  final firstTs = candles.first.timestamp;
  final lastTs = candles.last.timestamp;

  // Group events by nearest candle index
  final Map<int, List<Event>> buckets = {};

  for (final event in filtered) {
    if (event.timestamp.isBefore(firstTs) ||
        event.timestamp.isAfter(lastTs)) {
      continue; // outside visible range
    }
    final candleIndex = _nearestCandleIndex(candles, event.timestamp);
    buckets.putIfAbsent(candleIndex, () => []).add(event);
  }

  // Convert to markers
  return buckets.entries.map((entry) {
    final x = entry.key * candleWidth + candleWidth / 2;
    return EventMarker(
      x: x,
      timestamp: candles[entry.key].timestamp,
      events: entry.value,
    );
  }).toList()
    ..sort((a, b) => a.x.compareTo(b.x));
}

/// Binary-search to find the candle whose timestamp is closest to [target].
int _nearestCandleIndex(List<CandleDto> candles, DateTime target) {
  int lo = 0;
  int hi = candles.length - 1;
  while (lo < hi) {
    final mid = (lo + hi) ~/ 2;
    if (candles[mid].timestamp.isBefore(target)) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  // lo is the first candle >= target. Check if lo-1 is closer (or equal distance, prefer earlier).
  if (lo > 0) {
    final diffLo = target.difference(candles[lo].timestamp).abs();
    final diffPrev = target.difference(candles[lo - 1].timestamp).abs();
    if (diffPrev <= diffLo) return lo - 1;
  }
  return lo;
}
