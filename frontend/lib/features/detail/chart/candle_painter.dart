import 'dart:math' as math;
import 'package:flutter/material.dart';

import '../../candles/api/candle_response.dart';

/// Paints candlesticks and optional EMA cloud overlay.
///
/// Renders only the visible portion defined by [startIndex] / [endIndex].
/// Y axis is auto-scaled to the visible range.
class CandlePainter extends CustomPainter {
  final List<CandleDto> candles;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  /// Pre-computed EMA values (same length as [candles]).  May be null.
  final List<double>? emaFast;
  final List<double>? emaSlow;

  /// Optional externally-computed price range (e.g. with vertical scaling).
  /// When provided, overrides auto-scaling from visible candles.
  final double? priceLo;
  final double? priceHi;

  static const Color upColor = Color(0xFF00C853);
  static const Color downColor = Color(0xFFFF1744);
  static const double _padFrac = 0.06;

  /// Extra candles behind the viewport for smooth indicator entry.
  static const int _indicatorPad = 60;

  CandlePainter({
    required this.candles,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
    this.emaFast,
    this.emaSlow,
    this.priceLo,
    this.priceHi,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (candles.isEmpty || startIndex >= endIndex) return;

    final h = size.height;
    final pad = h * _padFrac;
    final chartH = h - 2 * pad;

    // Visible min/max for auto-scaling Y (or use externally provided range).
    double lo, hi;
    if (priceLo != null && priceHi != null) {
      lo = priceLo!;
      hi = priceHi!;
    } else {
      lo = double.infinity;
      hi = double.negativeInfinity;
      for (var i = startIndex; i < endIndex && i < candles.length; i++) {
        if (candles[i].low < lo) lo = candles[i].low;
        if (candles[i].high > hi) hi = candles[i].high;
      }
      _expandRange(emaFast, startIndex, endIndex, (v) {
        if (v < lo) lo = v;
        if (v > hi) hi = v;
      });
      _expandRange(emaSlow, startIndex, endIndex, (v) {
        if (v < lo) lo = v;
        if (v > hi) hi = v;
      });
    }

    final range = (hi - lo) == 0 ? 1.0 : (hi - lo);
    double toY(double price) => pad + chartH * (1 - (price - lo) / range);

    // ── Grid lines ──
    _drawGrid(canvas, size, pad, chartH, lo, range);

    // ── EMA cloud ──
    if (emaFast != null && emaSlow != null) {
      _drawEmaCloud(canvas, size, toY);
    }

    // ── EMA lines ──
    if (emaFast != null) _drawEmaLine(canvas, toY, emaFast!, const Color(0xFF42A5F5));
    if (emaSlow != null) _drawEmaLine(canvas, toY, emaSlow!, const Color(0xFFFFAB40));

    // ── Candles ──
    final wickPaint = Paint()..style = PaintingStyle.stroke;
    final bodyPaint = Paint()..style = PaintingStyle.fill;

    for (var i = startIndex; i < endIndex && i < candles.length; i++) {
      final c = candles[i];
      final cx = (i - startIndex) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      final up = c.close >= c.open;
      final color = up ? upColor : downColor;

      // Wick
      wickPaint.color = color;
      wickPaint.strokeWidth = math.max(1.0, candleWidth * 0.08).clamp(1.0, 3.0);
      canvas.drawLine(Offset(cx, toY(c.high)), Offset(cx, toY(c.low)), wickPaint);

      // Body
      bodyPaint.color = color;
      final bodyW = candleWidth * 0.7;
      final top = toY(up ? c.close : c.open);
      final bottom = toY(up ? c.open : c.close);
      final bodyH = (bottom - top).abs();
      canvas.drawRect(
        Rect.fromLTWH(cx - bodyW / 2, top, bodyW, bodyH < 1 ? 1 : bodyH),
        bodyPaint,
      );
    }
  }

  void _drawGrid(Canvas canvas, Size size, double pad, double chartH,
      double lo, double range) {
    const lines = 5;
    final gridPaint = Paint()
      ..color = const Color(0x1EFFFFFF)
      ..strokeWidth = 0.5;
    for (var i = 0; i <= lines; i++) {
      final y = pad + chartH * i / lines;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), gridPaint);
    }
  }

  void _drawEmaCloud(Canvas canvas, Size size, double Function(double) toY) {
    final fast = emaFast!;
    final slow = emaSlow!;
    final path = Path();
    final topPoints = <Offset>[];
    final bottomPoints = <Offset>[];

    final loopStart = (startIndex - _indicatorPad).clamp(0, candles.length);
    for (var i = loopStart; i < endIndex && i < candles.length; i++) {
      if (i >= fast.length || i >= slow.length) continue;
      final fv = fast[i];
      final sv = slow[i];
      if (fv.isNaN || sv.isNaN) continue;

      final cx = (i - startIndex) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      topPoints.add(Offset(cx, toY(math.max(fv, sv))));
      bottomPoints.add(Offset(cx, toY(math.min(fv, sv))));
    }

    if (topPoints.length < 2) return;

    path.moveTo(topPoints.first.dx, topPoints.first.dy);
    for (final p in topPoints.skip(1)) {
      path.lineTo(p.dx, p.dy);
    }
    for (final p in bottomPoints.reversed) {
      path.lineTo(p.dx, p.dy);
    }
    path.close();

    // Colour based on fast vs slow at midpoint.
    final mid = topPoints.length ~/ 2;
    final midI = startIndex + mid;
    final bullish = midI < fast.length && midI < slow.length &&
        !fast[midI].isNaN && !slow[midI].isNaN &&
        fast[midI] > slow[midI];

    final cloudPaint = Paint()
      ..style = PaintingStyle.fill
      ..color = bullish
          ? const Color(0x1800C853)
          : const Color(0x18FF1744);
    canvas.drawPath(path, cloudPaint);
  }

  void _drawEmaLine(
      Canvas canvas, double Function(double) toY, List<double> values, Color color) {
    final path = Path();
    bool started = false;
    final loopStart = (startIndex - _indicatorPad).clamp(0, candles.length);
    for (var i = loopStart; i < endIndex && i < candles.length; i++) {
      if (i >= values.length || values[i].isNaN) continue;
      final cx = (i - startIndex) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      final y = toY(values[i]);
      if (!started) {
        path.moveTo(cx, y);
        started = true;
      } else {
        path.lineTo(cx, y);
      }
    }
    if (!started) return;
    canvas.drawPath(
      path,
      Paint()
        ..style = PaintingStyle.stroke
        ..color = color
        ..strokeWidth = 1.2,
    );
  }

  void _expandRange(List<double>? values, int start, int end,
      void Function(double) apply) {
    if (values == null) return;
    for (var i = start; i < end && i < values.length; i++) {
      if (!values[i].isNaN) apply(values[i]);
    }
  }

  @override
  bool shouldRepaint(covariant CandlePainter old) =>
      old.startIndex != startIndex ||
      old.endIndex != endIndex ||
      old.candleWidth != candleWidth ||
      old.scrollPixelOffset != scrollPixelOffset ||
      old.priceLo != priceLo ||
      old.priceHi != priceHi ||
      !identical(old.candles, candles) ||
      !identical(old.emaFast, emaFast) ||
      !identical(old.emaSlow, emaSlow);
}
