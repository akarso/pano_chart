import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/notifications/notification_router.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

void main() {
  group('NotificationRouter', () {
    late GlobalKey<NavigatorState> navKey;
    late PreferencesService prefs;

    setUp(() async {
      navKey = GlobalKey<NavigatorState>();
      SharedPreferences.setMockInitialValues({});
      prefs = await PreferencesService.create();
    });

    testWidgets('twitter type pushes social screen', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        socialScreen: () {
          pushed = true;
          return const Scaffold(body: Text('Social'));
        },
      );
      // Enable social notifications.
      prefs.notificationsEnabled = true;

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'twitter'});
      await tester.pumpAndSettle();

      expect(pushed, isTrue);
      expect(find.text('Social'), findsOneWidget);
    });

    testWidgets('disabled category suppresses navigation', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        socialScreen: () {
          pushed = true;
          return const Scaffold(body: Text('Social'));
        },
      );
      // Social notifications disabled (default is false).
      prefs.notificationsEnabled = false;

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'twitter'});
      await tester.pumpAndSettle();

      expect(pushed, isFalse);
      expect(find.text('Home'), findsOneWidget);
    });

    testWidgets('macro type pushes events screen', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        macroScreen: () {
          pushed = true;
          return const Scaffold(body: Text('Events'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'macro'});
      await tester.pumpAndSettle();

      expect(pushed, isTrue);
      expect(find.text('Events'), findsOneWidget);
    });

    testWidgets('news type pushes news screen', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        newsScreen: () {
          pushed = true;
          return const Scaffold(body: Text('News'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'news'});
      await tester.pumpAndSettle();

      expect(pushed, isTrue);
      expect(find.text('News'), findsOneWidget);
    });

    testWidgets('market type pushes market screen', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        marketScreen: (_) {
          pushed = true;
          return const Scaffold(body: Text('Market'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'market'});
      await tester.pumpAndSettle();

      expect(pushed, isTrue);
      expect(find.text('Market'), findsOneWidget);
    });

    testWidgets('market type passes timeframe from data', (tester) async {
      String? receivedTimeframe;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        marketScreen: (timeframe) {
          receivedTimeframe = timeframe;
          return Scaffold(body: Text('Market $timeframe'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'market', 'timeframe': '15m'});
      await tester.pumpAndSettle();

      expect(receivedTimeframe, '15m');
      expect(find.text('Market 15m'), findsOneWidget);
    });

    testWidgets('setup type pushes screen with symbol', (tester) async {
      String? receivedSymbol;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        setupScreen: (symbol, [timeframe]) {
          receivedSymbol = symbol;
          return Scaffold(body: Text('Setup: $symbol'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'setup', 'symbol': 'BTCUSDT'});
      await tester.pumpAndSettle();

      expect(receivedSymbol, 'BTCUSDT');
      expect(find.text('Setup: BTCUSDT'), findsOneWidget);
    });

    testWidgets('watchlist_transition pushes the symbol detail screen', (
      tester,
    ) async {
      String? receivedSymbol;
      String? receivedTimeframe;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        setupScreen: (symbol, [timeframe]) {
          receivedSymbol = symbol;
          receivedTimeframe = timeframe;
          return Scaffold(body: Text('Detail: $symbol $timeframe'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({
        'type': 'watchlist_transition',
        'symbol': 'ETHUSDT',
        'timeframe': '1h',
      });
      await tester.pumpAndSettle();

      expect(receivedSymbol, 'ETHUSDT');
      expect(receivedTimeframe, '1h');
      expect(find.text('Detail: ETHUSDT 1h'), findsOneWidget);
    });

    testWidgets('watchlist_transition forwards a non-default timeframe', (
      tester,
    ) async {
      String? receivedTimeframe;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        setupScreen: (symbol, [timeframe]) {
          receivedTimeframe = timeframe;
          return Scaffold(body: Text('Detail: $symbol $timeframe'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({
        'type': 'watchlist_transition',
        'symbol': 'BTCUSDT',
        'timeframe': '15m',
      });
      await tester.pumpAndSettle();

      expect(receivedTimeframe, '15m');
      expect(find.text('Detail: BTCUSDT 15m'), findsOneWidget);
    });

    testWidgets('disabled watchlist alerts do not navigate', (tester) async {
      var pushed = false;
      prefs.notifyWatchlistTransitions = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        setupScreen: (symbol, [timeframe]) {
          pushed = true;
          return Scaffold(body: Text('Detail: $symbol'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'watchlist_transition', 'symbol': 'ETHUSDT'});
      await tester.pumpAndSettle();

      expect(pushed, isFalse);
      expect(find.text('Home'), findsOneWidget);
    });

    testWidgets('watchlist_transition ignores an empty symbol', (tester) async {
      var pushed = false;
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        setupScreen: (symbol, [timeframe]) {
          pushed = true;
          return Scaffold(body: Text('Detail: $symbol'));
        },
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'watchlist_transition', 'symbol': ''});
      await tester.pumpAndSettle();

      expect(pushed, isFalse);
      expect(find.text('Home'), findsOneWidget);
    });

    testWidgets('unknown type does not navigate', (tester) async {
      final router = NotificationRouter(
        navigatorKey: navKey,
        prefs: prefs,
        socialScreen: () => const Scaffold(body: Text('Social')),
      );

      await tester.pumpWidget(
        MaterialApp(
          navigatorKey: navKey,
          home: const Scaffold(body: Text('Home')),
        ),
      );

      router.handle({'type': 'unknown'});
      await tester.pumpAndSettle();

      expect(find.text('Home'), findsOneWidget);
    });

    testWidgets('null navigator key state is handled gracefully', (
      tester,
    ) async {
      // Router with a key that has no navigator attached yet.
      final detachedKey = GlobalKey<NavigatorState>();
      final router = NotificationRouter(
        navigatorKey: detachedKey,
        prefs: prefs,
        socialScreen: () => const Scaffold(body: Text('Social')),
      );
      prefs.notificationsEnabled = true;

      // Should not throw.
      router.handle({'type': 'twitter'});
    });
  });
}
