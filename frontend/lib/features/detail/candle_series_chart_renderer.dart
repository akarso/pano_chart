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
  CandleChartPainter(this.series) : super(repaint: null);

  static const int _gridLines = 5;
  static const double _priceScaleWidth = 48.0;
  static const double _gridColor = 0.12; // white alpha fraction

  @override
  void paint(Canvas canvas, Size size) {
    final candles = series.candles;
    if (candles.isEmpty) return;

    final min = candles.map((c) => c.low).reduce((a, b) => a < b ? a : b);
    final max = candles.map((c) => c.high).reduce((a, b) => a > b ? a : b);
    final range = (max - min) == 0 ? 1.0 : (max - min);
    final pad =
        size.height * CandleSeriesChartRenderer._verticalPaddingFraction;
    final chartWidth = size.width - _priceScaleWidth;
    final chartHeight = size.height - 2 * pad;

    // ── Horizontal grid lines + right price labels ──
    final gridPaint = Paint()
      ..color = Colors.white.withAlpha((_gridColor * 255).round())
      ..strokeWidth = 0.5;
    for (var i = 0; i <= _gridLines; i++) {
      final frac = i / _gridLines;
      final y = pad + chartHeight * frac;
      canvas.drawLine(Offset(0, y), Offset(chartWidth, y), gridPaint);
      // Price label on right
      final price = max - frac * range;
      final tp = TextPainter(
        text: TextSpan(
          text: _formatPrice(price),
          style: TextStyle(
            color: Colors.white.withAlpha((0.45 * 255).round()),
            fontSize: 9,
          ),
        ),
        textDirection: TextDirection.ltr,
      )..layout();
      tp.paint(canvas, Offset(chartWidth + 4, y - tp.height / 2));
    }

    // ── Candles ──
    final candleWidth = chartWidth / candles.length;
    final wickWidth = (candleWidth * 0.08).clamp(1.0, 2.5);

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

      // Wick
      final wickPaint = Paint()
        ..color = color
        ..strokeWidth = wickWidth
        ..style = PaintingStyle.stroke;
      canvas.drawLine(Offset(x, highY), Offset(x, lowY), wickPaint);

      // Body
      final bodyPaint = Paint()
        ..color = color
        ..style = PaintingStyle.fill;
      final left = x - candleWidth * 0.35;
      final right = x + candleWidth * 0.35;
      final top = up ? closeY : openY;
      final bottom = up ? openY : closeY;
      final bodyHeight = (bottom - top).abs();
      final rect = Rect.fromLTRB(
        left,
        top,
        right,
        bodyHeight < 1 ? top + 1 : bottom,
      );
      canvas.drawRect(rect, bodyPaint);
    }
  }

  String _formatPrice(double price) {
    if (price >= 1000) return price.toStringAsFixed(0);
    if (price >= 1) return price.toStringAsFixed(2);
    if (price >= 0.01) return price.toStringAsFixed(4);
    return price.toStringAsFixed(6);
  }

  @override
  bool shouldRepaint(covariant CandleChartPainter oldDelegate) {
    return !identical(series, oldDelegate.series);
  }
}
