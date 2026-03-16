import 'package:flutter/material.dart';

/// Paints a glowing dot at the last point of a sparkline to indicate
/// a value change after refresh.
///
/// - [color]: green for positive change, red for negative.
/// - [progress]: 0→1→0 flash animation progress. At peak (0.5) the dot
///   and glow are brightest; at 0 and 1 they are invisible.
/// - [points]: the sparkline data (needed to compute the y-position).
/// - [normalize] and [globalMaxPct]: same as SparklineRenderer for
///   consistent positioning.
class SparklineFlashDotPainter extends CustomPainter {
  final Color color;
  final double progress;
  final List<double> points;
  final bool normalize;
  final double globalMaxPct;

  SparklineFlashDotPainter({
    required this.color,
    required this.progress,
    required this.points,
    this.normalize = true,
    this.globalMaxPct = 0.05,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2 || progress <= 0) return;

    // Compute y position of the last point using the same logic as
    // SparklineRenderer.
    final lastX = size.width;
    final double lastY;

    if (normalize) {
      final minVal = points.reduce((a, b) => a < b ? a : b);
      final maxVal = points.reduce((a, b) => a > b ? a : b);
      final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);
      lastY = size.height - ((points.last - minVal) / range) * size.height;
    } else {
      final first = points.first;
      final pct = first == 0 ? 0.0 : (points.last - first) / first;
      var ratio = (pct + globalMaxPct) / (2 * globalMaxPct);
      if (ratio < 0) ratio = 0;
      if (ratio > 1) ratio = 1;
      lastY = size.height - ratio * size.height;
    }

    // Flash envelope mapped from progress 0→1 over 4250ms total:
    //   0→0.0588  (  0–250ms)  glow up      0→1
    //   0.0588→0.5294  (250–2250ms) full bright   1
    //   0.5294→1.0  (2250–4250ms) fade out     1→0
    const double rampEnd = 250 / 4250;   // ≈ 0.0588
    const double holdEnd = 2250 / 4250;  // ≈ 0.5294
    final double flash;
    if (progress <= rampEnd) {
      flash = progress / rampEnd;
    } else if (progress <= holdEnd) {
      flash = 1.0;
    } else {
      flash = (1.0 - progress) / (1.0 - holdEnd);
    }
    final opacity = (flash * 255).round().clamp(0, 255);
    if (opacity <= 0) return;

    final center = Offset(lastX, lastY);

    // Glow
    final glowPaint = Paint()
      ..color = color.withAlpha((opacity * 0.4).round())
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 8);
    canvas.drawCircle(center, 6.0, glowPaint);

    // Bright dot
    final dotPaint = Paint()
      ..color = color.withAlpha(opacity);
    canvas.drawCircle(center, 3.0, dotPaint);
  }

  @override
  bool shouldRepaint(covariant SparklineFlashDotPainter old) {
    return progress != old.progress ||
        color != old.color ||
        !identical(points, old.points);
  }
}
