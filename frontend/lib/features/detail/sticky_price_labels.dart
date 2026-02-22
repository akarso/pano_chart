import 'package:flutter/material.dart';
import '../candles/api/candle_response.dart';

class StickyPriceLabels extends StatelessWidget {
  final CandleSeriesResponse series;
  const StickyPriceLabels({required this.series});

  @override
  Widget build(BuildContext context) {
    final candles = series.candles;
    if (candles.isEmpty) return SizedBox.shrink();
    final min = candles.map((c) => c.low).reduce((a, b) => a < b ? a : b);
    final max = candles.map((c) => c.high).reduce((a, b) => a > b ? a : b);
    final range = (max - min) == 0 ? 1.0 : (max - min);
    const int gridLines = 5;
    const double chartHeight = 220;
    const double padFrac = 0.08;
    final pad = chartHeight * padFrac;
    final usableHeight = chartHeight - 2 * pad;
    List<Widget> labels = [];
    for (var i = 0; i <= gridLines; i++) {
      final frac = i / gridLines;
      final y = pad + usableHeight * frac;
      final price = max - frac * range;
      labels.add(Positioned(
        top: y +8,
        right: 0,
        child: Container(
          color: Colors.transparent,
          child: Text(
            _formatPrice(price),
            style: TextStyle(
              color: Colors.white.withAlpha((0.45 * 255).round()),
              fontSize: 9,
            ),
          ),
        ),
      ));
    }
    return Stack(children: labels);
  }

  String _formatPrice(double price) {
    if (price >= 1000) return price.toStringAsFixed(0);
    if (price >= 1) return price.toStringAsFixed(2);
    if (price >= 0.01) return price.toStringAsFixed(4);
    return price.toStringAsFixed(6);
  }
}