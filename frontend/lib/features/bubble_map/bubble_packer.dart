import 'dart:math' as math;

import 'bubble_token.dart';

/// A [BubbleToken] with computed layout coordinates.
class PackedBubble {
  final BubbleToken token;
  double x;
  double y;
  final double radius;

  PackedBubble({
    required this.token,
    required this.x,
    required this.y,
    required this.radius,
  });
}

/// Packs bubbles into a rectangular area using spiral placement.
///
/// Bubbles are sorted largest-first and placed along an Archimedean spiral,
/// choosing the first position that does not overlap any already-placed bubble.
/// After all bubbles are placed, the cluster is centred inside the given
/// [width]×[height] rectangle.
class BubblePacker {
  /// Minimum bubble radius in logical pixels.
  static const double minRadius = 16.0;

  /// Maximum bubble radius as a fraction of the shorter screen dimension.
  static const double maxRadiusFraction = 0.18;

  /// Packs [tokens] into a [width]×[height] area.
  ///
  /// [sizeBy] controls which metric determines bubble size:
  ///   - `'volume'` (default) — logarithmic volume scaling
  ///   - `'change'` — absolute price change scaling
  ///
  /// Returns a list of [PackedBubble] with layout coordinates set.
  List<PackedBubble> pack(
    List<BubbleToken> tokens, {
    required double width,
    required double height,
    String sizeBy = 'volume',
  }) {
    if (tokens.isEmpty || width <= 0 || height <= 0) return [];

    final radii = _computeRadii(tokens, width, height, sizeBy);

    // Create index pairs and sort by radius descending.
    final indices = List<int>.generate(tokens.length, (i) => i);
    indices.sort((a, b) => radii[b].compareTo(radii[a]));

    final placed = <PackedBubble>[];
    final cx = width / 2;
    final cy = height / 2;

    for (final i in indices) {
      final r = radii[i];
      final pos = _findPosition(placed, r, cx, cy);
      placed.add(PackedBubble(token: tokens[i], x: pos.x, y: pos.y, radius: r));
    }

    // Re-centre the cluster.
    _centre(placed, width, height);

    return placed;
  }

  // ---- internal helpers ----

  List<double> _computeRadii(
    List<BubbleToken> tokens,
    double width,
    double height,
    String sizeBy,
  ) {
    final maxR = math.min(width, height) * maxRadiusFraction;

    final values = tokens.map((t) {
      if (sizeBy == 'change') return t.priceChange.abs();
      // volume — use log1p to compress large values while keeping order.
      return t.volume > 0 ? math.log(t.volume + 1) : 0.0;
    }).toList();

    final vMax = values.fold<double>(0.0, math.max);
    if (vMax == 0) return List.filled(tokens.length, minRadius);

    return values.map((v) {
      final t = v / vMax; // 0..1
      return minRadius + (maxR - minRadius) * t;
    }).toList();
  }

  /// Finds the first non-overlapping position along an Archimedean spiral.
  _Pos _findPosition(List<PackedBubble> placed, double r, double cx, double cy) {
    if (placed.isEmpty) return _Pos(cx, cy);

    const angleStep = 0.3; // radians per step
    const radiusGrowth = 1.5; // spiral tightness
    const maxSteps = 5000;

    for (var step = 0; step < maxSteps; step++) {
      final angle = step * angleStep;
      final dist = radiusGrowth * angle; // grows with angle
      final px = cx + dist * math.cos(angle);
      final py = cy + dist * math.sin(angle);
      if (!_overlaps(placed, px, py, r)) {
        return _Pos(px, py);
      }
    }

    // Fallback — should not happen with reasonable data.
    return _Pos(cx, cy);
  }

  bool _overlaps(List<PackedBubble> placed, double px, double py, double r) {
    const padding = 2.0; // minimum gap between bubbles
    for (final b in placed) {
      final dx = b.x - px;
      final dy = b.y - py;
      final minDist = b.radius + r + padding;
      if (dx * dx + dy * dy < minDist * minDist) return true;
    }
    return false;
  }

  /// Shifts all bubbles so the bounding box is centred in
  /// the [width]×[height] viewport.
  void _centre(List<PackedBubble> bubbles, double width, double height) {
    if (bubbles.isEmpty) return;

    var minX = double.infinity, maxX = double.negativeInfinity;
    var minY = double.infinity, maxY = double.negativeInfinity;

    for (final b in bubbles) {
      if (b.x - b.radius < minX) minX = b.x - b.radius;
      if (b.x + b.radius > maxX) maxX = b.x + b.radius;
      if (b.y - b.radius < minY) minY = b.y - b.radius;
      if (b.y + b.radius > maxY) maxY = b.y + b.radius;
    }

    final clusterW = maxX - minX;
    final clusterH = maxY - minY;
    final dx = (width - clusterW) / 2 - minX;
    final dy = (height - clusterH) / 2 - minY;

    for (final b in bubbles) {
      b.x += dx;
      b.y += dy;
    }
  }
}

class _Pos {
  final double x;
  final double y;
  const _Pos(this.x, this.y);
}
