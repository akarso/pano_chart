import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/api/events_api.dart';
import 'package:pano_chart_frontend/features/events/api/events_response_dto.dart';
import 'package:pano_chart_frontend/features/events/application/get_events.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';
import 'package:pano_chart_frontend/features/events/events_view_model.dart';

class _FakeGetEvents implements GetEvents {
  List<Event> result = [];
  bool shouldThrow = false;

  @override
  Future<List<Event>> execute(GetEventsInput input) async {
    if (shouldThrow) throw Exception('network error');
    return result;
  }
}

void main() {
  group('EventsViewModel', () {
    test('initial state', () {
      final vm = EventsViewModel(_FakeGetEvents());
      expect(vm.state.isLoading, isFalse);
      expect(vm.state.events, isEmpty);
      expect(vm.state.showEvents, isTrue);
      expect(vm.state.filterLevel, EventFilterLevel.highAndMedium);
    });

    test('load sets events on success', () async {
      final fake = _FakeGetEvents()
        ..result = [
          Event(
            id: 'e1',
            country: 'US',
            title: 'CPI',
            impact: EventImpact.high,
            timestamp: DateTime.utc(2025, 3, 3),
          ),
        ];
      final vm = EventsViewModel(fake);
      int notifications = 0;
      vm.onChanged = () => notifications++;

      await vm.load('2025-03-01', '2025-03-07');

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.events.length, 1);
      expect(vm.state.error, isNull);
      expect(notifications, greaterThanOrEqualTo(2)); // loading + loaded
    });

    test('load sets error on failure', () async {
      final fake = _FakeGetEvents()..shouldThrow = true;
      final vm = EventsViewModel(fake);

      await vm.load('2025-03-01', '2025-03-07');

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.events, isEmpty);
      expect(vm.state.error, isNotNull);
    });

    test('toggleShowEvents flips flag', () {
      final vm = EventsViewModel(_FakeGetEvents());
      expect(vm.state.showEvents, isTrue);
      vm.toggleShowEvents();
      expect(vm.state.showEvents, isFalse);
      vm.toggleShowEvents();
      expect(vm.state.showEvents, isTrue);
    });

    test('setFilterLevel changes filter', () {
      final vm = EventsViewModel(_FakeGetEvents());
      vm.setFilterLevel(EventFilterLevel.highOnly);
      expect(vm.state.filterLevel, EventFilterLevel.highOnly);
    });

    test('filteredEvents respects filter level', () async {
      final fake = _FakeGetEvents()
        ..result = [
          Event(id: 'h', country: 'US', title: 'H', impact: EventImpact.high, timestamp: DateTime.utc(2025, 1, 1)),
          Event(id: 'm', country: 'US', title: 'M', impact: EventImpact.medium, timestamp: DateTime.utc(2025, 1, 1)),
          Event(id: 'l', country: 'US', title: 'L', impact: EventImpact.low, timestamp: DateTime.utc(2025, 1, 1)),
        ];
      final vm = EventsViewModel(fake);
      await vm.load('2025-01-01', '2025-01-02');

      // Default: highAndMedium
      expect(vm.state.filteredEvents.length, 2);

      vm.setFilterLevel(EventFilterLevel.highOnly);
      expect(vm.state.filteredEvents.length, 1);
      expect(vm.state.filteredEvents[0].id, 'h');

      vm.setFilterLevel(EventFilterLevel.all);
      expect(vm.state.filteredEvents.length, 3);
    });
  });
}
