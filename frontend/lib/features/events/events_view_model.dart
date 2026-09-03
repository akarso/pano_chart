import 'dart:ui' show VoidCallback;

import '../../domain/event.dart';
import 'application/get_events.dart';
import 'event_filter.dart';
import '../../infrastructure/preferences_service.dart';

/// All countries available for event querying.
const List<String> kAvailableCountries = [
  'United States',
  'Euro Area',
  'China',
];

/// Display label for an [EventImpact] in the macro events filter.
String macroInfluenceLabel(EventImpact impact) {
  switch (impact) {
    case EventImpact.high:
      return 'High';
    case EventImpact.medium:
      return 'Moderate';
    case EventImpact.low:
      return 'Standard';
  }
}

/// Immutable state for the events feature.
class EventsState {
  final bool isLoading;
  final List<Event> events;
  final String? error;
  final bool showEvents;
  final EventFilterLevel filterLevel;

  /// Countries currently selected for the macro events screen.
  final Set<String> selectedCountries;

  /// Impact levels currently enabled for the macro events screen.
  /// Independent of [filterLevel] which controls the chart overlay.
  final Set<EventImpact> macroInfluenceFilter;

  const EventsState({
    required this.isLoading,
    required this.events,
    required this.error,
    required this.showEvents,
    required this.filterLevel,
    required this.selectedCountries,
    required this.macroInfluenceFilter,
  });

  factory EventsState.initial({
    bool showEvents = true,
    EventFilterLevel filterLevel = EventFilterLevel.highAndMedium,
    Set<String>? selectedCountries,
    Set<EventImpact>? macroInfluenceFilter,
  }) =>
      EventsState(
        isLoading: false,
        events: const [],
        error: null,
        showEvents: showEvents,
        filterLevel: filterLevel,
        selectedCountries: selectedCountries ?? const {'United States'},
        macroInfluenceFilter:
            macroInfluenceFilter ?? const {EventImpact.high, EventImpact.medium, EventImpact.low},
      );

  EventsState copyWith({
    bool? isLoading,
    List<Event>? events,
    String? error,
    bool? showEvents,
    EventFilterLevel? filterLevel,
    Set<String>? selectedCountries,
    Set<EventImpact>? macroInfluenceFilter,
  }) {
    return EventsState(
      isLoading: isLoading ?? this.isLoading,
      events: events ?? this.events,
      error: error,
      showEvents: showEvents ?? this.showEvents,
      filterLevel: filterLevel ?? this.filterLevel,
      selectedCountries: selectedCountries ?? this.selectedCountries,
      macroInfluenceFilter: macroInfluenceFilter ?? this.macroInfluenceFilter,
    );
  }

  /// Returns events filtered by the chart overlay [filterLevel].
  List<Event> get filteredEvents =>
      events.where((e) => filterLevel.allows(e.impact)).toList();

  /// Returns events filtered by the macro screen [macroInfluenceFilter].
  List<Event> get macroFilteredEvents =>
      events.where((e) => macroInfluenceFilter.contains(e.impact)).toList();
}

/// ViewModel for macroeconomic events. Loads events for a date range
/// and exposes filtering + toggle state.
class EventsViewModel {
  EventsState _state = EventsState.initial();
  EventsState get state => _state;

  final GetEvents _getEvents;
  VoidCallback? onChanged;
  PreferencesService? _prefs;

  EventsViewModel(this._getEvents);

  void attachPrefs(PreferencesService? prefs) {
    _prefs = prefs;
    if (prefs != null) {
      _state = _state.copyWith(
        showEvents: prefs.showEvents,
        filterLevel: EventFilterLevel.fromString(prefs.eventFilter),
        selectedCountries: prefs.selectedCountries,
        macroInfluenceFilter: prefs.macroInfluenceFilter
            .map((s) => EventImpact.fromString(s))
            .toSet(),
      );
    }
  }

  /// Loads events spanning [dateFrom] to [dateTo] (ISO date strings).
  ///
  /// An optional [country] can be provided to filter server-side.
  Future<void> load(String dateFrom, String dateTo, {String? country}) async {
    _setState(_state.copyWith(isLoading: true, error: null));
    try {
      final events = await _getEvents.execute(
        GetEventsInput(dateFrom: dateFrom, dateTo: dateTo, country: country),
      );
      _setState(_state.copyWith(isLoading: false, events: events));
    } catch (e) {
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  /// Loads events for multiple [countries] and merges results by timestamp.
  ///
  /// The API only accepts one country per request, so we iterate and merge.
  Future<void> loadMultiCountry(String dateFrom, String dateTo,
      Set<String> countries) async {
    if (countries.isEmpty) {
      _setState(_state.copyWith(isLoading: false, events: const []));
      return;
    }
    _setState(_state.copyWith(isLoading: true, error: null));
    try {
      final futures = countries.map((c) => _getEvents.execute(
            GetEventsInput(dateFrom: dateFrom, dateTo: dateTo, country: c),
          ));
      final results = await Future.wait(futures);
      final merged = <Event>[];
      final seen = <String>{};
      for (final list in results) {
        for (final event in list) {
          if (seen.add(event.id)) merged.add(event);
        }
      }
      merged.sort((a, b) => a.timestamp.compareTo(b.timestamp));
      _setState(_state.copyWith(isLoading: false, events: merged));
    } catch (e) {
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  /// Toggles a country in the selected set and persists.
  void toggleCountry(String country) {
    final next = Set<String>.from(_state.selectedCountries);
    if (next.contains(country)) {
      next.remove(country);
    } else {
      next.add(country);
    }
    _setState(_state.copyWith(selectedCountries: next));
    _prefs?.selectedCountries = next;
  }

  /// Toggles an influence level in the macro filter and persists.
  void toggleMacroInfluence(EventImpact impact) {
    final next = Set<EventImpact>.from(_state.macroInfluenceFilter);
    if (next.contains(impact)) {
      next.remove(impact);
    } else {
      next.add(impact);
    }
    _setState(_state.copyWith(macroInfluenceFilter: next));
    _prefs?.macroInfluenceFilter = next.map((i) => i.name).toSet();
  }

  /// Toggles event overlay visibility.
  void toggleShowEvents() {
    final next = !_state.showEvents;
    _setState(_state.copyWith(showEvents: next));
    _prefs?.showEvents = next;
  }

  /// Changes the impact filter level (for chart overlay).
  void setFilterLevel(EventFilterLevel level) {
    _setState(_state.copyWith(filterLevel: level));
    _prefs?.eventFilter = level.name;
  }

  void _setState(EventsState next) {
    _state = next;
    onChanged?.call();
  }
}
