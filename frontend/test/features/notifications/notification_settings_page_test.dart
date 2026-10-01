import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/notifications/notification_settings_page.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

Future<PreferencesService> _prefs() async {
  SharedPreferences.setMockInitialValues({});
  return PreferencesService.create();
}

/// The watchlist row sits at the bottom of a long ListView (after the
/// market/setup/macro sections) — off the default test viewport, so
/// ListView's sliver never builds it until scrolled into view.
Future<void> _scrollToWatchlistRow(WidgetTester tester) async {
  await tester.dragUntilVisible(
    find.text('Regime change alerts'),
    find.byType(ListView),
    const Offset(0, -200),
  );
  await tester.pumpAndSettle();
}

void main() {
  group('NotificationSettingsPage watchlist row (PR-101)', () {
    testWidgets('shown for a pro user', (tester) async {
      final prefs = await _prefs();

      await tester.pumpWidget(
        MaterialApp(home: NotificationSettingsPage(prefs: prefs, isPro: true)),
      );
      await tester.pumpAndSettle();
      await _scrollToWatchlistRow(tester);

      expect(find.text('Watchlist'), findsOneWidget);
      expect(find.text('Regime change alerts'), findsOneWidget);
    });

    testWidgets('hidden for a free user (pro-gated, matches backend)', (
      tester,
    ) async {
      final prefs = await _prefs();

      await tester.pumpWidget(
        MaterialApp(home: NotificationSettingsPage(prefs: prefs, isPro: false)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Regime change alerts'), findsNothing);
    });

    testWidgets('only offers the four MTF-supported timeframes', (
      tester,
    ) async {
      final prefs = await _prefs();

      await tester.pumpWidget(
        MaterialApp(home: NotificationSettingsPage(prefs: prefs, isPro: true)),
      );
      await tester.pumpAndSettle();
      await _scrollToWatchlistRow(tester);

      final row = find.ancestor(
        of: find.text('Regime change alerts'),
        matching: find.byType(Row),
      );
      await tester.tap(
        find.descendant(of: row, matching: find.byType(DropdownButton<String>)),
      );
      await tester.pumpAndSettle();

      expect(find.text('15m').hitTestable(), findsOneWidget);
      expect(find.text('1m').hitTestable(), findsNothing);
      expect(find.text('5m').hitTestable(), findsNothing);
    });

    testWidgets('toggling persists to prefs', (tester) async {
      final prefs = await _prefs();
      expect(prefs.notifyWatchlistTransitions, isTrue);

      await tester.pumpWidget(
        MaterialApp(home: NotificationSettingsPage(prefs: prefs, isPro: true)),
      );
      await tester.pumpAndSettle();
      await _scrollToWatchlistRow(tester);

      await tester.tap(
        find.widgetWithText(SwitchListTile, 'Regime change alerts'),
      );
      await tester.pumpAndSettle();

      expect(prefs.notifyWatchlistTransitions, isFalse);
    });
  });
}
