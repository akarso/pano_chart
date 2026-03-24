import 'package:flutter/material.dart';

import 'volatility_model.dart';
import 'volatility_painter.dart';

/// Compact bar chart showing intraday volatility profile below the
/// candlestick chart. Bars above the centre line mean above-average
/// activity; colour encodes spike risk (green → yellow → red).
class VolatilityWidget extends StatelessWidget {
  /// Volatility buckets aligned to visible candles—one entry per bar.
  final List<VolatilityBucket> bars;

  const VolatilityWidget({
    super.key,
    required this.bars,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 60,
      child: CustomPaint(
        size: const Size(double.infinity, 60),
        painter: VolatilityPainter(
          data: bars,
          start: 0,
          count: bars.length,
        ),
      ),
    );
  }
}
