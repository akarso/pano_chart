import 'package:flutter/material.dart';

/// Paints behavioral indicator lines (Greed, Fear, Patience, Panic)
/// in a dedicated oscillator panel.
///
/// All four dimensions use a fixed 0–100 scale, analogous to RSI.
/// Colours are chosen to avoid collision with classic indicators
/// (EMA blue/orange, RSI purple, ATR teal).
class BehaviorOscillatorPainter extends CustomPainter {
  final List<double>? greed;
  final List<double>? fear;
  final List<double>? patience;
  final List<double>? panic;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  // Distinct colours that avoid the classic indicator palette.
  static const Color greedColor = Color(0xFFFFD54F); // amber / gold
  static const Color fearColor = Color(0xFFFF7043); // coral / deep orange
  static const Color patienceColor = Color(0xFF4DD0E1); // cyan
  static const Color panicColor = Color(0xFFEC407A); // hot pink

  static const Color _levelColor = Color(0x33FFFFFF);

  /// Extra candles behind the viewport for smooth indicator entry.
  static const int _indicatorPad = 60;

  BehaviorOscillatorPainter({
    this.greed,
    this.fear,
    this.patience,
    this.panic,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final h = size.height;
    final w = size.width;

    // Reference levels at 25, 50, 75.
    final levelPaint = Paint()
      ..color = _levelColor
      ..strokeWidth = 0.5;
    final dashes = <double>[4, 4];
    for (final level in [25.0, 50.0, 75.0]) {
      final y = h * (1 - level / 100);
      _drawDashedLine(canvas, Offset(0, y), Offset(w, y), levelPaint, dashes);
    }

    if (greed != null) {
      _drawLine(canvas, size, greed!, startIndex, endIndex,
          color: greedColor);
    }
    if (fear != null) {
      _drawLine(canvas, size, fear!, startIndex, endIndex,
          color: fearColor);
    }
    if (patience != null) {
      _drawLine(canvas, size, patience!, startIndex, endIndex,
          color: patienceColor);
    }
    if (panic != null) {
      _drawLine(canvas, size, panic!, startIndex, endIndex,
          color: panicColor);
    }
  }

  void _drawLine(
    Canvas canvas,
    Size size,
    List<double> values,
    int start,
    int end, {
    required Color color,
  }) {
    const minVal = 0.0;
    const maxVal = 100.0;
    const range = maxVal - minVal;
    final path = Path();
    bool started = false;

    final loopStart = (start - _indicatorPad).clamp(0, values.length);
    for (var i = loopStart; i < end && i < values.length; i++) {
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
  bool shouldRepaint(covariant BehaviorOscillatorPainter old) =>
      old.startIndex != startIndex ||
      old.endIndex != endIndex ||
      old.candleWidth != candleWidth ||
      old.scrollPixelOffset != scrollPixelOffset ||
      !identical(old.greed, greed) ||
      !identical(old.fear, fear) ||
      !identical(old.patience, patience) ||
      !identical(old.panic, panic);
}
