import '../../domain/event.dart';
import '../candles/api/candle_response.dart';
import 'chart_event_overlay_painter.dart';
import 'event_filter.dart';

/// Computes [EventMarker] positions by aligning events to the nearest
/// candle in a candle series. Events outside the visible range are excluded.
///
/// [candleWidth] is the pixel width per candle (same value used by the
/// chart renderer).
/// [scrollPixelOffset] is subtracted from each marker x to align with the
/// candle painter's sub-candle fractional scroll.
List<EventMarker> buildEventMarkers({
  required CandleSeriesResponse series,
  required List<Event> events,
  required EventFilterLevel filterLevel,
  required double candleWidth,
  double scrollPixelOffset = 0.0,
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
    final x = entry.key * candleWidth + candleWidth / 2 - scrollPixelOffset;
    return EventMarker(
      x: x,
      timestamp: candles[entry.key].timestamp,
      events: entry.value,
    );
  }).toList()
    ..sort((a, b) => a.x.compareTo(b.x));
}

/// Computes [EventMarker] positions for events scheduled AFTER the last
/// candle. Positions are calculated by time-extrapolation using the candle
/// duration, producing virtual "slot" indices beyond the candle array.
///
/// Events exceeding [maxSlots] candle durations past the last candle are
/// clamped to the projection boundary.
List<EventMarker> buildFutureEventMarkers({
  required DateTime lastCandleTimestamp,
  required int totalCandleCount,
  required List<Event> events,
  required EventFilterLevel filterLevel,
  required double candleWidth,
  required Duration candleDuration,
  required int maxSlots,
  required int visibleStartIndex,
  required double scrollPixelOffset,
}) {
  if (events.isEmpty || maxSlots <= 0 || candleDuration.inMilliseconds <= 0) {
    return const [];
  }

  final filtered = events.where((e) => filterLevel.allows(e.impact)).toList();
  if (filtered.isEmpty) return const [];

  final maxTs = lastCandleTimestamp.add(candleDuration * maxSlots);

  // Group future events by virtual slot index
  final Map<int, List<Event>> buckets = {};

  for (final event in filtered) {
    if (!event.timestamp.isAfter(lastCandleTimestamp)) continue;
    if (event.timestamp.isAfter(maxTs)) continue;

    final diff = event.timestamp.difference(lastCandleTimestamp);
    final slot =
        (diff.inMilliseconds / candleDuration.inMilliseconds).floor();
    final clampedSlot = slot.clamp(0, maxSlots - 1);
    final absIndex = totalCandleCount + clampedSlot;
    buckets.putIfAbsent(absIndex, () => []).add(event);
  }

  return buckets.entries.map((entry) {
    final x = (entry.key - visibleStartIndex) * candleWidth +
        candleWidth / 2 -
        scrollPixelOffset;
    // Compute projected timestamp for the slot
    final slotOffset = entry.key - totalCandleCount;
    final ts = lastCandleTimestamp.add(candleDuration * slotOffset);
    return EventMarker(
      x: x,
      timestamp: ts,
      events: entry.value,
      isFuture: true,
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
