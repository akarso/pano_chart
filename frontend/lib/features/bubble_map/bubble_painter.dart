import 'dart:math' as math;
import 'package:flutter/material.dart';

import 'bubble_packer.dart';

/// Paints bubble circles with size, colour, optional regime border and symbol
/// labels using [Canvas] directly for performance.
class BubblePainter extends CustomPainter {
  final List<PackedBubble> bubbles;

  /// Index of the currently highlighted (tapped) bubble, or -1.
  final int highlightIndex;

  /// Optional per-bubble rotation angles in radians (physics mode).
  /// When null or empty, no rotation is applied.
  final List<double>? angles;

  BubblePainter({
    required this.bubbles,
    this.highlightIndex = -1,
    this.angles,
  });

  // ---- colour helpers ----

  /// Maps a price change percentage to a colour on a red–grey–green scale.
  ///
  /// Values are clamped to ±10 % to avoid uniform saturation during extreme
  /// market moves (spec §11.1).
  static Color colorForChange(double change) {
    const maxChange = 10.0;
    final clamped = change.clamp(-maxChange, maxChange);
    final t = (clamped.abs() / maxChange).clamp(0.0, 1.0);

    if (change > 0.2) {
      return Color.lerp(const Color(0xFF555555), const Color(0xFF00C853), t)!;
    } else if (change < -0.2) {
      return Color.lerp(const Color(0xFF555555), const Color(0xFFFF1744), t)!;
    }
    return const Color(0xFF555555);
  }

  /// Returns a border colour for the dominant regime, or null if no regime
  /// qualifies (`score < 0.5`).
  static Color? regimeBorderColor(String badgeComponent) {
    switch (badgeComponent) {
      case 'sideways':
        return const Color(0xFF42A5F5); // blue
      case 'compression':
        return const Color(0xFFFFD600); // yellow
      case 'breakout':
        return const Color(0xFFAB47BC); // purple
      default:
        return null;
    }
  }

  // ---- painting ----

  @override
  void paint(Canvas canvas, Size size) {
    final fillPaint = Paint()..style = PaintingStyle.fill;
    final borderPaint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2.0;

    for (var i = 0; i < bubbles.length; i++) {
      final b = bubbles[i];
      final angle = (angles != null && i < angles!.length) ? angles![i] : 0.0;
      final hasRotation = angle != 0.0;

      if (hasRotation) {
        canvas.save();
        canvas.translate(b.x, b.y);
        canvas.rotate(angle);
        canvas.translate(-b.x, -b.y);
      }

      // Fill — colour driven by pre-computed colorValue.
      fillPaint.color = colorForChange(b.colorValue);
      canvas.drawCircle(Offset(b.x, b.y), b.radius, fillPaint);

      // Optional regime border
      final borderColor = regimeBorderColor(b.token.badgeComponent);
      if (borderColor != null) {
        borderPaint.color = borderColor;
        canvas.drawCircle(Offset(b.x, b.y), b.radius, borderPaint);
      }

      // Highlight ring on tap
      if (i == highlightIndex) {
        final hlPaint = Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 3.0
          ..color = Colors.white;
        canvas.drawCircle(Offset(b.x, b.y), b.radius + 2, hlPaint);
      }

      // Symbol label (strip USDT suffix for readability)
      _drawLabel(canvas, b);

      if (hasRotation) {
        canvas.restore();
      }
    }
  }

  void _drawLabel(Canvas canvas, PackedBubble b) {
    var label = b.token.symbol;
    if (label.endsWith('USDT')) {
      label = label.substring(0, label.length - 4);
    }

    // Sub-label driven by colorValue (price change % or normalised volume).
    final changeSign = b.colorValue >= 0 ? '+' : '';
    final changeLine = '$changeSign${b.colorValue.toStringAsFixed(1)}%';

    // Scale font to fit inside the bubble; allow very small sizes so every
    // bubble gets a label.
    final labelSize = math.max(5.0, b.radius * 0.34);
    final changeSize = math.max(4.0, b.radius * 0.24);

    final labelSpan = TextSpan(
      text: label,
      style: TextStyle(
        color: Colors.white,
        fontSize: labelSize,
        fontWeight: FontWeight.w700,
      ),
    );
    final changeSpan = TextSpan(
      text: changeLine,
      style: TextStyle(
        color: Colors.white70,
        fontSize: changeSize,
        fontWeight: FontWeight.w500,
      ),
    );

    final labelPainter = TextPainter(
      text: labelSpan,
      textAlign: TextAlign.center,
      textDirection: TextDirection.ltr,
    )..layout();

    final changePainter = TextPainter(
      text: changeSpan,
      textAlign: TextAlign.center,
      textDirection: TextDirection.ltr,
    )..layout();

    final gap = 2.0;
    final totalH = labelPainter.height + gap + changePainter.height;

    labelPainter.paint(
      canvas,
      Offset(b.x - labelPainter.width / 2, b.y - totalH / 2),
    );
    changePainter.paint(
      canvas,
      Offset(
        b.x - changePainter.width / 2,
        b.y - totalH / 2 + labelPainter.height + gap,
      ),
    );
  }

  @override
  bool shouldRepaint(covariant BubblePainter oldDelegate) {
    return oldDelegate.bubbles != bubbles ||
        oldDelegate.highlightIndex != highlightIndex ||
        oldDelegate.angles != angles;
  }
}
