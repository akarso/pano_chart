import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../plan_data.dart';

/// One horizontal channel line that [PlanLevelsPainter] will draw.
class PlanChannelLine {
  final double price;
  final double y;
  final Offset start;
  final Offset end;
  final Color color;

  const PlanChannelLine({
    required this.price,
    required this.y,
    required this.start,
    required this.end,
    required this.color,
  });
}

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

  static const channelLowColor = Color(0xFF26A69A);
  static const channelMidColor = Color(0xFF90A4AE);
  static const channelHighColor = Color(0xFFEF5350);

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

  /// Channel segments (Low, Mid, High) that [paint] would draw for [size].
  /// Empty when levels are invalid or the scale is unusable.
  @visibleForTesting
  List<PlanChannelLine> channelLines(Size size) {
    if (!levels.valid) return const [];
    final range = priceHi - priceLo;
    if (range <= 0 || !range.isFinite) return const [];
    final pad = size.height * padFrac;
    final chartH = size.height - 2 * pad;
    if (chartH <= 0) return const [];

    double toY(double price) => pad + chartH * (1 - (price - priceLo) / range);
    final chartRight = size.width - yAxisWidth;

    PlanChannelLine? line(double price, Color color) {
      if (!price.isFinite) return null;
      final y = toY(price);
      if (y < -1 || y > size.height + 1) return null;
      return PlanChannelLine(
        price: price,
        y: y,
        start: Offset(0, y),
        end: Offset(chartRight, y),
        color: color,
      );
    }

    return [
      for (final item in [
        (levels.low, channelLowColor),
        (levels.mid, channelMidColor),
        (levels.high, channelHighColor),
      ])
        if (line(item.$1, item.$2) case final PlanChannelLine seg) seg,
    ];
  }

  @override
  void paint(Canvas canvas, Size size) {
    for (final seg in channelLines(size)) {
      final paint = Paint()
        ..color = seg.color
        ..strokeWidth = 1.0
        ..style = PaintingStyle.stroke;
      canvas.drawLine(seg.start, seg.end, paint);
    }
    if (!levels.valid) return;

    final range = priceHi - priceLo;
    if (range <= 0 || !range.isFinite) return;
    final pad = size.height * padFrac;
    final chartH = size.height - 2 * pad;
    if (chartH <= 0) return;

    double toY(double price) => pad + chartH * (1 - (price - priceLo) / range);
    final chartRight = size.width - yAxisWidth;

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
