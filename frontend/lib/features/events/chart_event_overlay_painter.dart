import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'events_list_screen.dart' show impactColor;

/// Describes a group of events at one X position on the chart.
class EventMarker {
  final double x;
  final DateTime timestamp;
  final List<Event> events;

  const EventMarker({
    required this.x,
    required this.timestamp,
    required this.events,
  });
}

/// Paints vertical event lines on top of the candle chart.
///
/// Each distinct timestamp gets a vertical line. If multiple events
/// share a timestamp they are merged and the highest-impact colour wins.
class EventOverlayPainter extends CustomPainter {
  final List<EventMarker> markers;

  EventOverlayPainter({required this.markers});

  @override
  void paint(Canvas canvas, Size size) {
    for (final marker in markers) {
      if (marker.x < 0 || marker.x > size.width) continue;

      final color = _highestImpactColor(marker.events);
      final paint = Paint()
        ..color = color.withAlpha(60)
        ..strokeWidth = 1.2;

      canvas.drawLine(
        Offset(marker.x, 0),
        Offset(marker.x, size.height),
        paint,
      );

      // If stacked (multiple events), draw a count badge at top
      if (marker.events.length > 1) {
        _drawCountBadge(
            canvas, marker.x, marker.events.length, color, size);
      } else {
        // Single event dot at top
        canvas.drawCircle(
          Offset(marker.x, 6),
          3,
          Paint()..color = color.withAlpha(180),
        );
      }
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
    return !identical(markers, old.markers);
  }
}
