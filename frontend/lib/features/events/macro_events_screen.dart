import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';

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
  final ScrollController _scrollController = ScrollController();

  // Rough average _EventTile height (padding + ~2 lines of text) + divider,
  // used only to pick a starting scroll offset before the fine-tuned
  // Scrollable.ensureVisible correction below — see _scrollToIndex. Doesn't
  // need to be exact: actual tile height varies with title wrapping.
  static const double _estimatedTileExtent = 72.0;

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
    _scrollController.dispose();
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
    final filtered = _visibleEvents(state);
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
    final index = sorted.indexWhere((e) => e.id == eventId);
    if (index < 0) return;
    _scrollToIndex(index, eventId, sorted, alignment: center ? 0.5 : 0.0);
  }

  void _scrollToClosestFuture(List<Event> sorted) {
    final now = DateTime.now().toUtc();
    final futureIdx = sorted.indexWhere((e) => e.timestamp.isAfter(now));

    if (futureIdx < 0) {
      // All events are past — scroll to end
      _scrollToIndex(sorted.length - 1, sorted.last.id, sorted, alignment: 1.0);
    } else if (futureIdx > 0) {
      // Show the last past event at the top edge → closest future
      // event appears one row below.
      _scrollToIndex(futureIdx - 1, sorted[futureIdx - 1].id, sorted, alignment: 0.0);
    }
    // futureIdx == 0 → already at top, nothing to scroll.
  }

  /// Maximum number of coarse-jump attempts _scrollToIndex will make before
  /// giving up on a target it still can't find a built context for.
  static const int _maxScrollAttempts = 5;

  /// Scrolls so the event at [index] (id [eventId]) is visible, aligned per
  /// [alignment] (0.0 = top edge, 0.5 = centered, 1.0 = bottom edge).
  /// [sorted] is the same list [index] was computed against — used to look
  /// up the index of whatever tile a failed attempt finds already built,
  /// so a retry can correct its estimate empirically (see
  /// _attemptScrollToIndex).
  ///
  /// The list is now lazily built (ListView.separated — see PR-077), so a
  /// target far outside the current viewport + cache extent may not have a
  /// mounted GlobalKey yet, and Scrollable.ensureVisible would silently
  /// no-op on it. Jump to an estimated offset first (bringing the target
  /// within the built/cached range), then fine-tune with ensureVisible once
  /// its context actually exists.
  void _scrollToIndex(int index, String eventId, List<Event> sorted,
      {required double alignment}) {
    final idToIndex = {for (var i = 0; i < sorted.length; i++) sorted[i].id: i};
    _attemptScrollToIndex(
      index,
      eventId,
      idToIndex,
      alignment: alignment,
      estimatedOffset: index * _estimatedTileExtent,
      attemptsLeft: _maxScrollAttempts,
    );
  }

  /// One coarse-jump attempt for _scrollToIndex, correcting its offset
  /// estimate and retrying (up to [_maxScrollAttempts] total) when the
  /// target still isn't built after landing.
  ///
  /// _estimatedTileExtent alone can be badly wrong in *either* direction:
  /// long, wrapped titles ahead of the target make real tiles taller than
  /// the guess (undershoot — the jump lands short of the target), while a
  /// run of short, single-line tiles makes them shorter (overshoot — the
  /// jump lands past it). Either way the target can end up outside the
  /// built/cached range, with Scrollable.ensureVisible silently no-op'ing
  /// on it and no other retry (_hasScrolled is already set by the time
  /// this runs) — see PR-077 CR follow-up ("Variable-height deep links
  /// fail" / "Retries Cannot Correct Overshoot": an earlier version of
  /// this retry only ever widened its estimate, which corrects undershoot
  /// but makes overshoot strictly worse every attempt).
  ///
  /// Corrects for both by using real data instead of guessing a direction:
  /// whatever tile a failed attempt lands near IS built (that's how it got
  /// there), so its distance from the top of the list — via
  /// Scrollable.ensureVisible's own offset-computation machinery — divided
  /// by its known index gives an empirical per-item extent grounded in
  /// this list's actual rendering, not the static guess. Re-estimating the
  /// target's offset from that converges within a couple of retries
  /// regardless of which direction the previous attempt missed by.
  void _attemptScrollToIndex(
    int index,
    String eventId,
    Map<String, int> idToIndex, {
    required double alignment,
    required double estimatedOffset,
    required int attemptsLeft,
  }) {
    if (!mounted || !_scrollController.hasClients) return;
    final jumpTarget = estimatedOffset
        .clamp(0.0, _scrollController.position.maxScrollExtent)
        .toDouble();
    _scrollController.jumpTo(jumpTarget);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final key = _eventKeys[eventId];
      if (key?.currentContext != null) {
        Scrollable.ensureVisible(
          key!.currentContext!,
          alignment: alignment,
          duration: const Duration(milliseconds: 300),
        );
        return;
      }
      if (attemptsLeft <= 1) return;

      // Find any currently-built tile to use as a real-geometry anchor —
      // prefer the one closest to the target index, for the most locally
      // accurate ratio.
      String? anchorId;
      int? anchorIndex;
      for (final entry in _eventKeys.entries) {
        if (entry.value.currentContext == null) continue;
        final idx = idToIndex[entry.key];
        if (idx == null) continue;
        if (anchorIndex == null ||
            (idx - index).abs() < (anchorIndex - index).abs()) {
          anchorId = entry.key;
          anchorIndex = idx;
        }
      }

      double nextEstimate;
      if (anchorId != null && anchorIndex != null && anchorIndex != 0) {
        final anchorContext = _eventKeys[anchorId]!.currentContext!;
        final anchorRenderObject = anchorContext.findRenderObject();
        final viewport = anchorRenderObject == null
            ? null
            : RenderAbstractViewport.maybeOf(anchorRenderObject);
        final anchorOffset = viewport == null
            ? jumpTarget // fallback: assume the anchor is ~where we jumped to
            : viewport.getOffsetToReveal(anchorRenderObject!, 0.0).offset;
        nextEstimate = index * (anchorOffset / anchorIndex);
      } else {
        // No usable anchor (shouldn't normally happen — a jump always
        // builds something) — fall back to the original static guess.
        nextEstimate = index * _estimatedTileExtent;
      }

      _attemptScrollToIndex(
        index,
        eventId,
        idToIndex,
        alignment: alignment,
        estimatedOffset: nextEstimate,
        attemptsLeft: attemptsLeft - 1,
      );
    });
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

  /// The events actually shown in the list: sorted, and — for a free user —
  /// capped to the last 2 past + next 3 upcoming. Used by both build() and
  /// _scrollToInitialPosition so an index computed against this list always
  /// matches the list ListView.separated actually renders — computing them
  /// separately let a free user's index be computed against the full,
  /// uncapped event set while only the capped set was ever built, so
  /// scrollToEventId's coarse jump aimed at a position that didn't
  /// correspond to anything on screen (see PR-077 CR follow-up).
  List<Event> _visibleEvents(EventsState state) {
    var filtered = List<Event>.of(state.macroFilteredEvents)
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
    return filtered;
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.viewModel.state;
    final filtered = _visibleEvents(state);

    // Prune keys for events no longer in the current filtered set — a
    // filter/country change or reload can otherwise let this map grow
    // unbounded across the screen's lifetime instead of tracking only
    // what's actually rendered.
    if (_eventKeys.isNotEmpty) {
      final liveIds = filtered.map((e) => e.id).toSet();
      _eventKeys.removeWhere((id, _) => !liveIds.contains(id));
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
      child: ListView.separated(
        controller: _scrollController,
        physics: const AlwaysScrollableScrollPhysics(),
        padding: EdgeInsets.only(
          left: 16, right: 16, top: 8,
          bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
        ),
        itemCount: filtered.length,
        separatorBuilder: (_, __) => Divider(color: Colors.white.withAlpha(25), height: 1),
        itemBuilder: (context, i) {
          final event = filtered[i];
          final key = _eventKeys.putIfAbsent(event.id, () => GlobalKey());
          return _EventTile(
            key: key,
            event: event,
            isHighlighted: event.id == _highlightedEventId,
          );
        },
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
