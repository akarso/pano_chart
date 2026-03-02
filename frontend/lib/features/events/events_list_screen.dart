import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'event_filter.dart';

/// Full-screen list of macroeconomic events.
///
/// Supports scroll-to-event via [scrollToEventId] and a brief highlight effect.
class EventsListScreen extends StatefulWidget {
  final List<Event> events;
  final EventFilterLevel filterLevel;
  final String? scrollToEventId;

  const EventsListScreen({
    Key? key,
    required this.events,
    required this.filterLevel,
    this.scrollToEventId,
  }) : super(key: key);

  @override
  State<EventsListScreen> createState() => _EventsListScreenState();
}

class _EventsListScreenState extends State<EventsListScreen> {
  final Map<String, GlobalKey> _itemKeys = {};
  String? _highlightedId;

  @override
  void initState() {
    super.initState();
    for (final e in widget.events) {
      _itemKeys[e.id] = GlobalKey();
    }
    if (widget.scrollToEventId != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _scrollToAndHighlight(widget.scrollToEventId!);
      });
    }
  }

  void _scrollToAndHighlight(String eventId) {
    final key = _itemKeys[eventId];
    if (key?.currentContext != null) {
      Scrollable.ensureVisible(
        key!.currentContext!,
        duration: const Duration(milliseconds: 400),
        curve: Curves.easeInOut,
      );
    }
    setState(() => _highlightedId = eventId);
    Future.delayed(const Duration(seconds: 2), () {
      if (mounted) setState(() => _highlightedId = null);
    });
  }

  @override
  Widget build(BuildContext context) {
    final filtered = widget.events
        .where((e) => widget.filterLevel.allows(e.impact))
        .toList()
      ..sort((a, b) => a.timestamp.compareTo(b.timestamp));

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        elevation: 0,
        title: const Text('Economic Events',
            style: TextStyle(color: Colors.white)),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back, color: Colors.white),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: filtered.isEmpty
          ? const Center(
              child: Text('No events',
                  style: TextStyle(color: Colors.white54, fontSize: 14)))
          : ListView.separated(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              itemCount: filtered.length,
              separatorBuilder: (_, __) =>
                  Divider(color: Colors.white.withAlpha(25), height: 1),
              itemBuilder: (context, index) {
                final event = filtered[index];
                final isHighlighted = _highlightedId == event.id;
                return AnimatedContainer(
                  key: _itemKeys[event.id],
                  duration: const Duration(milliseconds: 500),
                  color: isHighlighted
                      ? Colors.white.withAlpha(20)
                      : Colors.transparent,
                  padding:
                      const EdgeInsets.symmetric(vertical: 12, horizontal: 4),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _ImpactDot(impact: event.impact),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              event.title,
                              style: const TextStyle(
                                color: Colors.white,
                                fontSize: 14,
                                fontWeight: FontWeight.w500,
                              ),
                            ),
                            const SizedBox(height: 4),
                            Row(
                              children: [
                                Text(
                                  event.country,
                                  style: const TextStyle(
                                    color: Colors.white54,
                                    fontSize: 12,
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Text(
                                  _formatImpact(event.impact),
                                  style: TextStyle(
                                    color: impactColor(event.impact),
                                    fontSize: 12,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                              ],
                            ),
                          ],
                        ),
                      ),
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.end,
                        children: [
                          Text(
                            _formatUtcTime(event.timestamp),
                            style: const TextStyle(
                              color: Colors.white54,
                              fontSize: 11,
                            ),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            _formatLocalTime(event.timestamp),
                            style: const TextStyle(
                              color: Colors.white38,
                              fontSize: 10,
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                );
              },
            ),
    );
  }

  String _formatUtcTime(DateTime utc) {
    final h = utc.hour.toString().padLeft(2, '0');
    final m = utc.minute.toString().padLeft(2, '0');
    final mon = utc.month.toString().padLeft(2, '0');
    final d = utc.day.toString().padLeft(2, '0');
    return '$mon/$d $h:$m UTC';
  }

  String _formatLocalTime(DateTime utc) {
    final local = utc.toLocal();
    final h = local.hour.toString().padLeft(2, '0');
    final m = local.minute.toString().padLeft(2, '0');
    return '$h:$m local';
  }

  String _formatImpact(EventImpact impact) {
    switch (impact) {
      case EventImpact.high:
        return 'HIGH';
      case EventImpact.medium:
        return 'MED';
      case EventImpact.low:
        return 'LOW';
    }
  }
}

/// Returns the color for a given impact level.
Color impactColor(EventImpact impact) {
  switch (impact) {
    case EventImpact.high:
      return Colors.red;
    case EventImpact.medium:
      return Colors.orange;
    case EventImpact.low:
      return Colors.grey;
  }
}

class _ImpactDot extends StatelessWidget {
  final EventImpact impact;
  const _ImpactDot({required this.impact});

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(top: 4),
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        color: impactColor(impact),
        shape: BoxShape.circle,
      ),
    );
  }
}
