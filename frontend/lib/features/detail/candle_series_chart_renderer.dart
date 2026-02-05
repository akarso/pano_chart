import 'package:flutter/material.dart';
import '../../domain/series_view_mode.dart';
import '../candles/api/candle_response.dart';
import '../overview/series_chart_renderer.dart';

/// Renders a candlestick chart for a CandleSeries using full OHLC data.
/// Only supports SeriesViewMode.candles.
class CandleSeriesChartRenderer implements SeriesChartRenderer {
  static const double _verticalPaddingFraction = 0.08;
  static const Color _upColor = Colors.green;
  static const Color _downColor = Colors.red;

  @override
  Widget build(
    BuildContext context, {
    required CandleSeriesResponse series,
    required SeriesViewMode viewMode,
  }) {
    assert(viewMode == SeriesViewMode.candles,
        'CandleSeriesChartRenderer only supports SeriesViewMode.candles');
    return CustomPaint(
      painter: CandleChartPainter(series),
      size: Size.infinite,
    );
  }
}

class CandleChartPainter extends CustomPainter {
  final CandleSeriesResponse series;
  CandleChartPainter(this.series);

  @override
  void paint(Canvas canvas, Size size) {
    final candles = series.candles;
    if (candles.isEmpty) return;

    final min = candles.map((c) => c.low).reduce((a, b) => a < b ? a : b);
    final max = candles.map((c) => c.high).reduce((a, b) => a > b ? a : b);
    final range = (max - min) == 0 ? 1.0 : (max - min);
    final pad =
        size.height * CandleSeriesChartRenderer._verticalPaddingFraction;
    final chartHeight = size.height - 2 * pad;
    final candleWidth = size.width / candles.length;

    for (var i = 0; i < candles.length; i++) {
      final c = candles[i];
      final x = i * candleWidth + candleWidth / 2;
      final openY = pad + chartHeight * (1 - (c.open - min) / range);
      final closeY = pad + chartHeight * (1 - (c.close - min) / range);
      final highY = pad + chartHeight * (1 - (c.high - min) / range);
      final lowY = pad + chartHeight * (1 - (c.low - min) / range);
      final up = c.close >= c.open;
      final color = up
          ? CandleSeriesChartRenderer._upColor
          : CandleSeriesChartRenderer._downColor;
      final paint = Paint()
        ..color = color
        ..strokeWidth = 2.0
        ..style = PaintingStyle.stroke;
      // Wick
      canvas.drawLine(Offset(x, highY), Offset(x, lowY), paint);
      // Body
      final bodyPaint = Paint()
        ..color = color
        ..style = PaintingStyle.fill;
      final left = x - candleWidth * 0.3;
      final right = x + candleWidth * 0.3;
      final top = up ? openY : closeY;
      final bottom = up ? closeY : openY;
      final rect = Rect.fromLTRB(left, top, right, bottom);
      canvas.drawRect(rect, bodyPaint);
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
