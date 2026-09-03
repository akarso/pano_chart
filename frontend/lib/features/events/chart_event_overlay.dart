import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'chart_event_overlay_painter.dart';
import 'events_list_screen.dart' show impactColor;

/// A tappable overlay that sits on top of the chart.
///
/// When the user taps near an event marker, a detail popup appears.
/// Tapping the popup body navigates to the Events List screen.
class ChartEventOverlay extends StatefulWidget {
  final List<EventMarker> markers;

  /// Called when the user taps an event overlay and wants to navigate
  /// to the events list (receives the event ID to scroll to).
  final ValueChanged<String>? onNavigateToEvent;

  /// Height of the price area.  Past-event lines stop here; future-event
  /// dashed lines extend beyond to reach the x-axis.
  final double? priceAreaHeight;

  const ChartEventOverlay({
    Key? key,
    required this.markers,
    this.onNavigateToEvent,
    this.priceAreaHeight,
  }) : super(key: key);

  @override
  State<ChartEventOverlay> createState() => _ChartEventOverlayState();
}

class _ChartEventOverlayState extends State<ChartEventOverlay> {
  EventMarker? _selected;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        // Paint the vertical lines — pointer-transparent.
        Positioned.fill(
          child: IgnorePointer(
            child: CustomPaint(
              painter: EventOverlayPainter(
                markers: widget.markers,
                priceAreaHeight: widget.priceAreaHeight,
              ),
            ),
          ),
        ),
        // Individual tap targets at each marker position.
        for (final m in widget.markers)
          Positioned(
            left: m.x - 20,
            top: 0,
            width: 40,
            height: 20,
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => setState(() =>
                  _selected = _selected == m ? null : m),
            ),
          ),
        // Dismiss layer — tapping anywhere else closes the popup.
        if (_selected != null)
          Positioned.fill(
            child: GestureDetector(
              behavior: HitTestBehavior.translucent,
              onTap: () => setState(() => _selected = null),
            ),
          ),
        // Detail popup
        if (_selected != null) _buildPopup(context, _selected!),
      ],
    );
  }

  Widget _buildPopup(BuildContext context, EventMarker marker) {
    // Position popup near the marker, at the top of the chart
    final left = (marker.x - 100).clamp(0.0, double.infinity);
    return Positioned(
      left: left,
      top: 16,
      child: GestureDetector(
        onTap: () {
          // Navigate to events list, scroll to first event in this marker
          widget.onNavigateToEvent?.call(marker.events.first.id);
        },
        child: Material(
          color: Colors.transparent,
          child: Container(
            width: 220,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.grey[900]!.withAlpha(240),
              borderRadius: BorderRadius.circular(10),
              border: Border.all(color: Colors.white24, width: 0.5),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(
                      '${marker.events.length} event${marker.events.length > 1 ? 's' : ''}',
                      style: const TextStyle(
                        color: Colors.white70,
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    InkWell(
                      onTap: () => setState(() => _selected = null),
                      child: const Icon(Icons.close,
                          size: 14, color: Colors.white54),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                for (final e in marker.events.take(5)) ...[
                  _EventPopupRow(event: e),
                  if (e != marker.events.last) const SizedBox(height: 4),
                ],
                if (marker.events.length > 5)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      '+${marker.events.length - 5} more',
                      style: const TextStyle(
                          color: Colors.white38, fontSize: 10),
                    ),
                  ),
                const SizedBox(height: 6),
                const Text(
                  'Tap to see all \u2192',
                  style: TextStyle(color: Colors.white38, fontSize: 10),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _EventPopupRow extends StatelessWidget {
  final Event event;
  const _EventPopupRow({required this.event});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 6,
          height: 6,
          decoration: BoxDecoration(
            color: impactColor(event.impact),
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 6),
        Expanded(
          child: Text(
            event.title,
            style: const TextStyle(color: Colors.white, fontSize: 11),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: 4),
        Text(
          event.country,
          style: const TextStyle(color: Colors.white38, fontSize: 10),
        ),
      ],
    );
  }
}
