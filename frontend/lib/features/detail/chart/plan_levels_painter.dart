import 'package:flutter/material.dart';

import '../plan_data.dart';

/// Draws Low / Mid / High channel lines and entry / stop / target ticks
/// for a **valid** range plan (PR-111). Callers must pass levels only when
/// `valid`; invalid plans should omit the painter entirely.
class PlanLevelsPainter extends CustomPainter {
  final PlanChartLevels levels;
  final double priceLo;
  final double priceHi;
  final double padFrac;
  final double yAxisWidth;

  // Cached label painters (allocated once per painter instance).
  static final _entryLabel = _labelPainter('E', const Color(0xFF42A5F5));
  static final _stopLabel = _labelPainter('S', const Color(0xFFFF7043));
  static final _targetLabel = _labelPainter('T', const Color(0xFF66BB6A));

  const PlanLevelsPainter({
    required this.levels,
    required this.priceLo,
    required this.priceHi,
    this.padFrac = 0.06,
    this.yAxisWidth = 44,
  });

  static TextPainter _labelPainter(String text, Color color) {
    return TextPainter(
      text: TextSpan(
        text: text,
        style: TextStyle(
          color: color,
          fontSize: 9,
          fontWeight: FontWeight.w600,
        ),
      ),
      textDirection: TextDirection.ltr,
    )..layout();
  }

  @override
  void paint(Canvas canvas, Size size) {
    if (!levels.valid) return;
    final range = priceHi - priceLo;
    if (range <= 0 || !range.isFinite) return;
    final pad = size.height * padFrac;
    final chartH = size.height - 2 * pad;
    if (chartH <= 0) return;

    double toY(double price) => pad + chartH * (1 - (price - priceLo) / range);

    final chartRight = size.width - yAxisWidth;

    void drawChannel(double price, Color color, double width) {
      if (!price.isFinite) return;
      final y = toY(price);
      if (y < -1 || y > size.height + 1) return;
      final paint = Paint()
        ..color = color
        ..strokeWidth = width
        ..style = PaintingStyle.stroke;
      canvas.drawLine(Offset(0, y), Offset(chartRight, y), paint);
    }

    drawChannel(levels.low, const Color(0xFF26A69A), 1.0);
    drawChannel(levels.mid, const Color(0xFF90A4AE), 1.0);
    drawChannel(levels.high, const Color(0xFFEF5350), 1.0);

    void drawTick(double? price, Color color, TextPainter label) {
      if (price == null || !price.isFinite) return;
      final y = toY(price);
      if (y < -1 || y > size.height + 1) return;
      final paint = Paint()
        ..color = color
        ..strokeWidth = 1.2
        ..style = PaintingStyle.stroke;
      // Tick + label sit left of the Y-axis gutter so they never overlap prices.
      final tickEnd = chartRight - 2;
      canvas.drawLine(Offset(tickEnd - 14, y), Offset(tickEnd, y), paint);
      label.paint(
        canvas,
        Offset(tickEnd - 16 - label.width, y - label.height / 2),
      );
    }

    drawTick(levels.entry, const Color(0xFF42A5F5), _entryLabel);
    drawTick(levels.stop, const Color(0xFFFF7043), _stopLabel);
    drawTick(levels.target, const Color(0xFF66BB6A), _targetLabel);
  }

  @override
  bool shouldRepaint(covariant PlanLevelsPainter old) {
    return old.levels != levels ||
        old.priceLo != priceLo ||
        old.priceHi != priceHi ||
        old.padFrac != padFrac ||
        old.yAxisWidth != yAxisWidth;
  }
}
