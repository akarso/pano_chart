import 'package:flutter/material.dart';

import 'volatility_model.dart';
import 'volatility_painter.dart';

/// Standalone volatility bar chart for use outside the main interactive
/// chart (e.g. previews or summaries). Bars above the centre line mean
/// above-average activity; colour encodes spike risk (green → yellow → red).
class VolatilityWidget extends StatelessWidget {
  /// Volatility buckets aligned to visible candles—one entry per bar.
  final List<VolatilityBucket?> bars;

  const VolatilityWidget({
    super.key,
    required this.bars,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 60,
      child: LayoutBuilder(builder: (context, constraints) {
        final w = constraints.maxWidth;
        final cw = bars.isEmpty ? 1.0 : w / bars.length;
        return CustomPaint(
          size: Size(w, 60),
          painter: VolatilityPainter(
            aligned: bars,
            startIndex: 0,
            endIndex: bars.length,
            candleWidth: cw,
            scrollPixelOffset: 0,
          ),
        );
      }),
    );
  }
}
