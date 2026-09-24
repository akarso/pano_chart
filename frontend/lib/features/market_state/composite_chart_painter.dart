import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';

/// First index of the scored window within [pointsLength], or 0 when the
/// whole series is scored / [windowBars] is unset.
int scoredWindowStart(int pointsLength, int windowBars) {
  if (windowBars <= 0 || windowBars >= pointsLength) return 0;
  return pointsLength - windowBars;
}

/// Draws the Market Composite Index with optional scored-window dimming and
/// an OLS regression overlay on the last [windowBars] points (PR-116).
class CompositeChartPainter extends CustomPainter {
  final List<IndexPoint> points;
  final Color lineColor;
  final Color regressionColor;
  final int windowBars;
  final bool solidRegression;

  CompositeChartPainter({
    required this.points,
    required this.lineColor,
    required this.regressionColor,
    this.windowBars = 0,
    this.solidRegression = true,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2) return;

    canvas.save();
    canvas.clipRect(Offset.zero & size);

    final values = points.map((p) => p.value).toList();
    var minV = values.reduce(math.min);
    var maxV = values.reduce(math.max);
    var range = maxV - minV;
    // Flat composite still draws baseline + OLS (SIDEWAYS / SILENT evidence).
    if (range == 0) {
      range = 1.0;
      minV -= 0.5;
      maxV += 0.5;
    }

    Offset pointAt(int i) {
      final x = (i / (points.length - 1)) * size.width;
      final y = size.height - ((values[i] - minV) / range) * size.height;
      return Offset(x, y);
    }

    double yFor(double v) =>
        size.height - ((v - minV) / range) * size.height;

    final scoredStart = scoredWindowStart(points.length, windowBars);

    // Fill first: dim prefix, then scored segment (so stroke sits on top).
    void fillSegment(int from, int toInclusive, int topAlpha) {
      if (toInclusive <= from) return;
      final fillPath = Path()..moveTo(pointAt(from).dx, pointAt(from).dy);
      for (var i = from + 1; i <= toInclusive; i++) {
        final p = pointAt(i);
        fillPath.lineTo(p.dx, p.dy);
      }
      final x0 = pointAt(from).dx;
      final x1 = pointAt(toInclusive).dx;
      fillPath
        ..lineTo(x1, size.height)
        ..lineTo(x0, size.height)
        ..close();
      final fillPaint = Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            lineColor.withAlpha(topAlpha),
            lineColor.withAlpha(0),
          ],
        ).createShader(Rect.fromLTRB(x0, 0, x1, size.height));
      canvas.drawPath(fillPath, fillPaint);
    }

    if (scoredStart > 0) {
      // 40% of the usual fill alpha (~60) ≈ 24.
      fillSegment(0, scoredStart, (0.4 * 60).round());
    }
    fillSegment(scoredStart, points.length - 1, 60);

    // Baseline at 100
    final baseY = yFor(100);
    final basePaint = Paint()
      ..color = Colors.white24
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;
    if (baseY >= 0 && baseY <= size.height) {
      canvas.drawLine(Offset(0, baseY), Offset(size.width, baseY), basePaint);
    }

    // Unscored prefix stroke at 40% opacity
    if (scoredStart > 0) {
      final dimPaint = Paint()
        ..color = lineColor.withAlpha((0.4 * 255).round())
        ..strokeWidth = 2
        ..style = PaintingStyle.stroke
        ..strokeJoin = StrokeJoin.round;
      final dimPath = Path()..moveTo(pointAt(0).dx, pointAt(0).dy);
      for (var i = 1; i <= scoredStart; i++) {
        final p = pointAt(i);
        dimPath.lineTo(p.dx, p.dy);
      }
      canvas.drawPath(dimPath, dimPaint);
    }

    // Scored window stroke (full opacity)
    final linePaint = Paint()
      ..color = lineColor
      ..strokeWidth = 2
      ..style = PaintingStyle.stroke
      ..strokeJoin = StrokeJoin.round;
    final path = Path()
      ..moveTo(pointAt(scoredStart).dx, pointAt(scoredStart).dy);
    for (var i = scoredStart + 1; i < points.length; i++) {
      final p = pointAt(i);
      path.lineTo(p.dx, p.dy);
    }
    canvas.drawPath(path, linePaint);

    // OLS regression over the scored window
    final win = windowBars > 0 ? math.min(windowBars, points.length) : 0;
    if (win >= 2) {
      final from = points.length - win;
      final ols = olsFit(values.sublist(from));
      if (ols != null) {
        final (slope, intercept) = ols;
        final y0 = intercept;
        final y1 = intercept + slope * (win - 1);
        final p0 = Offset(pointAt(from).dx, yFor(y0));
        final p1 = Offset(pointAt(points.length - 1).dx, yFor(y1));
        final regPaint = Paint()
          ..color = regressionColor
          ..strokeWidth = 1.5
          ..style = PaintingStyle.stroke;
        if (solidRegression) {
          canvas.drawLine(p0, p1, regPaint);
        } else {
          _drawDashedLine(canvas, p0, p1, regPaint);
        }
      }
    }

    canvas.restore();
  }

  void _drawDashedLine(Canvas canvas, Offset a, Offset b, Paint paint) {
    final path = Path()
      ..moveTo(a.dx, a.dy)
      ..lineTo(b.dx, b.dy);
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      const dash = 6.0;
      const gap = 4.0;
      while (distance < metric.length) {
        final next = math.min(distance + dash, metric.length);
        canvas.drawPath(metric.extractPath(distance, next), paint);
        distance = next + gap;
      }
    }
  }

  @override
  bool shouldRepaint(covariant CompositeChartPainter other) {
    return other.points != points ||
        other.lineColor != lineColor ||
        other.regressionColor != regressionColor ||
        other.windowBars != windowBars ||
        other.solidRegression != solidRegression;
  }
}

/// OLS slope and intercept for indexed values `y = intercept + slope * i`.
/// Returns null when the series is too short. A flat series yields slope 0.
(double slope, double intercept)? olsFit(List<double> y) {
  final n = y.length;
  if (n < 2) return null;
  var sumX = 0.0, sumY = 0.0, sumXY = 0.0, sumXX = 0.0;
  for (var i = 0; i < n; i++) {
    final x = i.toDouble();
    sumX += x;
    sumY += y[i];
    sumXY += x * y[i];
    sumXX += x * x;
  }
  final denom = n * sumXX - sumX * sumX;
  if (denom == 0) return null;
  final slope = (n * sumXY - sumX * sumY) / denom;
  final intercept = (sumY - slope * sumX) / n;
  return (slope, intercept);
}
