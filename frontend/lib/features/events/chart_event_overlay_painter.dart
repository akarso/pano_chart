import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'events_list_screen.dart' show impactColor;

/// Describes a group of events at one X position on the chart.
class EventMarker {
  final double x;
  final DateTime timestamp;
  final List<Event> events;

  /// True when this marker represents a future event (beyond the last candle).
  final bool isFuture;

  const EventMarker({
    required this.x,
    required this.timestamp,
    required this.events,
    this.isFuture = false,
  });
}

/// Paints vertical event lines on top of the candle chart.
///
/// Each distinct timestamp gets a vertical line. If multiple events
/// share a timestamp they are merged and the highest-impact colour wins.
class EventOverlayPainter extends CustomPainter {
  final List<EventMarker> markers;

  /// Height of the price area.  Past-event lines stop at this height;
  /// future-event dashed lines extend to the full canvas height (x-axis).
  final double? priceAreaHeight;

  EventOverlayPainter({required this.markers, this.priceAreaHeight});

  @override
  void paint(Canvas canvas, Size size) {
    for (final marker in markers) {
      if (marker.x < 0 || marker.x > size.width) continue;

      final color = _highestImpactColor(marker.events);

      if (marker.isFuture) {
        _drawFutureMarker(canvas, marker, color, size);
      } else {
        _drawPastMarker(canvas, marker, color, size);
      }
    }
  }

  void _drawPastMarker(
      Canvas canvas, EventMarker marker, Color color, Size size) {
    final lineBottom = priceAreaHeight ?? size.height;
    final paint = Paint()
      ..color = color.withAlpha(60)
      ..strokeWidth = 1.2;

    canvas.drawLine(
      Offset(marker.x, 0),
      Offset(marker.x, lineBottom),
      paint,
    );

    if (marker.events.length > 1) {
      _drawCountBadge(canvas, marker.x, marker.events.length, color, size);
    } else {
      canvas.drawCircle(
        Offset(marker.x, 6),
        3,
        Paint()..color = color.withAlpha(180),
      );
    }
  }

  /// Future events: dashed line at 70% opacity, muted badge/dot.
  void _drawFutureMarker(
      Canvas canvas, EventMarker marker, Color color, Size size) {
    final paint = Paint()
      ..color = color.withAlpha(42) // ~60 * 0.7
      ..strokeWidth = 1.2;

    // Dashed vertical line
    const dashLen = 4.0;
    const gapLen = 3.0;
    double y = 0;
    while (y < size.height) {
      final end = (y + dashLen).clamp(0.0, size.height);
      canvas.drawLine(Offset(marker.x, y), Offset(marker.x, end), paint);
      y += dashLen + gapLen;
    }

    if (marker.events.length > 1) {
      _drawCountBadge(
          canvas, marker.x, marker.events.length, color.withAlpha(160), size);
    } else {
      canvas.drawCircle(
        Offset(marker.x, 6),
        3,
        Paint()..color = color.withAlpha(130),
      );
    }
  }

  void _drawCountBadge(
      Canvas canvas, double x, int count, Color color, Size size) {
    final badgePaint = Paint()..color = color.withAlpha(200);
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

  Color _highestImpactColor(List<Event> events) {
    for (final e in events) {
      if (e.impact == EventImpact.high) return impactColor(EventImpact.high);
    }
    for (final e in events) {
      if (e.impact == EventImpact.medium) {
        return impactColor(EventImpact.medium);
      }
    }
    return impactColor(EventImpact.low);
  }

  @override
  bool shouldRepaint(covariant EventOverlayPainter old) {
    return !identical(markers, old.markers) ||
        priceAreaHeight != old.priceAreaHeight;
  }
}
