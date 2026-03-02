import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'events_list_screen.dart' show impactColor;
import 'events_view_model.dart';

/// Standalone screen that loads macroeconomic events for the US
/// (yesterday → tomorrow) and displays them in a scrollable list.
///
/// Reachable from the overview menu as "Macroeconomical Events".
class MacroEventsScreen extends StatefulWidget {
  final EventsViewModel viewModel;

  const MacroEventsScreen({Key? key, required this.viewModel})
      : super(key: key);

  @override
  State<MacroEventsScreen> createState() => _MacroEventsScreenState();
}

class _MacroEventsScreenState extends State<MacroEventsScreen> {
  @override
  void initState() {
    super.initState();
    widget.viewModel.onChanged = () {
      if (mounted) setState(() {});
    };
    _loadEvents();
  }

  @override
  void dispose() {
    widget.viewModel.onChanged = null;
    super.dispose();
  }

  void _loadEvents() {
    final now = DateTime.now().toUtc();
    final from = now.subtract(const Duration(days: 1));
    final to = now.add(const Duration(days: 1));
    final fmt = (DateTime d) =>
        '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
    widget.viewModel.load(fmt(from), fmt(to), country: 'United States');
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;
    final filtered = state.filteredEvents..sort((a, b) => a.timestamp.compareTo(b.timestamp));

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        elevation: 0,
        title: const Text('Macro Events — US',
            style: TextStyle(color: Colors.white)),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back, color: Colors.white),
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: _buildBody(state, filtered),
    );
  }

  Widget _buildBody(EventsState state, List<Event> filtered) {
    if (state.isLoading && state.events.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.events.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(state.error!, style: const TextStyle(color: Colors.white54)),
            const SizedBox(height: 12),
            TextButton(onPressed: _loadEvents, child: const Text('Retry')),
          ],
        ),
      );
    }
    if (filtered.isEmpty) {
      return const Center(
        child: Text('No events for this period',
            style: TextStyle(color: Colors.white54, fontSize: 14)),
      );
    }

    return RefreshIndicator(
      onRefresh: () async => _loadEvents(),
      child: ListView.separated(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        itemCount: filtered.length,
        separatorBuilder: (_, __) =>
            Divider(color: Colors.white.withAlpha(25), height: 1),
        itemBuilder: (_, index) => _EventTile(event: filtered[index]),
      ),
    );
  }
}

class _EventTile extends StatelessWidget {
  final Event event;
  const _EventTile({required this.event});

  bool get _isPast => event.timestamp.isBefore(DateTime.now().toUtc());

  @override
  Widget build(BuildContext context) {
    return Opacity(
      opacity: _isPast ? 0.45 : 1.0,
      child: Padding(
      padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Impact dot
          Container(
            margin: const EdgeInsets.only(top: 4),
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              color: impactColor(event.impact),
              shape: BoxShape.circle,
            ),
          ),
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
                    Text(event.country,
                        style: const TextStyle(
                            color: Colors.white54, fontSize: 12)),
                    const SizedBox(width: 8),
                    Text(
                      _impactLabel(event.impact),
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
              Text(_fmtUtc(event.timestamp),
                  style:
                      const TextStyle(color: Colors.white54, fontSize: 11)),
              const SizedBox(height: 2),
              Text(_fmtLocal(event.timestamp),
                  style:
                      const TextStyle(color: Colors.white38, fontSize: 10)),
            ],
          ),
        ],
      ),
    ),
    );
  }

  static String _impactLabel(EventImpact i) {
    switch (i) {
      case EventImpact.high:
        return 'HIGH';
      case EventImpact.medium:
        return 'MED';
      case EventImpact.low:
        return 'LOW';
    }
  }

  static String _fmtUtc(DateTime utc) {
    final y = utc.year.toString();
    final mon = utc.month.toString().padLeft(2, '0');
    final d = utc.day.toString().padLeft(2, '0');
    final h = utc.hour.toString().padLeft(2, '0');
    final m = utc.minute.toString().padLeft(2, '0');
    return '$y-$mon-$d $h:$m UTC';
  }

  static String _fmtLocal(DateTime utc) {
    final local = utc.toLocal();
    final h = local.hour.toString().padLeft(2, '0');
    final m = local.minute.toString().padLeft(2, '0');
    return '$h:$m local';
  }
}
