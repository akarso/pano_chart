import 'dart:math' as math;
import 'package:flutter/material.dart';

import 'volatility_model.dart';

/// Custom painter that renders intraday volatility as bars around a
/// centre baseline (normalized = 1.0).
///
/// Bars above the baseline indicate above-average activity; below
/// indicates quiet periods. Colour encodes spike probability:
/// green (low), yellow (moderate), red (high).
class VolatilityPainter extends CustomPainter {
  final List<VolatilityBucket> data;

  /// First visible minute-of-day index into [data].
  final int start;

  /// Number of bars to render.
  final int count;

  VolatilityPainter({
    required this.data,
    required this.start,
    required this.count,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (data.isEmpty || count <= 0) return;

    final barWidth = size.width / count;
    final baselineY = size.height / 2;
    final paint = Paint();

    for (int i = 0; i < count; i++) {
      final idx = (start + i) % data.length;
      if (idx < 0 || idx >= data.length) continue;

      final b = data[idx];
      final x = i * barWidth;

      // Clamp extreme deviations to prevent visual explosion.
      final deviation = (b.normalized - 1.0).clamp(-1.5, 1.5);
      final barHeight = deviation.abs() * size.height * 0.45;

      final top = deviation >= 0 ? baselineY - barHeight : baselineY;
      final bottom = deviation >= 0 ? baselineY : baselineY + barHeight;

      paint.color = _colorFromSpike(b.spikeProb);

      canvas.drawRect(
        Rect.fromLTRB(x, top, x + barWidth * 0.9, bottom),
        paint,
      );
    }

    // Draw baseline.
    final baselinePaint = Paint()
      ..color = const Color(0x33FFFFFF) // Colors.white.withOpacity(0.2)
      ..strokeWidth = 1;

    canvas.drawLine(
      Offset(0, baselineY),
      Offset(size.width, baselineY),
      baselinePaint,
    );
  }

  static Color _colorFromSpike(double spike) {
    final s = spike.clamp(0.0, 1.0);
    if (s < 0.33) {
      return const Color(0xCC4CAF50); // green, 0.8 opacity
    } else if (s < 0.66) {
      return const Color(0xD9FFEB3B); // yellow, 0.85 opacity
    } else {
      return const Color(0xE6F44336); // red, 0.9 opacity
    }
  }

  @override
  bool shouldRepaint(covariant VolatilityPainter old) =>
      data != old.data || start != old.start || count != old.count;
}
