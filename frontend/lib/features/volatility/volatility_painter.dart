import 'package:flutter/material.dart';

import 'volatility_model.dart';

/// Custom painter that renders intraday volatility as bars around a
/// centre baseline (normalized = 1.0), aligned 1:1 with chart candles.
///
/// Bars above the baseline indicate above-average activity; below
/// indicates quiet periods. Colour encodes spike probability:
/// green (low), yellow (moderate), red (high).
class VolatilityPainter extends CustomPainter {
  /// One entry per candle (may be null when no bucket matches).
  final List<VolatilityBucket?> aligned;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  VolatilityPainter({
    required this.aligned,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (aligned.isEmpty || startIndex >= endIndex) return;

    // Compute spike_prob percentiles from visible data for adaptive coloring.
    final spikes = <double>[];
    for (var i = startIndex; i < endIndex && i < aligned.length; i++) {
      final b = aligned[i];
      if (b != null) spikes.add(b.spikeProb);
    }
    spikes.sort();
    final p33 = spikes.isNotEmpty ? spikes[(spikes.length * 0.33).floor()] : 0.0;
    final p66 = spikes.isNotEmpty ? spikes[(spikes.length * 0.66).floor().clamp(0, spikes.length - 1)] : 0.0;

    final baselineY = size.height / 2;
    final paint = Paint()..style = PaintingStyle.fill;

    for (var i = startIndex; i < endIndex && i < aligned.length; i++) {
      final b = aligned[i];
      if (b == null) continue;

      final cx =
          (i - startIndex) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      final barW = candleWidth * 0.7;

      // Clamp extreme deviations to prevent visual explosion.
      final deviation = (b.normalized - 1.0).clamp(-1.5, 1.5);
      final barHeight = deviation.abs() * size.height * 0.45 * 2.5;

      final top = deviation >= 0 ? baselineY - barHeight : baselineY;
      final bottom = deviation >= 0 ? baselineY : baselineY + barHeight;

      paint.color = _colorFromSpike(b.spikeProb, p33, p66);

      canvas.drawRect(
        Rect.fromLTRB(cx - barW / 2, top, cx + barW / 2, bottom),
        paint,
      );
    }

    // Draw baseline.
    final baselinePaint = Paint()
      ..color = const Color(0x33FFFFFF)
      ..strokeWidth = 0.5;

    canvas.drawLine(
      Offset(0, baselineY),
      Offset(size.width, baselineY),
      baselinePaint,
    );
  }

  /// Adaptive color: thresholds are the 33rd/66th percentiles of visible data.
  static Color _colorFromSpike(double spike, double p33, double p66) {
    if (spike <= p33) {
      return const Color(0xCC4CAF50); // green
    } else if (spike <= p66) {
      return const Color(0xD9FFEB3B); // yellow
    } else {
      return const Color(0xE6F44336); // red
    }
  }

  @override
  bool shouldRepaint(covariant VolatilityPainter old) =>
      old.startIndex != startIndex ||
      old.endIndex != endIndex ||
      old.candleWidth != candleWidth ||
      old.scrollPixelOffset != scrollPixelOffset ||
      !identical(old.aligned, aligned);
}
