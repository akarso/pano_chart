import '../api/social_models.dart';
import '../../candles/api/candle_response.dart';
import 'social_chart_overlay_painter.dart';

/// Computes [SocialMarker] positions by aligning social posts to the
/// nearest candle in a candle series.
///
/// Only posts within the visible candle range are included.
List<SocialMarker> buildSocialMarkers({
  required CandleSeriesResponse series,
  required List<SocialPost> posts,
  required double candleWidth,
  double scrollPixelOffset = 0.0,
}) {
  final candles = series.candles;
  if (candles.isEmpty || posts.isEmpty) return const [];

  final firstTs = candles.first.timestamp;
  final lastTs = candles.last.timestamp;

  // Group posts by nearest candle index.
  final Map<int, List<SocialPost>> buckets = {};

  for (final post in posts) {
    final dt = post.dateTime;
    if (dt.isBefore(firstTs) || dt.isAfter(lastTs)) continue;
    final idx = _nearestCandleIndex(candles, dt);
    buckets.putIfAbsent(idx, () => []).add(post);
  }

  return buckets.entries.map((entry) {
    final x = entry.key * candleWidth + candleWidth / 2 - scrollPixelOffset;
    return SocialMarker(
      x: x,
      timestamp: candles[entry.key].timestamp,
      posts: entry.value,
    );
  }).toList()
    ..sort((a, b) => a.x.compareTo(b.x));
}

/// Binary-search for the candle closest to [target].
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
  if (lo > 0) {
    final diffLo = target.difference(candles[lo].timestamp).abs();
    final diffPrev = target.difference(candles[lo - 1].timestamp).abs();
    if (diffPrev <= diffLo) return lo - 1;
  }
  return lo;
}
