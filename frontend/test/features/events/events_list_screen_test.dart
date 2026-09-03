import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';
import 'package:pano_chart_frontend/features/events/events_list_screen.dart';

void main() {
  group('EventsListScreen', () {
    final events = [
      Event(id: 'e1', country: 'US', title: 'CPI', impact: EventImpact.high, timestamp: DateTime.utc(2025, 3, 1, 8, 30)),
      Event(id: 'e2', country: 'EU', title: 'PMI', impact: EventImpact.medium, timestamp: DateTime.utc(2025, 3, 1, 10, 0)),
      Event(id: 'e3', country: 'JP', title: 'GDP', impact: EventImpact.low, timestamp: DateTime.utc(2025, 3, 1, 0, 0)),
    ];

    testWidgets('renders events list with correct titles', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: EventsListScreen(
            events: events,
            filterLevel: EventFilterLevel.all,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('CPI'), findsOneWidget);
      expect(find.text('PMI'), findsOneWidget);
      expect(find.text('GDP'), findsOneWidget);
    });

    testWidgets('filters by highOnly', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: EventsListScreen(
            events: events,
            filterLevel: EventFilterLevel.highOnly,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('CPI'), findsOneWidget);
      expect(find.text('PMI'), findsNothing);
      expect(find.text('GDP'), findsNothing);
    });

    testWidgets('shows "No events" when all filtered out', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: EventsListScreen(
            events: const [],
            filterLevel: EventFilterLevel.all,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('No events'), findsOneWidget);
    });

    testWidgets('shows app bar title', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: EventsListScreen(
            events: events,
            filterLevel: EventFilterLevel.all,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Economic Events'), findsOneWidget);
    });
  });
}
