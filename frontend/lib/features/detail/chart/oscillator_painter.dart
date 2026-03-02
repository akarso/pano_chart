import 'package:flutter/material.dart';

/// Paints RSI and optional ATR lines in the oscillator panel.
///
/// RSI and ATR share the same vertical space.  RSI uses a fixed 0–100 scale;
/// ATR auto-scales to its visible range.
class OscillatorPainter extends CustomPainter {
  final List<double>? rsi;
  final List<double>? atr;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  static const Color _rsiColor = Color(0xFFAB47BC);
  static const Color _atrColor = Color(0xFF26A69A);
  static const Color _levelColor = Color(0x33FFFFFF);

  OscillatorPainter({
    this.rsi,
    this.atr,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final h = size.height;
    final w = size.width;

    // ── RSI reference levels (30, 50, 70) ──
    if (rsi != null) {
      final levelPaint = Paint()
        ..color = _levelColor
        ..strokeWidth = 0.5;
      final dashes = <double>[4, 4];
      for (final level in [30.0, 50.0, 70.0]) {
        final y = h * (1 - level / 100);
        _drawDashedLine(canvas, Offset(0, y), Offset(w, y), levelPaint, dashes);
      }
    }

    // ── RSI line ──
    if (rsi != null) {
      _drawLine(
        canvas, size, rsi!, startIndex, endIndex,
        color: _rsiColor,
        minVal: 0,
        maxVal: 100,
      );
    }

    // ── ATR line (auto-scaled) ──
    if (atr != null) {
      double lo = double.infinity, hi = double.negativeInfinity;
      for (var i = startIndex; i < endIndex && i < atr!.length; i++) {
        final v = atr![i];
        if (v.isNaN) continue;
        if (v < lo) lo = v;
        if (v > hi) hi = v;
      }
      if (lo < hi) {
        _drawLine(
          canvas, size, atr!, startIndex, endIndex,
          color: _atrColor,
          minVal: lo,
          maxVal: hi,
        );
      }
    }
  }

  void _drawLine(
    Canvas canvas,
    Size size,
    List<double> values,
    int start,
    int end, {
    required Color color,
    required double minVal,
    required double maxVal,
  }) {
    final range = maxVal - minVal;
    if (range == 0) return;
    final path = Path();
    bool started = false;

    for (var i = start; i < end && i < values.length; i++) {
      final v = values[i];
      if (v.isNaN) continue;
      final cx =
          (i - start) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      final y = size.height * (1 - (v - minVal) / range);
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

  void _drawDashedLine(
      Canvas canvas, Offset from, Offset to, Paint paint, List<double> dash) {
    final dx = to.dx - from.dx;
    final dy = to.dy - from.dy;
    final len = (dx * dx + dy * dy);
    if (len == 0) return;
    // Simple approximation for horizontal lines.
    var x = from.dx;
    var drawing = true;
    var dashIdx = 0;
    while (x < to.dx) {
      final seg = dash[dashIdx % dash.length];
      final end = (x + seg).clamp(from.dx, to.dx);
      if (drawing) {
        canvas.drawLine(Offset(x, from.dy), Offset(end, from.dy), paint);
      }
      x = end;
      drawing = !drawing;
      dashIdx++;
    }
  }

  @override
  bool shouldRepaint(covariant OscillatorPainter old) =>
      old.startIndex != startIndex ||
      old.endIndex != endIndex ||
      old.candleWidth != candleWidth ||
      old.scrollPixelOffset != scrollPixelOffset ||
      !identical(old.rsi, rsi) ||
      !identical(old.atr, atr);
}
