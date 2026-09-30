import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/detail_context.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_api.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_controller.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

/// In-memory WatchlistApi fake, mirroring the overview widget test's fake.
class _FakeWatchlistApi implements WatchlistApi {
  List<String> symbols;
  final List<List<String>> replaceCalls = [];
  final List<List<String>> removeCalls = [];
  Object? replaceError;
  Object? removeError;
  bool holdMutations = false;
  final List<Completer<void>> _holds = [];

  _FakeWatchlistApi([List<String> initial = const []])
    : symbols = List.of(initial);

  void release() {
    final pending = List.of(_holds);
    _holds.clear();
    for (final hold in pending) {
      if (!hold.isCompleted) hold.complete();
    }
  }

  Future<void> _maybeHold() {
    if (!holdMutations) return Future<void>.value();
    final hold = Completer<void>();
    _holds.add(hold);
    return hold.future;
  }

  @override
  Future<List<String>> fetch() async => symbols;

  @override
  Future<List<String>> replace(List<String> newSymbols) async {
    replaceCalls.add(List.of(newSymbols));
    await _maybeHold();
    if (replaceError != null) throw replaceError!;
    symbols = List.of(newSymbols);
    return symbols;
  }

  @override
  Future<List<String>> remove(List<String> toRemove) async {
    removeCalls.add(List.of(toRemove));
    await _maybeHold();
    if (removeError != null) throw removeError!;
    symbols = symbols.where((s) => !toRemove.contains(s)).toList();
    return symbols;
  }
}

DetailContext _fakeContext() => const DetailContext(
  rank: 3,
  totalScore: 0.78,
  trendScore: 0.12,
  sidewaysScore: 0.82,
  gainScore: 0.04,
  volume: 18400000,
);

CandleSeriesResponse _fakeSeries() {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: [
      CandleDto(
        timestamp: DateTime.utc(2025, 1, 1),
        open: 100.0,
        high: 105.0,
        low: 95.0,
        close: 102.0,
        volume: 1000.0,
      ),
    ],
  );
}

Widget _app({bool isFavourite = false, WatchlistController? watchlist}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('BTCUSDT'),
      timeframe: const Timeframe('1h'),
      series: _fakeSeries(),
      detailContext: _fakeContext(),
      isFavourite: isFavourite,
      watchlist: watchlist,
    ),
  );
}

void main() {
  group('DetailScreen watchlist toggle (PR-101)', () {
    testWidgets('tapping the star adds the symbol locally and syncs a PUT', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final watchlistApi = _FakeWatchlistApi();
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(_app(watchlist: watchlist));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star_border), findsOneWidget);

      await tester.tap(find.byIcon(Icons.star_border));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star), findsOneWidget);
      expect(prefs.favourites, {'BTCUSDT'});
      expect(watchlistApi.replaceCalls, hasLength(1));
      expect(watchlistApi.replaceCalls.single, ['BTCUSDT']);
    });

    testWidgets('tapping a starred symbol removes it and syncs a DELETE', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('BTCUSDT');
      final watchlistApi = _FakeWatchlistApi(['BTCUSDT']);
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(_app(isFavourite: false, watchlist: watchlist));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star), findsOneWidget);

      await tester.tap(find.byIcon(Icons.star));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star_border), findsOneWidget);
      expect(prefs.favourites, isEmpty);
      expect(watchlistApi.removeCalls, hasLength(1));
      expect(watchlistApi.removeCalls.single, ['BTCUSDT']);
    });

    testWidgets(
      'renders starred when the shared watchlist already contains it',
      (tester) async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('BTCUSDT');
        final watchlist = WatchlistController(
          prefs: prefs,
          api: _FakeWatchlistApi(['BTCUSDT']),
        );

        // isFavourite defaults false — the controller, not the constructor
        // flag, decides the star. Bubble Map and deep links pass the flag
        // too, but a stale false must not un-star a watchlisted symbol.
        await tester.pumpWidget(_app(watchlist: watchlist));
        await tester.pumpAndSettle();

        expect(find.byIcon(Icons.star), findsOneWidget);
      },
    );

    testWidgets('a failed upgrade keeps the star that started it', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final watchlistApi = _FakeWatchlistApi()
        ..replaceError = Exception('offline');
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(_app(watchlist: watchlist));
      await tester.pumpAndSettle();

      await tester.tap(find.byIcon(Icons.star_border));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star), findsOneWidget);
      expect(prefs.favourites, {'BTCUSDT'});
      expect(find.text(watchlistSyncFailedMessage), findsOneWidget);
    });

    testWidgets('two taps before either sync finishes keep the last tap', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final watchlistApi = _FakeWatchlistApi()..holdMutations = true;
      final watchlist = WatchlistController(prefs: prefs, api: watchlistApi);

      await tester.pumpWidget(_app(watchlist: watchlist));
      await tester.pumpAndSettle();

      await tester.tap(find.byIcon(Icons.star_border));
      await tester.pump();
      expect(watchlistApi.replaceCalls, hasLength(1));

      await tester.tap(find.byIcon(Icons.star));
      await tester.pump();

      watchlistApi.release();
      await tester.pump();
      watchlistApi.release();
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.star_border), findsOneWidget);
      expect(prefs.favourites, isEmpty);
      expect(watchlist.contains('BTCUSDT'), isFalse);
    });

    testWidgets(
      'still persists locally when no watchlist controller is wired',
      (tester) async {
        SharedPreferences.setMockInitialValues({});

        await tester.pumpWidget(_app());
        await tester.pumpAndSettle();

        await tester.tap(find.byIcon(Icons.star_border));
        await tester.pumpAndSettle();

        expect(find.byIcon(Icons.star), findsOneWidget);
        expect(tester.takeException(), isNull);

        final prefs = PreferencesService(await SharedPreferences.getInstance());
        expect(prefs.favourites, {'BTCUSDT'});
      },
    );
  });
}
