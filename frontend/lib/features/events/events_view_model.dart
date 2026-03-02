import 'dart:ui' show VoidCallback;

import '../../domain/event.dart';
import 'application/get_events.dart';
import 'event_filter.dart';
import '../../infrastructure/preferences_service.dart';

/// Immutable state for the events feature.
class EventsState {
  final bool isLoading;
  final List<Event> events;
  final String? error;
  final bool showEvents;
  final EventFilterLevel filterLevel;

  const EventsState({
    required this.isLoading,
    required this.events,
    required this.error,
    required this.showEvents,
    required this.filterLevel,
  });

  factory EventsState.initial({
    bool showEvents = true,
    EventFilterLevel filterLevel = EventFilterLevel.highAndMedium,
  }) =>
      EventsState(
        isLoading: false,
        events: const [],
        error: null,
        showEvents: showEvents,
        filterLevel: filterLevel,
      );

  EventsState copyWith({
    bool? isLoading,
    List<Event>? events,
    String? error,
    bool? showEvents,
    EventFilterLevel? filterLevel,
  }) {
    return EventsState(
      isLoading: isLoading ?? this.isLoading,
      events: events ?? this.events,
      error: error,
      showEvents: showEvents ?? this.showEvents,
      filterLevel: filterLevel ?? this.filterLevel,
    );
  }

  /// Returns events filtered by the current [filterLevel].
  List<Event> get filteredEvents =>
      events.where((e) => filterLevel.allows(e.impact)).toList();
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
      );
    }
  }

  /// Loads events spanning [dateFrom] to [dateTo] (ISO date strings).
  Future<void> load(String dateFrom, String dateTo) async {
    _setState(_state.copyWith(isLoading: true, error: null));
    try {
      final events = await _getEvents.execute(
        GetEventsInput(dateFrom: dateFrom, dateTo: dateTo),
      );
      _setState(_state.copyWith(isLoading: false, events: events));
    } catch (e) {
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  /// Toggles event overlay visibility.
  void toggleShowEvents() {
    final next = !_state.showEvents;
    _setState(_state.copyWith(showEvents: next));
    _prefs?.showEvents = next;
  }

  /// Changes the impact filter level.
  void setFilterLevel(EventFilterLevel level) {
    _setState(_state.copyWith(filterLevel: level));
    _prefs?.eventFilter = level.name;
  }

  void _setState(EventsState next) {
    _state = next;
    onChanged?.call();
  }
}
