import 'package:flutter/material.dart';
import '../../domain/series_view_mode.dart';
import '../candles/api/candle_response.dart';
import 'series_chart_renderer.dart';

/// Renders a line chart for a CandleSeries using close prices only.
/// Only supports SeriesViewMode.line.
class LineSeriesChartRenderer implements SeriesChartRenderer {
  static const double _verticalPaddingFraction = 0.08; // 8% top/bottom
  static const Color _lineColor = Colors.blueGrey;

  @override
  Widget build(
    BuildContext context, {
    required CandleSeriesResponse series,
    required SeriesViewMode viewMode,
  }) {
    assert(viewMode == SeriesViewMode.line,
        'LineSeriesChartRenderer only supports SeriesViewMode.line');
    return CustomPaint(
      painter: _LineChartPainter(series),
      size: Size.infinite,
    );
  }
}

class _LineChartPainter extends CustomPainter {
  final CandleSeriesResponse series;
  _LineChartPainter(this.series);

  @override
  void paint(Canvas canvas, Size size) {
    final closes = series.candles.map((c) => c.close).toList();
    if (closes.length < 2) return;

    final min = closes.reduce((a, b) => a < b ? a : b);
    final max = closes.reduce((a, b) => a > b ? a : b);
    final range = (max - min) == 0 ? 1.0 : (max - min);
    final pad = size.height * LineSeriesChartRenderer._verticalPaddingFraction;
    final chartHeight = size.height - 2 * pad;

    final points = <Offset>[];
    for (var i = 0; i < closes.length; i++) {
      final x = i * size.width / (closes.length - 1);
      final norm = (closes[i] - min) / range;
      final y = pad + chartHeight * (1 - norm);
      points.add(Offset(x, y));
    }

    final paint = Paint()
      ..color = LineSeriesChartRenderer._lineColor
      ..strokeWidth = 2.0
      ..style = PaintingStyle.stroke;

    final path = Path()..moveTo(points[0].dx, points[0].dy);
    for (var i = 1; i < points.length; i++) {
      path.lineTo(points[i].dx, points[i].dy);
    }
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
