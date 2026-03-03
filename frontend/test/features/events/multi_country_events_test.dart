import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/application/get_events.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';
import 'package:pano_chart_frontend/features/events/events_view_model.dart';
import 'package:pano_chart_frontend/features/events/macro_events_screen.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

// ---- Fakes ----

class _FakeGetEvents implements GetEvents {
  /// Map of country → events returned for that country.
  Map<String, List<Event>> countryEvents = {};
  int callCount = 0;
  List<String> calledCountries = [];
  bool shouldThrow = false;

  @override
  Future<List<Event>> execute(GetEventsInput input) async {
    callCount++;
    if (input.country != null) calledCountries.add(input.country!);
    if (shouldThrow) throw Exception('network error');
    return countryEvents[input.country] ?? [];
  }
}

// ---- Test helpers ----

Event _event(String id, String country, EventImpact impact, DateTime ts) =>
    Event(id: id, country: country, title: id, impact: impact, timestamp: ts);

// ---- Tests ----

void main() {
  group('EventsState — new fields', () {
    test('initial state has default countries and influence filter', () {
      final state = EventsState.initial();
      expect(state.selectedCountries, {'United States'});
      expect(state.macroInfluenceFilter,
          {EventImpact.high, EventImpact.medium, EventImpact.low});
    });

    test('copyWith preserves new fields when not overridden', () {
      final state = EventsState.initial(
        selectedCountries: {'China'},
        macroInfluenceFilter: {EventImpact.high},
      );
      final next = state.copyWith(isLoading: true);
      expect(next.selectedCountries, {'China'});
      expect(next.macroInfluenceFilter, {EventImpact.high});
    });

    test('macroFilteredEvents uses macroInfluenceFilter', () {
      final state = EventsState.initial(
        macroInfluenceFilter: {EventImpact.high},
      ).copyWith(events: [
        _event('h', 'US', EventImpact.high, DateTime.utc(2025, 1, 1)),
        _event('m', 'US', EventImpact.medium, DateTime.utc(2025, 1, 1)),
        _event('l', 'US', EventImpact.low, DateTime.utc(2025, 1, 1)),
      ]);
      expect(state.macroFilteredEvents.length, 1);
      expect(state.macroFilteredEvents.first.id, 'h');
    });

    test('macroFilteredEvents independent of chart filterLevel', () {
      final state = EventsState.initial(
        filterLevel: EventFilterLevel.highOnly,
        macroInfluenceFilter: {EventImpact.high, EventImpact.medium, EventImpact.low},
      ).copyWith(events: [
        _event('h', 'US', EventImpact.high, DateTime.utc(2025, 1, 1)),
        _event('m', 'US', EventImpact.medium, DateTime.utc(2025, 1, 1)),
        _event('l', 'US', EventImpact.low, DateTime.utc(2025, 1, 1)),
      ]);
      // Chart overlay shows only high
      expect(state.filteredEvents.length, 1);
      // Macro screen shows all three
      expect(state.macroFilteredEvents.length, 3);
    });
  });

  group('EventsViewModel — multi-country', () {
    test('loadMultiCountry fetches each country and merges', () async {
      final fake = _FakeGetEvents()
        ..countryEvents = {
          'United States': [
            _event('us1', 'United States', EventImpact.high,
                DateTime.utc(2025, 3, 1, 14)),
          ],
          'China': [
            _event('cn1', 'China', EventImpact.medium,
                DateTime.utc(2025, 3, 1, 8)),
          ],
        };
      final vm = EventsViewModel(fake);
      await vm.loadMultiCountry(
          '2025-03-01', '2025-03-02', {'United States', 'China'});

      expect(vm.state.events.length, 2);
      // Merged and sorted by timestamp — China event first (08:00 < 14:00)
      expect(vm.state.events[0].id, 'cn1');
      expect(vm.state.events[1].id, 'us1');
      expect(fake.callCount, 2);
    });

    test('loadMultiCountry deduplicates by event ID', () async {
      final dup = _event(
          'dup', 'United States', EventImpact.high, DateTime.utc(2025, 3, 1));
      final fake = _FakeGetEvents()
        ..countryEvents = {
          'United States': [dup],
          'Euro Area': [dup], // same ID
        };
      final vm = EventsViewModel(fake);
      await vm.loadMultiCountry(
          '2025-03-01', '2025-03-02', {'United States', 'Euro Area'});

      expect(vm.state.events.length, 1);
    });

    test('loadMultiCountry with empty countries sets empty events', () async {
      final fake = _FakeGetEvents();
      final vm = EventsViewModel(fake);
      await vm.loadMultiCountry('2025-03-01', '2025-03-02', {});

      expect(vm.state.events, isEmpty);
      expect(fake.callCount, 0);
    });

    test('loadMultiCountry sets error on failure', () async {
      final fake = _FakeGetEvents()..shouldThrow = true;
      final vm = EventsViewModel(fake);
      await vm.loadMultiCountry(
          '2025-03-01', '2025-03-02', {'United States'});

      expect(vm.state.error, isNotNull);
      expect(vm.state.isLoading, isFalse);
    });

    test('toggleCountry adds and removes', () {
      final vm = EventsViewModel(_FakeGetEvents());
      expect(vm.state.selectedCountries, {'United States'});

      vm.toggleCountry('China');
      expect(vm.state.selectedCountries, {'United States', 'China'});

      vm.toggleCountry('United States');
      expect(vm.state.selectedCountries, {'China'});
    });

    test('toggleMacroInfluence adds and removes', () {
      final vm = EventsViewModel(_FakeGetEvents());
      expect(vm.state.macroInfluenceFilter,
          {EventImpact.high, EventImpact.medium, EventImpact.low});

      vm.toggleMacroInfluence(EventImpact.low);
      expect(vm.state.macroInfluenceFilter,
          {EventImpact.high, EventImpact.medium});

      vm.toggleMacroInfluence(EventImpact.low);
      expect(vm.state.macroInfluenceFilter,
          {EventImpact.high, EventImpact.medium, EventImpact.low});
    });
  });

  group('EventsViewModel — persistence', () {
    test('attachPrefs restores countries and influence filter', () async {
      SharedPreferences.setMockInitialValues({
        'settings.selectedCountries': ['Euro Area', 'China'],
        'settings.macroInfluenceFilter': ['high'],
      });
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      final vm = EventsViewModel(_FakeGetEvents());
      vm.attachPrefs(prefs);

      expect(vm.state.selectedCountries, {'Euro Area', 'China'});
      expect(vm.state.macroInfluenceFilter, {EventImpact.high});
    });

    test('toggleCountry persists to prefs', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      final vm = EventsViewModel(_FakeGetEvents());
      vm.attachPrefs(prefs);

      vm.toggleCountry('China');
      expect(prefs.selectedCountries, contains('China'));
    });

    test('toggleMacroInfluence persists to prefs', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      final vm = EventsViewModel(_FakeGetEvents());
      vm.attachPrefs(prefs);

      vm.toggleMacroInfluence(EventImpact.low);
      expect(prefs.macroInfluenceFilter, isNot(contains('low')));
    });
  });

  group('kAvailableCountries', () {
    test('contains expected countries', () {
      expect(kAvailableCountries, ['United States', 'Euro Area', 'China']);
    });
  });

  group('macroInfluenceLabel', () {
    test('returns correct labels', () {
      expect(macroInfluenceLabel(EventImpact.high), 'High');
      expect(macroInfluenceLabel(EventImpact.medium), 'Moderate');
      expect(macroInfluenceLabel(EventImpact.low), 'Standard');
    });
  });

  group('MacroEventsScreen', () {
    testWidgets('shows filter icon in app bar', (tester) async {
      final vm = EventsViewModel(_FakeGetEvents());
      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.filter_list), findsOneWidget);
    });

    testWidgets('tapping filter icon expands country checkboxes',
        (tester) async {
      final fake = _FakeGetEvents()
        ..countryEvents = {
          'United States': [
            _event('us1', 'United States', EventImpact.high,
                DateTime.utc(2025, 3, 1)),
          ],
        };
      final vm = EventsViewModel(fake);
      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      // Filter panel not visible yet
      expect(find.text('Countries'), findsNothing);

      // Tap filter icon
      await tester.tap(find.byIcon(Icons.filter_list));
      await tester.pumpAndSettle();

      // Now filter panel visible
      expect(find.text('Countries'), findsOneWidget);
      expect(find.text('Influence'), findsOneWidget);
      expect(find.text('United States'), findsWidgets);
      expect(find.text('Euro Area'), findsOneWidget);
      expect(find.text('China'), findsOneWidget);
    });

    testWidgets('influence filter chips show all three levels',
        (tester) async {
      final vm = EventsViewModel(_FakeGetEvents());
      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      // Open filter
      await tester.tap(find.byIcon(Icons.filter_list));
      await tester.pumpAndSettle();

      expect(find.text('High'), findsOneWidget);
      expect(find.text('Moderate'), findsOneWidget);
      expect(find.text('Standard'), findsOneWidget);
    });

    testWidgets('title reflects selected countries count', (tester) async {
      final vm = EventsViewModel(_FakeGetEvents());
      // Default: only US
      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Macro Events — United States'), findsOneWidget);
    });

    testWidgets('events are filtered by macroInfluenceFilter',
        (tester) async {
      final fake = _FakeGetEvents()
        ..countryEvents = {
          'United States': [
            _event('h', 'United States', EventImpact.high,
                DateTime.utc(2099, 3, 1, 14)),
            _event('l', 'United States', EventImpact.low,
                DateTime.utc(2099, 3, 1, 15)),
          ],
        };
      final vm = EventsViewModel(fake);
      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      // Both events visible (default: all influence levels)
      expect(find.text('h'), findsOneWidget);
      expect(find.text('l'), findsOneWidget);

      // Open filter, toggle off Standard (low)
      await tester.tap(find.byIcon(Icons.filter_list));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Standard'));
      await tester.pumpAndSettle();

      // Low event should be gone
      expect(find.text('h'), findsOneWidget);
      expect(find.text('l'), findsNothing);
    });

    testWidgets('shows empty state when no countries selected',
        (tester) async {
      final vm = EventsViewModel(_FakeGetEvents());
      // Deselect the default country
      vm.toggleCountry('United States');

      await tester.pumpWidget(
        MaterialApp(home: MacroEventsScreen(viewModel: vm)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Select at least one country'), findsOneWidget);
    });
  });

  group('PreferencesService — new fields', () {
    test('selectedCountries defaults to United States', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      expect(prefs.selectedCountries, {'United States'});
    });

    test('selectedCountries round-trips', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      prefs.selectedCountries = {'Euro Area', 'China'};
      expect(prefs.selectedCountries, {'Euro Area', 'China'});
    });

    test('macroInfluenceFilter defaults to all', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      expect(prefs.macroInfluenceFilter, {'high', 'medium', 'low'});
    });

    test('macroInfluenceFilter round-trips', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs =
          PreferencesService(await SharedPreferences.getInstance());
      prefs.macroInfluenceFilter = {'high'};
      expect(prefs.macroInfluenceFilter, {'high'});
    });
  });
}
