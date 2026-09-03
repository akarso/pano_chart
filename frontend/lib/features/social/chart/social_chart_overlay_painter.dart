import 'package:flutter/material.dart';

import '../api/social_models.dart';

/// Describes a group of social posts at one X position on the chart.
class SocialMarker {
  final double x;
  final DateTime timestamp;
  final List<SocialPost> posts;

  const SocialMarker({
    required this.x,
    required this.timestamp,
    required this.posts,
  });
}

/// Blue social-post theme color.
const Color kSocialBlue = Color(0xFF42A5F5);

/// Paints vertical social-post lines on top of the candle chart.
///
/// Each distinct timestamp gets a vertical blue line. If multiple posts
/// share a timestamp they group and show a count badge.
class SocialChartOverlayPainter extends CustomPainter {
  final List<SocialMarker> markers;
  final double? priceAreaHeight;

  SocialChartOverlayPainter({required this.markers, this.priceAreaHeight});

  @override
  void paint(Canvas canvas, Size size) {
    for (final marker in markers) {
      if (marker.x < 0 || marker.x > size.width) continue;
      _drawMarker(canvas, marker, size);
    }
  }

  void _drawMarker(Canvas canvas, SocialMarker marker, Size size) {
    final lineBottom = priceAreaHeight ?? size.height;
    final paint = Paint()
      ..color = kSocialBlue.withAlpha(50)
      ..strokeWidth = 1.0;

    canvas.drawLine(
      Offset(marker.x, 0),
      Offset(marker.x, lineBottom),
      paint,
    );

    if (marker.posts.length > 1) {
      _drawCountBadge(canvas, marker.x, marker.posts.length);
    } else {
      canvas.drawCircle(
        Offset(marker.x, 6),
        3,
        Paint()..color = kSocialBlue.withAlpha(160),
      );
    }
  }

  void _drawCountBadge(Canvas canvas, double x, int count) {
    final badgePaint = Paint()..color = kSocialBlue.withAlpha(180);
    canvas.drawCircle(Offset(x, 8), 7, badgePaint);

    final tp = TextPainter(
      text: TextSpan(
        text: '$count',
        style: const TextStyle(
          color: Colors.white,
          fontSize: 8,
          fontWeight: FontWeight.bold,
        ),
      ),
      textDirection: TextDirection.ltr,
    )..layout();
    tp.paint(canvas, Offset(x - tp.width / 2, 8 - tp.height / 2));
  }

  @override
  bool shouldRepaint(covariant SocialChartOverlayPainter old) {
    return !identical(markers, old.markers) ||
        priceAreaHeight != old.priceAreaHeight;
  }
}
