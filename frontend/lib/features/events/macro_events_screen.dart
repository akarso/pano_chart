import 'package:flutter/material.dart';

import '../../domain/event.dart';
import 'events_list_screen.dart' show impactColor;
import 'events_view_model.dart';

/// Standalone screen that loads macroeconomic events for selected countries
/// (yesterday → tomorrow) and displays them in a scrollable list.
///
/// Reachable from the overview menu as "Macroeconomical Events".
class MacroEventsScreen extends StatefulWidget {
  final EventsViewModel viewModel;

  /// When non-null, the screen centers on this event and highlights it.
  final String? scrollToEventId;

  /// Whether the user has pro access (free: 3 upcoming + 2 past).
  final bool isProUser;

  const MacroEventsScreen({
    Key? key,
    required this.viewModel,
    this.scrollToEventId,
    this.isProUser = false,
  }) : super(key: key);

  @override
  State<MacroEventsScreen> createState() => MacroEventsScreenState();
}

class MacroEventsScreenState extends State<MacroEventsScreen> {
  bool _filterExpanded = false;
  bool _hasScrolled = false;
  String? _highlightedEventId;
  final Map<String, GlobalKey> _eventKeys = {};

  @override
  void initState() {
    super.initState();
    _highlightedEventId = widget.scrollToEventId;
    widget.viewModel.onChanged = () {
      if (mounted) {
        setState(() {});
        if (!_hasScrolled) {
          WidgetsBinding.instance.addPostFrameCallback((_) {
            _scrollToInitialPosition();
          });
        }
      }
    };
    _loadEvents();
  }

  @override
  void dispose() {
    widget.viewModel.onChanged = null;
    super.dispose();
  }

  /// Scrolls to the initial position after events are loaded.
  ///
  /// If [scrollToEventId] was provided, centers on that event.
  /// Otherwise, scrolls so the closest future event is one row below
  /// the top edge.
  void _scrollToInitialPosition() {
    if (_hasScrolled || !mounted) return;
    final state = widget.viewModel.state;
    // Wait until loading finishes so we scroll on fresh data,
    // not stale events left over from a previous screen.
    if (state.isLoading) return;
    final filtered = List<Event>.of(state.macroFilteredEvents)
      ..sort((a, b) => a.timestamp.compareTo(b.timestamp));
    if (filtered.isEmpty) return;

    _hasScrolled = true;

    if (widget.scrollToEventId != null) {
      _scrollToEvent(widget.scrollToEventId!, filtered, center: true);
    } else {
      _scrollToClosestFuture(filtered);
    }
  }

  void _scrollToEvent(
      String eventId, List<Event> sorted, {bool center = false}) {
    final key = _eventKeys[eventId];
    if (key?.currentContext != null) {
      Scrollable.ensureVisible(
        key!.currentContext!,
        alignment: center ? 0.5 : 0.0,
        duration: const Duration(milliseconds: 300),
      );
    }
  }

  void _scrollToClosestFuture(List<Event> sorted) {
    final now = DateTime.now().toUtc();
    final futureIdx = sorted.indexWhere((e) => e.timestamp.isAfter(now));

    if (futureIdx < 0) {
      // All events are past — scroll to end
      final key = _eventKeys[sorted.last.id];
      if (key?.currentContext != null) {
        Scrollable.ensureVisible(
          key!.currentContext!,
          alignment: 1.0,
          duration: const Duration(milliseconds: 300),
        );
      }
    } else if (futureIdx > 0) {
      // Show the last past event at the top edge → closest future
      // event appears one row below.
      final key = _eventKeys[sorted[futureIdx - 1].id];
      if (key?.currentContext != null) {
        Scrollable.ensureVisible(
          key!.currentContext!,
          alignment: 0.0,
          duration: const Duration(milliseconds: 300),
        );
      }
    }
    // futureIdx == 0 → already at top, nothing to scroll.
  }

  void _loadEvents() {
    final now = DateTime.now().toUtc();
    final from = now.subtract(const Duration(days: 1));
    final to = now.add(const Duration(days: 2));
    final fmt = (DateTime d) =>
        '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
    widget.viewModel
        .loadMultiCountry(fmt(from), fmt(to), widget.viewModel.state.selectedCountries);
  }

  String _titleLabel() {
    final countries = widget.viewModel.state.selectedCountries;
    if (countries.isEmpty) return 'Macro Events';
    if (countries.length == 1) return 'Macro Events — ${countries.first}';
    return 'Macro Events — ${countries.length} regions';
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;
    var filtered = state.macroFilteredEvents
      ..sort((a, b) => a.timestamp.compareTo(b.timestamp));

    // Free tier: show only 3 upcoming + 2 past events.
    if (!widget.isProUser) {
      final now = DateTime.now().toUtc();
      final past = filtered.where((e) => e.timestamp.isBefore(now)).toList();
      final upcoming = filtered.where((e) => !e.timestamp.isBefore(now)).toList();
      filtered = [
        ...past.length > 2 ? past.sublist(past.length - 2) : past,
        ...upcoming.length > 3 ? upcoming.sublist(0, 3) : upcoming,
      ];
    }

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        elevation: 0,
        title: Text(_titleLabel(),
            style: const TextStyle(color: Colors.white)),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back, color: Colors.white),
          onPressed: () => Navigator.of(context).pop(),
        ),
        actions: [
          IconButton(
            icon: Icon(
              _filterExpanded ? Icons.filter_list_off : Icons.filter_list,
              color: Colors.white,
            ),
            onPressed: () => setState(() => _filterExpanded = !_filterExpanded),
          ),
        ],
      ),
      body: Column(
        children: [
          _buildFilterPanel(state),
          Expanded(child: _buildBody(state, filtered)),
        ],
      ),
    );
  }

  // ---- filter panel ----

  Widget _buildFilterPanel(EventsState state) {
    return AnimatedSize(
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeInOut,
      alignment: Alignment.topCenter,
      clipBehavior: Clip.hardEdge,
      child: _filterExpanded
          ? Container(
              width: double.infinity,
              decoration: BoxDecoration(
                color: const Color(0xFF1A1A1A),
                border: Border(
                  bottom: BorderSide(
                      color: Colors.white.withAlpha(25), width: 1),
                ),
              ),
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Text('Countries',
                      style: TextStyle(
                          color: Colors.white70,
                          fontSize: 12,
                          fontWeight: FontWeight.w600)),
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 4,
                    runSpacing: 4,
                    children: kAvailableCountries.map((country) {
                      final selected =
                          state.selectedCountries.contains(country);
                      return _FilterChip(
                        label: country,
                        selected: selected,
                        onTap: () {
                          widget.viewModel.toggleCountry(country);
                          _loadEvents();
                        },
                      );
                    }).toList(),
                  ),
                  const SizedBox(height: 14),
                  const Text('Influence',
                      style: TextStyle(
                          color: Colors.white70,
                          fontSize: 12,
                          fontWeight: FontWeight.w600)),
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 4,
                    runSpacing: 4,
                    children: EventImpact.values.map((impact) {
                      final selected =
                          state.macroInfluenceFilter.contains(impact);
                      return _FilterChip(
                        label: macroInfluenceLabel(impact),
                        selected: selected,
                        dotColor: impactColor(impact),
                        onTap: () {
                          widget.viewModel.toggleMacroInfluence(impact);
                        },
                      );
                    }).toList(),
                  ),
                ],
              ),
            )
          : const SizedBox.shrink(),
    );
  }

  // ---- main body ----

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
    if (state.selectedCountries.isEmpty) {
      return const Center(
        child: Text('Select at least one country',
            style: TextStyle(color: Colors.white54, fontSize: 14)),
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
      child: SingleChildScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: EdgeInsets.only(
          left: 16, right: 16, top: 8,
          bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
        ),
        child: Column(
          children: [
            for (var i = 0; i < filtered.length; i++) ...[
              Builder(builder: (_) {
                final event = filtered[i];
                final key =
                    _eventKeys.putIfAbsent(event.id, () => GlobalKey());
                return _EventTile(
                  key: key,
                  event: event,
                  isHighlighted: event.id == _highlightedEventId,
                );
              }),
              if (i < filtered.length - 1)
                Divider(color: Colors.white.withAlpha(25), height: 1),
            ],
          ],
        ),
      ),
    );
  }
}

// ---- filter chip ----

class _FilterChip extends StatelessWidget {
  final String label;
  final bool selected;
  final Color? dotColor;
  final VoidCallback onTap;

  const _FilterChip({
    required this.label,
    required this.selected,
    this.dotColor,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: selected
              ? Colors.white.withAlpha(20)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(
            color: selected ? Colors.white54 : Colors.white24,
            width: 1,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (dotColor != null) ...[
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  color: dotColor,
                  shape: BoxShape.circle,
                ),
              ),
              const SizedBox(width: 6),
            ],
            Text(
              label,
              style: TextStyle(
                color: selected ? Colors.white : Colors.white54,
                fontSize: 12,
                fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
              ),
            ),
            if (selected) ...[
              const SizedBox(width: 4),
              const Icon(Icons.check, size: 14, color: Colors.white70),
            ],
          ],
        ),
      ),
    );
  }
}

// ---- event tile ----

class _EventTile extends StatelessWidget {
  final Event event;
  final bool isHighlighted;
  const _EventTile({
    super.key,
    required this.event,
    this.isHighlighted = false,
  });

  bool get _isPast => event.timestamp.isBefore(DateTime.now().toUtc());

  @override
  Widget build(BuildContext context) {
    Widget tile = Opacity(
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
    if (isHighlighted) {
      tile = DecoratedBox(
        decoration: BoxDecoration(
          border: Border.all(color: Colors.green, width: 1.5),
          borderRadius: BorderRadius.circular(6),
        ),
        child: tile,
      );
    }
    return tile;
  }

  static String _impactLabel(EventImpact i) {
    switch (i) {
      case EventImpact.high:
        return 'HIGH';
      case EventImpact.medium:
        return 'MOD';
      case EventImpact.low:
        return 'STD';
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
