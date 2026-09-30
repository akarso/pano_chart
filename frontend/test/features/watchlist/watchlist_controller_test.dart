import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/features/watchlist/http_watchlist_api.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_api.dart';
import 'package:pano_chart_frontend/features/watchlist/watchlist_controller.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';

class _Api implements WatchlistApi {
  List<String> symbols;
  final List<List<String>> replaceCalls = [];
  final List<List<String>> removeCalls = [];
  Object? fetchError;
  Object? replaceError;
  final List<Object?> scriptedReplaceErrors = [];
  bool commitThenTimeout = false;
  bool timeoutWithoutCommit = false;
  int timeoutsRemaining = 0;
  final List<List<String>> scriptedFetches = [];
  bool holdMutations = false;
  int fetches = 0;
  final List<Completer<void>> _holds = [];

  _Api([List<String> initial = const []]) : symbols = List.of(initial);

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
  Future<List<String>> fetch() async {
    fetches++;
    if (fetchError != null) throw fetchError!;
    if (scriptedFetches.isNotEmpty) {
      symbols = List.of(scriptedFetches.removeAt(0));
    }
    return List.of(symbols);
  }

  @override
  Future<List<String>> replace(List<String> next) async {
    replaceCalls.add(List.of(next));
    await _maybeHold();
    if (commitThenTimeout) {
      symbols = List.of(next);
      throw TimeoutException('watchlist');
    }
    if (timeoutWithoutCommit || timeoutsRemaining > 0) {
      if (timeoutsRemaining > 0) timeoutsRemaining--;
      throw TimeoutException('watchlist');
    }
    if (scriptedReplaceErrors.isNotEmpty) {
      final error = scriptedReplaceErrors.removeAt(0);
      if (error != null) throw error;
    }
    if (replaceError != null) throw replaceError!;
    symbols = List.of(next);
    return List.of(symbols);
  }

  @override
  Future<List<String>> remove(List<String> toRemove) async {
    removeCalls.add(List.of(toRemove));
    await _maybeHold();
    if (timeoutWithoutCommit || timeoutsRemaining > 0) {
      if (timeoutsRemaining > 0) timeoutsRemaining--;
      throw TimeoutException('watchlist');
    }
    for (final symbol in toRemove) {
      if (_invalidSymbol(symbol)) {
        throw HttpWatchlistApiException(
          statusCode: 400,
          message:
              'Watchlist remove error: 400 {"error":"invalid symbol \\"$symbol\\": nope"}',
        );
      }
    }
    symbols = symbols.where((symbol) => !toRemove.contains(symbol)).toList();
    return List.of(symbols);
  }
}

bool _invalidSymbol(String symbol) {
  if (symbol.isEmpty || symbol.length > 32) return true;
  if (symbol == 'WORSE') return true;
  if (symbol == 'BAD' || RegExp(r'^BAD\d+$').hasMatch(symbol)) return true;
  return false;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('WatchlistController', () {
    test(
      'uploads a non-empty local cache when the server list is empty',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('ETHUSDT');
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        expect(watchlist.symbols, {'ETHUSDT'});
        expect(prefs.favourites, {'ETHUSDT'});
        expect(prefs.watchlistMigrated, isTrue);
        expect(api.replaceCalls, hasLength(1));
        expect(api.replaceCalls.single.toSet(), {'ETHUSDT'});

        api.symbols = [];
        await watchlist.reconcile();

        expect(watchlist.symbols, isEmpty);
        expect(prefs.favourites, isEmpty);
        expect(api.replaceCalls, hasLength(1));
      },
    );

    test(
      'a failed fetch then an add does not replace with the local cache',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('ETHUSDT');
        final api = _Api(['SOLUSDT'])..fetchError = Exception('offline');
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();
        expect(watchlist.serverSnapshotKnown, isFalse);
        expect(api.replaceCalls, isEmpty);
        expect(watchlist.symbols, {'ETHUSDT'});

        final added = await watchlist.toggle('BTCUSDT');
        expect(added, isFalse);
        expect(watchlist.takeStatus(), watchlistPendingMessage);
        expect(api.replaceCalls, isEmpty);
        expect(watchlist.symbols, {'ETHUSDT', 'BTCUSDT'});

        api.fetchError = null;
        await watchlist.reconcile();

        expect(watchlist.symbols, {'SOLUSDT', 'ETHUSDT', 'BTCUSDT'});
        expect(api.replaceCalls, hasLength(1));
        expect(api.replaceCalls.single.toSet(), {
          'SOLUSDT',
          'ETHUSDT',
          'BTCUSDT',
        });
      },
    );

    test(
      'a toggle queued in the same turn as reconcile merges onto the server',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api(['SOLUSDT']);
        final watchlist = WatchlistController(prefs: prefs, api: api);

        final reconcileDone = watchlist.reconcile();
        final toggleDone = watchlist.toggle('BTCUSDT');
        await reconcileDone;
        await toggleDone;

        expect(watchlist.symbols, {'SOLUSDT', 'BTCUSDT'});
        expect(api.replaceCalls.single.toSet(), {'SOLUSDT', 'BTCUSDT'});
      },
    );

    test(
      'a timeout after the server committed refetches instead of rolling back',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);
        await watchlist.reconcile();

        api.commitThenTimeout = true;
        await watchlist.toggle('BTCUSDT');

        expect(watchlist.symbols, {'BTCUSDT'});
        expect(watchlist.serverSnapshotKnown, isTrue);
        expect(watchlist.takeStatus(), isNull);
        // The refetch sees the commit, so there is no follow-up PUT.
        expect(api.fetches, 2);
        expect(api.replaceCalls, hasLength(1));

        api.commitThenTimeout = false;
        await watchlist.toggle('BTCUSDT');
        expect(api.removeCalls.single, ['BTCUSDT']);
        expect(watchlist.symbols, isEmpty);
      },
    );

    test('the 51st symbol is rejected locally', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final existing = [for (var i = 0; i < kWatchlistMaxSymbols; i++) 'S$i'];
      final api = _Api(existing);
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();
      final replacesAfterMigrate = api.replaceCalls.length;

      final added = await watchlist.toggle('EXTRA');

      expect(added, isFalse);
      expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
      expect(watchlist.symbols.contains('EXTRA'), isFalse);
      expect(api.replaceCalls, hasLength(replacesAfterMigrate));
      expect(watchlist.takeStatus(), watchlistCapMessage);
    });

    test('a 429 rolls the star back with a rate-limit message', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api()
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 429,
          message: 'Watchlist replace error: 429 slow down',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();

      final added = await watchlist.toggle('BTCUSDT');

      expect(added, isFalse);
      expect(watchlist.symbols, isEmpty);
      expect(watchlist.takeStatus(), watchlistRateLimitedMessage);
    });

    test('a 400 rolls the star back with the cap message', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api()
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 400,
          message: 'Watchlist replace error: 400 too many symbols, max 50',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();

      final added = await watchlist.toggle('BTCUSDT');

      expect(added, isFalse);
      expect(watchlist.symbols, isEmpty);
      expect(watchlist.takeStatus(), watchlistCapMessage);
    });

    test('a cache over 50 uploads the first 50 and does not retry', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      for (var i = 0; i < kWatchlistMaxSymbols + 1; i++) {
        prefs.addFavourite('S$i');
      }
      final api = _Api();
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(api.replaceCalls.single, hasLength(kWatchlistMaxSymbols));
      expect(
        api.replaceCalls.single,
        isNot(contains('S$kWatchlistMaxSymbols')),
      );
      expect(watchlist.symbols.contains('S$kWatchlistMaxSymbols'), isFalse);
      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.takeStatus(), contains('S$kWatchlistMaxSymbols'));

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(watchlist.takeStatus(), isNull);
    });

    test(
      'removing one star from an oversized cache uploads the rest',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        for (var i = 0; i < kWatchlistMaxSymbols + 1; i++) {
          prefs.addFavourite('S$i');
        }
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);

        final removed = await watchlist.toggle('S0');

        expect(removed, isTrue);
        expect(watchlist.takeStatus(), isNull);
        expect(watchlist.symbols.contains('S0'), isFalse);
        expect(api.replaceCalls, hasLength(1));
        expect(api.replaceCalls.single, hasLength(kWatchlistMaxSymbols));
        expect(api.replaceCalls.single, isNot(contains('S0')));
      },
    );

    test(
      'an invalid symbol is skipped and is not reported as a full list',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('ETHUSDT');
        prefs.addFavourite('BAD');
        final api = _Api()
          ..scriptedReplaceErrors.add(
            const HttpWatchlistApiException(
              statusCode: 400,
              message:
                  'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
            ),
          );
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        expect(api.replaceCalls, hasLength(2));
        expect(api.replaceCalls.last.toSet(), {'ETHUSDT'});
        expect(watchlist.symbols, {'ETHUSDT'});
        expect(prefs.watchlistMigrated, isTrue);
        expect(watchlist.takeStatus(), contains('BAD'));
        expect(watchlist.takeStatus(), isNull);
      },
    );

    test(
      'a timeout refetch that merges a server symbol is not pending',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);
        await watchlist.reconcile();

        api.timeoutsRemaining = 1;
        api.scriptedFetches.add(['SOLUSDT']);
        final added = await watchlist.toggle('BTCUSDT');

        expect(added, isTrue);
        expect(watchlist.takeStatus(), isNull);
        expect(watchlist.symbols, containsAll(['BTCUSDT', 'SOLUSDT']));
        expect(api.replaceCalls.last.toSet(), {'BTCUSDT', 'SOLUSDT'});
      },
    );

    test('a timeout that does not change the server stays pending', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api();
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();

      api.timeoutWithoutCommit = true;
      final added = await watchlist.toggle('BTCUSDT');

      expect(added, isFalse);
      expect(watchlist.symbols, {'BTCUSDT'});
      expect(api.symbols, isEmpty);
      expect(watchlist.takeStatus(), watchlistPendingMessage);
      expect(api.replaceCalls, hasLength(2));
    });

    test('a failed upgrade does not undo a removal queued behind it', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      prefs.addFavourite('BTCUSDT');
      final api = _Api(['BTCUSDT'])
        ..holdMutations = true
        ..scriptedReplaceErrors.add(
          const HttpWatchlistApiException(
            statusCode: 500,
            message: 'Watchlist replace error: 500 unavailable',
          ),
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      final reconcileDone = watchlist.reconcile();
      await Future<void>.delayed(Duration.zero);
      expect(api.replaceCalls, hasLength(1));
      expect(api.replaceCalls.single, contains('BTCUSDT'));

      final removed = watchlist.toggle('BTCUSDT');
      api.holdMutations = false;
      api.release();
      await reconcileDone;
      expect(await removed, isTrue);

      expect(watchlist.symbols.contains('BTCUSDT'), isFalse);
      expect(watchlist.symbols, contains('ETHUSDT'));
      expect(api.replaceCalls.last, isNot(contains('BTCUSDT')));
      expect(prefs.watchlistMigrated, isTrue);
    });

    test('a failed upgrade keeps a removal for the retry', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      prefs.addFavourite('BTCUSDT');
      final api = _Api(['BTCUSDT'])
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 500,
          message: 'Watchlist replace error: 500 unavailable',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      final removed = await watchlist.toggle('BTCUSDT');

      expect(removed, isFalse);
      expect(watchlist.symbols.contains('BTCUSDT'), isFalse);
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(prefs.watchlistMigrated, isFalse);
      expect(watchlist.takeStatus(), watchlistSyncFailedMessage);

      api.replaceError = null;
      await watchlist.reconcile();

      expect(api.replaceCalls.last.toSet(), {'ETHUSDT'});
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(prefs.watchlistMigrated, isTrue);
    });

    test('a failed upgrade keeps the star that started the upload', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api()
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 500,
          message: 'Watchlist replace error: 500 unavailable',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      final added = await watchlist.toggle('BTCUSDT');

      expect(added, isFalse);
      expect(watchlist.symbols, {'BTCUSDT'});
      expect(prefs.favourites, {'BTCUSDT'});
      expect(prefs.watchlistMigrated, isFalse);
      expect(watchlist.takeStatus(), watchlistSyncFailedMessage);

      api.replaceError = null;
      await watchlist.reconcile();

      expect(api.replaceCalls.last.toSet(), {'BTCUSDT'});
      expect(watchlist.symbols, {'BTCUSDT'});
      expect(prefs.watchlistMigrated, isTrue);
    });

    test(
      'a failed upgrade keeps a star left pending by a failed fetch',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api()..fetchError = Exception('offline');
        final watchlist = WatchlistController(prefs: prefs, api: api);

        final added = await watchlist.toggle('BTCUSDT');

        expect(added, isFalse);
        expect(watchlist.symbols, {'BTCUSDT'});
        expect(api.replaceCalls, isEmpty);
        expect(watchlist.takeStatus(), watchlistPendingMessage);

        api.fetchError = null;
        api.replaceError = const HttpWatchlistApiException(
          statusCode: 500,
          message: 'Watchlist replace error: 500 unavailable',
        );
        await watchlist.reconcile();

        expect(watchlist.symbols, {'BTCUSDT'});
        expect(prefs.watchlistMigrated, isFalse);
        expect(api.symbols, isEmpty);

        api.replaceError = null;
        await watchlist.reconcile();

        expect(api.replaceCalls.last.toSet(), {'BTCUSDT'});
        expect(watchlist.symbols, {'BTCUSDT'});
        expect(prefs.watchlistMigrated, isTrue);
      },
    );

    test('a failed upgrade keeps a star queued behind it', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api(['SOLUSDT'])
        ..holdMutations = true
        ..scriptedReplaceErrors.add(
          const HttpWatchlistApiException(
            statusCode: 500,
            message: 'Watchlist replace error: 500 unavailable',
          ),
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      final reconcileDone = watchlist.reconcile();
      await Future<void>.delayed(Duration.zero);
      expect(api.replaceCalls, hasLength(1));

      final added = watchlist.toggle('BTCUSDT');
      api.holdMutations = false;
      api.release();
      await reconcileDone;
      expect(await added, isTrue);

      expect(watchlist.symbols, containsAll(['ETHUSDT', 'BTCUSDT', 'SOLUSDT']));
      expect(api.replaceCalls.last.toSet(), {'ETHUSDT', 'BTCUSDT', 'SOLUSDT'});
    });

    test('a too-many upgrade response does not drop local stars', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api(['SOLUSDT'])
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 400,
          message: 'Watchlist replace error: 400 too many symbols, max 50',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(prefs.watchlistMigrated, isFalse);
      expect(watchlist.symbols, {'ETHUSDT'});
      final status = watchlist.takeStatus();
      expect(status, contains(watchlistCapMessage));
      expect(status, contains(watchlistPendingMessage));

      api.replaceError = null;
      await watchlist.reconcile();

      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.symbols, containsAll(['ETHUSDT', 'SOLUSDT']));
    });

    test('an oversized union keeps this device\'s stars', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('LOCAL');
      final server = [for (var i = 0; i < kWatchlistMaxSymbols; i++) 'S$i'];
      final api = _Api(server);
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
      expect(watchlist.symbols, contains('LOCAL'));
      expect(api.replaceCalls.single, contains('LOCAL'));
      expect(api.replaceCalls.single, isNot(contains('S49')));
      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.takeStatus(), contains('S49'));
    });

    test('several invalid symbols are all named', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      prefs.addFavourite('BAD');
      prefs.addFavourite('WORSE');
      final api = _Api()
        ..scriptedReplaceErrors.add(
          const HttpWatchlistApiException(
            statusCode: 400,
            message:
                'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
          ),
        )
        ..scriptedReplaceErrors.add(
          const HttpWatchlistApiException(
            statusCode: 400,
            message:
                'Watchlist replace error: 400 {"error":"invalid symbol \\"WORSE\\": nope"}',
          ),
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(api.replaceCalls.last.toSet(), {'ETHUSDT'});
      expect(watchlist.symbols, {'ETHUSDT'});
      final status = watchlist.takeStatus();
      expect(status, contains('BAD'));
      expect(status, contains('WORSE'));
    });

    test('an empty symbol is rejected', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api();
      final watchlist = WatchlistController(prefs: prefs, api: api);

      final added = await watchlist.toggle('');

      expect(added, isFalse);
      expect(watchlist.symbols, isEmpty);
      expect(api.replaceCalls, isEmpty);
      expect(watchlist.takeStatus(), isNotNull);
    });

    test('a timed-out upgrade that the server applied is kept', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api()..commitThenTimeout = true;
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(api.fetches, 2);
      expect(watchlist.takeStatus(), isNull);
    });

    test('a timed-out upgrade that did not land says it is pending', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api()..timeoutWithoutCommit = true;
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(prefs.watchlistMigrated, isFalse);
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(api.symbols, isEmpty);
      expect(watchlist.takeStatus(), watchlistPendingMessage);
    });

    test('a full payload that is too many is not sent again', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      for (var i = 0; i < kWatchlistMaxSymbols; i++) {
        prefs.addFavourite('S$i');
      }
      final api = _Api()
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 400,
          message: 'Watchlist replace error: 400 too many symbols, max 50',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(api.replaceCalls.single, hasLength(kWatchlistMaxSymbols));
      expect(prefs.watchlistMigrated, isFalse);
      expect(prefs.watchlistBlockedUpgrade, isNotNull);
      expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
      final status = watchlist.takeStatus();
      expect(status, contains(watchlistCapMessage));
      expect(status, contains(watchlistPendingMessage));

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));

      final restarted = WatchlistController(prefs: prefs, api: api);
      expect(restarted.takeStatus(), watchlistPendingMessage);
      await restarted.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(restarted.symbols, hasLength(kWatchlistMaxSymbols));
      expect(restarted.takeStatus(), contains(watchlistPendingMessage));
    });

    test('an unrecognized 400 is not sent again', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api()
        ..replaceError = const HttpWatchlistApiException(
          statusCode: 400,
          message: 'Watchlist replace error: 400 unrecognized',
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(prefs.watchlistBlockedUpgrade, isNotNull);
      expect(prefs.watchlistUpgradePending, isTrue);
      expect(watchlist.takeStatus(), watchlistPendingMessage);

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(1));
      expect(watchlist.takeStatus(), contains(watchlistPendingMessage));

      final restarted = WatchlistController(prefs: prefs, api: api);
      expect(restarted.takeStatus(), watchlistPendingMessage);
      await restarted.reconcile();
      expect(api.replaceCalls, hasLength(1));
    });

    test('an invalid name already on the server is dropped once', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api(['BAD', 'ETHUSDT'])
        ..scriptedReplaceErrors.add(
          const HttpWatchlistApiException(
            statusCode: 400,
            message:
                'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
          ),
        );
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(api.replaceCalls, hasLength(2));
      expect(api.replaceCalls.first, contains('BAD'));
      expect(api.replaceCalls.last, isNot(contains('BAD')));
      expect(api.replaceCalls.last.toSet(), {'ETHUSDT'});
      expect(api.removeCalls, isEmpty);
      expect(prefs.watchlistBlockedUpgrade, isNull);
      expect(api.symbols, isNot(contains('BAD')));
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.takeStatus(), contains('BAD'));

      await watchlist.reconcile();

      expect(api.symbols, isNot(contains('BAD')));
      expect(watchlist.symbols, isNot(contains('BAD')));
    });

    test(
      'an invalid name the cap left off is dropped without blocking',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        for (var i = 0; i < kWatchlistMaxSymbols; i++) {
          prefs.addFavourite('S$i');
        }
        final server = [
          for (var i = 0; i < kWatchlistMaxSymbols; i++) 'S$i',
          'BAD',
        ];
        final api = _Api(server);
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        expect(api.removeCalls, isNotEmpty);
        expect(api.removeCalls.first, ['BAD']);
        expect(prefs.watchlistBlockedUpgrade, isNull);
        expect(api.replaceCalls, isNotEmpty);
        expect(api.replaceCalls.last, hasLength(kWatchlistMaxSymbols));
        expect(api.replaceCalls.last, isNot(contains('BAD')));
        expect(api.symbols, isNot(contains('BAD')));
        expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
        expect(prefs.watchlistMigrated, isTrue);
        expect(watchlist.takeStatus(), contains('BAD'));

        await watchlist.reconcile();

        expect(api.symbols, isNot(contains('BAD')));
        expect(watchlist.symbols, isNot(contains('BAD')));
      },
    );

    test(
      'a post-migration timeout does not persist the pending flag',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);
        await watchlist.reconcile();

        api.timeoutWithoutCommit = true;
        await watchlist.toggle('BTCUSDT');

        expect(watchlist.takeStatus(), watchlistPendingMessage);
        expect(prefs.watchlistUpgradePending, isFalse);

        final restarted = WatchlistController(prefs: prefs, api: api);
        expect(restarted.takeStatus(), isNull);
      },
    );

    test('a pending add survives restart and is pushed on reconcile', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api();
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();

      api.timeoutWithoutCommit = true;
      await watchlist.toggle('BTCUSDT');
      expect(watchlist.symbols, {'BTCUSDT'});
      expect(api.symbols, isEmpty);

      api.timeoutWithoutCommit = false;
      final restarted = WatchlistController(prefs: prefs, api: api);
      expect(restarted.symbols, {'BTCUSDT'});

      await restarted.reconcile();

      expect(restarted.symbols, {'BTCUSDT'});
      expect(api.symbols, ['BTCUSDT']);
      expect(api.replaceCalls, isNotEmpty);
      expect(api.replaceCalls.last, ['BTCUSDT']);
      expect(prefs.favourites, {'BTCUSDT'});
    });

    test(
      'a pending remove survives restart and is pushed on reconcile',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api(['BTCUSDT']);
        final watchlist = WatchlistController(prefs: prefs, api: api);
        await watchlist.reconcile();
        expect(watchlist.symbols, {'BTCUSDT'});

        api.timeoutWithoutCommit = true;
        await watchlist.toggle('BTCUSDT');
        expect(watchlist.symbols, isEmpty);
        expect(api.symbols, ['BTCUSDT']);

        api.timeoutWithoutCommit = false;
        final restarted = WatchlistController(prefs: prefs, api: api);
        expect(restarted.symbols, isEmpty);

        await restarted.reconcile();

        expect(restarted.symbols, isEmpty);
        expect(api.symbols, isEmpty);
        expect(api.removeCalls, isNotEmpty);
        expect(prefs.favourites, isEmpty);
      },
    );

    test(
      'a pending edit is kept when reconcile merges a server symbol',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api();
        final watchlist = WatchlistController(prefs: prefs, api: api);
        await watchlist.reconcile();

        api.timeoutWithoutCommit = true;
        await watchlist.toggle('BTCUSDT');

        api.timeoutWithoutCommit = false;
        api.symbols = ['ETHUSDT'];
        final restarted = WatchlistController(prefs: prefs, api: api);

        await restarted.reconcile();

        expect(restarted.symbols, {'BTCUSDT', 'ETHUSDT'});
        expect(api.symbols.toSet(), {'BTCUSDT', 'ETHUSDT'});
      },
    );

    test(
      'a skipped invalid frees a slot and keeps left-off names that still do not fit',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('BAD');
        for (var i = 0; i < kWatchlistMaxSymbols; i++) {
          prefs.addFavourite('S$i');
        }
        final api = _Api()
          ..scriptedReplaceErrors.add(
            const HttpWatchlistApiException(
              statusCode: 400,
              message:
                  'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
            ),
          );
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        final status = watchlist.takeStatus();
        expect(status, contains('BAD'));
        expect(watchlist.symbols.contains('BAD'), isFalse);
        // After BAD is dropped the 50 valid symbols all fit.
        expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
        expect(watchlist.symbols.contains('S49'), isTrue);
        expect(status, isNot(contains('Left off')));
      },
    );

    test(
      'left-off and invalid names both appear after the cap is rebuilt',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('BAD');
        for (var i = 0; i < kWatchlistMaxSymbols; i++) {
          prefs.addFavourite('S$i');
        }
        final server = [for (var i = 0; i < kWatchlistMaxSymbols; i++) 'T$i'];
        final api = _Api(server)
          ..scriptedReplaceErrors.add(
            const HttpWatchlistApiException(
              statusCode: 400,
              message:
                  'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
            ),
          );
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        final status = watchlist.takeStatus();
        expect(status, contains('BAD'));
        expect(status, contains('Left off'));
        expect(watchlist.symbols.contains('BAD'), isFalse);
        expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
      },
    );

    test(
      'skipping an invalid symbol keeps a server symbol that now fits',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        prefs.addFavourite('BAD');
        for (var i = 0; i < kWatchlistMaxSymbols - 1; i++) {
          prefs.addFavourite('S$i');
        }
        final api = _Api(['ETHUSDT'])
          ..scriptedReplaceErrors.add(
            const HttpWatchlistApiException(
              statusCode: 400,
              message:
                  'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD\\": nope"}',
            ),
          );
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        expect(watchlist.symbols.contains('BAD'), isFalse);
        expect(watchlist.symbols, contains('ETHUSDT'));
        expect(watchlist.symbols, hasLength(kWatchlistMaxSymbols));
        expect(api.replaceCalls.last, contains('ETHUSDT'));
        expect(api.symbols, contains('ETHUSDT'));
        expect(prefs.watchlistMigrated, isTrue);
      },
    );

    test(
      'stripping the payload to empty does not delete the server list',
      () async {
        SharedPreferences.setMockInitialValues({});
        final prefs = await PreferencesService.create();
        final api = _Api(['SOLUSDT']);
        for (var i = 0; i < kWatchlistMaxSymbols; i++) {
          prefs.addFavourite('BAD$i');
          api.scriptedReplaceErrors.add(
            HttpWatchlistApiException(
              statusCode: 400,
              message:
                  'Watchlist replace error: 400 {"error":"invalid symbol \\"BAD$i\\": nope"}',
            ),
          );
        }
        final watchlist = WatchlistController(prefs: prefs, api: api);

        await watchlist.reconcile();

        expect(api.removeCalls, isEmpty);
        expect(api.symbols, ['SOLUSDT']);
        expect(watchlist.symbols, {'SOLUSDT'});
        expect(prefs.watchlistMigrated, isTrue);
        final status = watchlist.takeStatus();
        expect(status, contains('BAD0'));
        expect(status, isNot(contains('Left off')));
        expect(status, isNot(contains('SOLUSDT')));
      },
    );

    test('a late upgrade clears the pending sentence after it lands', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      prefs.addFavourite('ETHUSDT');
      final api = _Api()..commitThenTimeout = true;
      final watchlist = WatchlistController(prefs: prefs, api: api);

      await watchlist.reconcile();

      expect(prefs.watchlistMigrated, isTrue);
      expect(watchlist.symbols, {'ETHUSDT'});
      expect(watchlist.takeStatus(), isNull);
    });

    test('a response for an older toggle is dropped', () async {
      SharedPreferences.setMockInitialValues({});
      final prefs = await PreferencesService.create();
      final api = _Api()..holdMutations = true;
      final watchlist = WatchlistController(prefs: prefs, api: api);
      await watchlist.reconcile();

      final first = watchlist.toggle('BTCUSDT');
      await Future<void>.delayed(Duration.zero);
      expect(api.replaceCalls, hasLength(1));

      final second = watchlist.toggle('BTCUSDT');
      api.release();
      await Future<void>.delayed(Duration.zero);
      api.release();
      await first;
      await second;

      expect(watchlist.symbols, isEmpty);
      expect(api.removeCalls.single, ['BTCUSDT']);
    });
  });
}
