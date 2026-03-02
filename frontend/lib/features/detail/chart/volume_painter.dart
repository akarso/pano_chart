import 'package:flutter/material.dart';

import '../../candles/api/candle_response.dart';

/// Paints volume bars aligned with candles, normalised to the visible window.
class VolumePainter extends CustomPainter {
  final List<CandleDto> candles;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  static const Color _upColor = Color(0x6600C853);
  static const Color _downColor = Color(0x66FF1744);

  VolumePainter({
    required this.candles,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (candles.isEmpty || startIndex >= endIndex) return;

    // Normalise volume to visible range.
    double maxVol = 0;
    for (var i = startIndex; i < endIndex && i < candles.length; i++) {
      if (candles[i].volume > maxVol) maxVol = candles[i].volume;
    }
    if (maxVol == 0) return;

    final paint = Paint()..style = PaintingStyle.fill;

    for (var i = startIndex; i < endIndex && i < candles.length; i++) {
      final c = candles[i];
      final cx =
          (i - startIndex) * candleWidth + candleWidth / 2 - scrollPixelOffset;
      final barW = candleWidth * 0.6;
      final barH = (c.volume / maxVol) * size.height * 0.9;
      final up = c.close >= c.open;
      paint.color = up ? _upColor : _downColor;

      canvas.drawRect(
        Rect.fromLTWH(cx - barW / 2, size.height - barH, barW, barH),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant VolumePainter old) =>
      old.startIndex != startIndex ||
      old.endIndex != endIndex ||
      old.candleWidth != candleWidth ||
      old.scrollPixelOffset != scrollPixelOffset ||
      !identical(old.candles, candles);
}
