import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/application/get_events.dart';
import 'package:pano_chart_frontend/features/events/events_view_model.dart';
import 'package:pano_chart_frontend/features/events/macro_events_screen.dart';

// ---- Fakes ----

class _FakeGetEvents implements GetEvents {
  Map<String, List<Event>> countryEvents = {};
  @override
  Future<List<Event>> execute(GetEventsInput input) async =>
      countryEvents[input.country] ?? [];
}

Event _ev(String id, EventImpact imp, DateTime ts) =>
    Event(id: id, country: 'United States', title: 'Evt_$id', impact: imp, timestamp: ts);

/// A long, wrapping title — real _EventTile height for these is well above
/// _estimatedTileExtent's fixed 72px guess, used to reproduce the PR-077 CR
/// follow-up ("Variable-height deep links fail"): enough of these ahead of
/// a distant scroll target defeats a single fixed-estimate coarse jump.
Event _evLongTitle(String id, EventImpact imp, DateTime ts) => Event(
      id: id,
      country: 'United States',
      title: 'Evt_$id: a deliberately long macroeconomic event headline '
          'written to wrap across several lines in the event tile so its '
          'real rendered height is well beyond the fixed per-tile estimate',
      impact: imp,
      timestamp: ts,
    );

// ---- helpers ----

/// Builds a [MacroEventsScreen] in a constrained 400px-tall viewport.
Widget _app(EventsViewModel vm, {String? scrollToEventId, bool isProUser = false}) {
  return MaterialApp(
    home: SizedBox(
      height: 400,
      child: MacroEventsScreen(
        viewModel: vm,
        scrollToEventId: scrollToEventId,
        isProUser: isProUser,
      ),
    ),
  );
}

/// Creates many events: [pastCount] past + [futureCount] future.
///
/// Past events are 1h, 2h ... ago; future events are 1h, 2h ... ahead.
/// IDs: 'p1'..'pN' and 'f1'..'fN'.
List<Event> _manyEvents({int pastCount = 15, int futureCount = 15}) {
  final now = DateTime.now().toUtc();
  final events = <Event>[];
  for (var i = pastCount; i >= 1; i--) {
    events.add(_ev('p$i', EventImpact.medium, now.subtract(Duration(hours: i))));
  }
  for (var i = 1; i <= futureCount; i++) {
    events.add(_ev('f$i', EventImpact.high, now.add(Duration(hours: i))));
  }
  return events;
}

// ---- Tests ----

void main() {
  group('MacroEventsScreen — scroll & highlight (PR-033)', () {
    testWidgets('highlights selected event with green border', (tester) async {
      final events = _manyEvents(pastCount: 2, futureCount: 2);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm, scrollToEventId: 'f1'));
      await tester.pumpAndSettle();

      // The highlighted event should have a DecoratedBox with green border.
      final decoratedBoxes = tester
          .widgetList<DecoratedBox>(find.byType(DecoratedBox))
          .where((db) {
        final dec = db.decoration;
        if (dec is BoxDecoration && dec.border is Border) {
          final border = dec.border! as Border;
          return border.top.color == Colors.green;
        }
        return false;
      });
      expect(decoratedBoxes.length, 1,
          reason: 'Exactly one tile should have a green border');
    });

    testWidgets('no green border when scrollToEventId is null',
        (tester) async {
      final events = _manyEvents(pastCount: 2, futureCount: 2);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm));
      await tester.pumpAndSettle();

      final greenBorders = tester
          .widgetList<DecoratedBox>(find.byType(DecoratedBox))
          .where((db) {
        final dec = db.decoration;
        if (dec is BoxDecoration && dec.border is Border) {
          final border = dec.border! as Border;
          return border.top.color == Colors.green;
        }
        return false;
      });
      expect(greenBorders, isEmpty);
    });

    testWidgets('scrollToEventId makes a distant event visible',
        (tester) async {
      // Many events so target (f12) would be off-screen without scrolling.
      // isProUser: true — the free-tier filter (3 upcoming + 2 past) would
      // otherwise strip f12 out of the list entirely before scrolling logic
      // even runs, which is a different thing than what this test checks.
      final events = _manyEvents(pastCount: 15, futureCount: 15);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(
          _app(vm, scrollToEventId: 'f12', isProUser: true));
      await tester.pumpAndSettle();

      // 'f12' should be visible (ListView built it because we scrolled)
      expect(find.text('Evt_f12'), findsOneWidget);
    });

    testWidgets(
        'scrollToEventId reaches a distant event past many wrapped-title tiles',
        (tester) async {
      // Regression test for PR-077 CR follow-up ("Variable-height deep
      // links fail"): 25 long-titled (multi-line, real height >>
      // _estimatedTileExtent's 72px guess) past events followed by a
      // handful of future ones. A single fixed-estimate coarse jump would
      // land far short of 'f3' — this only reaches it if the retry/widen
      // logic in _scrollToIndex actually kicks in.
      final now = DateTime.now().toUtc();
      final events = <Event>[
        for (var i = 25; i >= 1; i--)
          _evLongTitle('p$i', EventImpact.medium, now.subtract(Duration(hours: i))),
        for (var i = 1; i <= 5; i++)
          _ev('f$i', EventImpact.high, now.add(Duration(hours: i))),
      ];
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(
          _app(vm, scrollToEventId: 'f3', isProUser: true));
      await tester.pumpAndSettle();

      expect(find.text('Evt_f3'), findsOneWidget);
    });

    testWidgets(
        'scrollToEventId reaches a distant target past many compact tiles that overshoot the estimate',
        (tester) async {
      // Regression test for PR-077 CR follow-up ("Retries Cannot Correct
      // Overshoot"): 200 short, single-line-title events, target in the
      // middle (not near the list's end, so clamping to maxScrollExtent
      // can't accidentally rescue an overshoot). Real _EventTile height
      // for compact titles is below _estimatedTileExtent's 72px guess, so
      // the first coarse jump overshoots the target — a retry that can
      // only ever widen its estimate (the original implementation) makes
      // this strictly worse each attempt and never recovers.
      final now = DateTime.now().toUtc();
      final events = <Event>[
        for (var i = 200; i >= 1; i--)
          _ev('p$i', EventImpact.medium, now.subtract(Duration(hours: i))),
      ];
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(
          _app(vm, scrollToEventId: 'p100', isProUser: true));
      await tester.pumpAndSettle();

      expect(find.text('Evt_p100'), findsOneWidget);
    });

    testWidgets(
        'scrollToEventId for a free user reaches a retained past event',
        (tester) async {
      // Regression test for PR-077 CR follow-up: a free user only ever
      // renders 2 past + 3 upcoming events (ListView.separated's actual
      // item count), but scrollToEventId's index used to be computed
      // against the FULL, uncapped event list via a separate filtering
      // pass — for a large event set, a retained event's index in the
      // full list (here, ~29th) had nothing to do with its actual
      // position (0 or 1) in the 5-item rendered list. build() and
      // _scrollToInitialPosition now share one _visibleEvents(state)
      // computation so the index always matches what's rendered.
      final events = _manyEvents(pastCount: 30, futureCount: 30);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      // p1 is the most recent past event — retained under the free-tier
      // "last 2 past" cap.
      await tester.pumpWidget(
          _app(vm, scrollToEventId: 'p1', isProUser: false));
      await tester.pumpAndSettle();

      expect(find.text('Evt_p1'), findsOneWidget);
    });

    testWidgets('default scroll positions closest future event visible',
        (tester) async {
      // 15 past + 5 future; without scroll the future events are off-screen
      final events = _manyEvents(pastCount: 15, futureCount: 5);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm));
      await tester.pumpAndSettle();

      // Closest future event (f1) should be visible
      expect(find.text('Evt_f1'), findsOneWidget);
    });

    testWidgets('all-past events scrolls to bottom', (tester) async {
      final events = _manyEvents(pastCount: 20, futureCount: 0);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm));
      await tester.pumpAndSettle();

      // Last past event (p1 — the most recent past, 1h ago) should be visible
      expect(find.text('Evt_p1'), findsOneWidget);
    });

    testWidgets('all-future events stay at top', (tester) async {
      final events = _manyEvents(pastCount: 0, futureCount: 20);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm));
      await tester.pumpAndSettle();

      // Earliest future event (f1) should be visible, still at top
      expect(find.text('Evt_f1'), findsOneWidget);
    });

    testWidgets('scrollToEventId for non-existent id falls back to default',
        (tester) async {
      final events = _manyEvents(pastCount: 15, futureCount: 5);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm, scrollToEventId: 'nonexistent'));
      await tester.pumpAndSettle();

      // No crash, screen still shows events
      expect(find.byType(MacroEventsScreen), findsOneWidget);
    });

    testWidgets('highlight persists across rebuilds', (tester) async {
      final events = _manyEvents(pastCount: 2, futureCount: 2);
      final fake = _FakeGetEvents()
        ..countryEvents = {'United States': events};
      final vm = EventsViewModel(fake);

      await tester.pumpWidget(_app(vm, scrollToEventId: 'f1'));
      await tester.pumpAndSettle();

      // Toggle filter to trigger rebuild
      await tester.tap(find.byIcon(Icons.filter_list));
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.filter_list_off));
      await tester.pumpAndSettle();

      // Green border should still be present after rebuilds
      final greenBorders = tester
          .widgetList<DecoratedBox>(find.byType(DecoratedBox))
          .where((db) {
        final dec = db.decoration;
        if (dec is BoxDecoration && dec.border is Border) {
          final border = dec.border! as Border;
          return border.top.color == Colors.green;
        }
        return false;
      });
      expect(greenBorders.length, 1);
    });
  });
}
