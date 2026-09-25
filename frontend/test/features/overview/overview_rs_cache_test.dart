import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/infrastructure/preferences_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _FakeGetOverview extends GetOverview {
  final List<OverviewResult> results;
  int calls = 0;

  _FakeGetOverview(this.results);

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v5',
    List<String> symbols = const [],
  }) async {
    final i = calls++;
    if (i >= results.length) throw Exception('network error');
    return results[i];
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('cache round-trips RS fields including rs zero', () async {
    SharedPreferences.setMockInitialValues({});
    final prefs = PreferencesService(await SharedPreferences.getInstance());

    final result = OverviewResult(
      rsAvailable: true,
      effectiveSort: 'leaders',
      hasMore: false,
      items: const [
        OverviewItem(symbol: 'BTCUSDT', rs: 0.0, beta: 1.0, rsRank: 0.5),
        OverviewItem(symbol: 'ETHUSDT', rs: 0.02, beta: 1.1, rsRank: 1.0),
      ],
    );
    final fake = _FakeGetOverview([result]);
    final vm = OverviewViewModel(fake);
    vm.attachPrefs(prefs);
    vm.changeSortSilent('leaders');
    await vm.loadInitial('1h');

    expect(vm.state.items[0].rs, 0.0);
    expect(vm.state.rsAvailable, true);

    // Force offline: next load fails and restores cache.
    final offline = OverviewViewModel(_FakeGetOverview([]));
    offline.attachPrefs(prefs);
    offline.changeSortSilent('leaders');
    await offline.loadInitial('1h');

    expect(offline.state.error, contains('Offline'));
    expect(offline.state.rsAvailable, true);
    expect(offline.state.items[0].rs, 0.0);
    expect(offline.state.items[0].beta, 1.0);
    expect(offline.state.items[1].rs, 0.02);
    // Leaders + rsAvailable re-applies sort: ETH (0.02) before BTC (0).
    expect(
      offline.state.items.map((e) => e.symbol).toList(),
      ['ETHUSDT', 'BTCUSDT'],
    );
  });

  test('cache with mismatched sort is ignored', () async {
    SharedPreferences.setMockInitialValues({});
    final prefs = PreferencesService(await SharedPreferences.getInstance());
    await prefs.setRankingsCache(
      '1h',
      jsonEncode({
        'sort': 'volume',
        'rsAvailable': false,
        'hasMore': false,
        'items': [
          {
            'symbol': 'AAAUSDT',
            'totalScore': 1,
            'trendScore': 0,
            'sidewaysScore': 0,
            'gainScore': 0,
            'volume': 9,
            'sparkline': <double>[],
          },
        ],
      }),
    );

    final vm = OverviewViewModel(_FakeGetOverview([]));
    vm.attachPrefs(prefs);
    vm.changeSortSilent('leaders');
    await vm.loadInitial('1h');

    expect(vm.state.items, isEmpty);
    expect(vm.state.error, isNotNull);
    expect(vm.state.error, isNot(contains('Offline')));
  });

  test('prefs write failure still applies network result', () async {
    SharedPreferences.setMockInitialValues({});
    final prefs = _ThrowingPrefs(await SharedPreferences.getInstance());
    final fake = _FakeGetOverview([
      OverviewResult(
        rsAvailable: true,
        effectiveSort: 'leaders',
        hasMore: false,
        items: const [OverviewItem(symbol: 'BTCUSDT', rs: 0.01)],
      ),
    ]);
    final vm = OverviewViewModel(fake);
    vm.attachPrefs(prefs);
    vm.changeSortSilent('leaders');
    await vm.loadInitial('1h');

    expect(vm.state.error, isNull);
    expect(vm.state.items.single.symbol, 'BTCUSDT');
    expect(vm.state.rsAvailable, true);
  });

  test('refresh writes cache', () async {
    SharedPreferences.setMockInitialValues({});
    final prefs = PreferencesService(await SharedPreferences.getInstance());

    final first = OverviewResult(
      rsAvailable: false,
      effectiveSort: 'volume',
      hasMore: false,
      items: const [OverviewItem(symbol: 'OLDUSDT', volume: 1)],
    );
    final refreshed = OverviewResult(
      rsAvailable: true,
      effectiveSort: 'volume',
      hasMore: false,
      items: const [
        OverviewItem(symbol: 'NEWUSDT', volume: 2, rs: 0.01, beta: 1.0),
      ],
    );
    final fake = _FakeGetOverview([first, refreshed]);
    final vm = OverviewViewModel(fake);
    vm.attachPrefs(prefs);
    await vm.loadInitial('1h');
    await vm.refresh('1h');

    expect(vm.state.items.single.symbol, 'NEWUSDT');

    final offline = OverviewViewModel(_FakeGetOverview([]));
    offline.attachPrefs(prefs);
    await offline.loadInitial('1h');
    expect(offline.state.items.single.symbol, 'NEWUSDT');
    expect(offline.state.items.single.rs, 0.01);
    expect(offline.state.rsAvailable, true);
  });

  test('cache write snapshots sort when user flips mid-write', () async {
    SharedPreferences.setMockInitialValues({});
    final gate = Completer<void>();
    final prefs = PreferencesService(await SharedPreferences.getInstance());
    final fake = _FakeGetOverview([
      OverviewResult(
        rsAvailable: false,
        effectiveSort: 'volume',
        hasMore: false,
        items: const [OverviewItem(symbol: 'VOLUSDT', volume: 9)],
      ),
    ]);
    final vm = OverviewViewModel(fake);
    vm.attachPrefs(prefs);
    vm.cacheWriteGate = gate;
    addTearDown(() => vm.cacheWriteGate = null);
    vm.changeSortSilent('volume');
    final pending = vm.loadInitial('1h');

    for (var i = 0; i < 50 && vm.state.items.isEmpty; i++) {
      await Future<void>.delayed(Duration.zero);
    }
    expect(vm.state.items, isNotEmpty);

    // Flip sort without bumping generation — write must keep snapshotted mode.
    vm.changeSortSilent('leaders');
    gate.complete();
    await pending;

    final raw = prefs.getRankingsCache('1h');
    expect(raw, isNotNull);
    final cache = jsonDecode(raw!) as Map<String, dynamic>;
    expect(cache['sort'], 'volume');
    expect((cache['items'] as List).single['symbol'], 'VOLUSDT');
  });

  test('cache write skipped when generation advances mid-write', () async {
    SharedPreferences.setMockInitialValues({});
    final gate = Completer<void>();
    final prefs = PreferencesService(await SharedPreferences.getInstance());
    final fake = _FakeGetOverview([
      OverviewResult(
        rsAvailable: false,
        effectiveSort: 'volume',
        hasMore: false,
        items: const [OverviewItem(symbol: 'VOLUSDT', volume: 9)],
      ),
      OverviewResult(
        rsAvailable: true,
        effectiveSort: 'leaders',
        hasMore: false,
        items: const [OverviewItem(symbol: 'LEADUSDT', rs: 0.1)],
      ),
    ]);
    final vm = OverviewViewModel(fake);
    vm.attachPrefs(prefs);
    vm.cacheWriteGate = gate;
    addTearDown(() => vm.cacheWriteGate = null);
    vm.changeSortSilent('volume');
    final pending = vm.loadInitial('1h');

    for (var i = 0; i < 50 && vm.state.items.isEmpty; i++) {
      await Future<void>.delayed(Duration.zero);
    }
    expect(vm.state.items, isNotEmpty);

    // Bumps generation — first write must be skipped after the gate.
    vm.changeSort('leaders', '1h');
    gate.complete();
    await pending;
    for (var i = 0; i < 50 && vm.state.items.every((e) => e.symbol != 'LEADUSDT'); i++) {
      await Future<void>.delayed(Duration.zero);
    }

    final raw = prefs.getRankingsCache('1h');
    expect(raw, isNotNull);
    final cache = jsonDecode(raw!) as Map<String, dynamic>;
    expect(cache['sort'], 'leaders');
    expect((cache['items'] as List).single['symbol'], 'LEADUSDT');
  });
}

class _ThrowingPrefs extends PreferencesService {
  _ThrowingPrefs(super.prefs);

  @override
  Future<void> setRankingsCache(String timeframe, String json) async {
    throw Exception('disk full');
  }
}
