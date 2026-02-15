import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

class _FakeGetOverview extends GetOverview {
  final List<OverviewResult> results;
  final List<Map<String, dynamic>> calls = [];
  Exception? error;

  /// Optional per-call completers for manual control of resolution timing.
  final List<Completer<void>> _completers = [];

  _FakeGetOverview({required this.results});

  /// Adds a completer that gates the next call. The call will suspend
  /// until the completer is completed externally.
  Completer<void> addGate() {
    final c = Completer<void>();
    _completers.add(c);
    return c;
  }

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
  }) async {
    // Capture the call index at invocation time (before any awaits).
    final callIdx = calls.length;
    calls.add({
      'timeframe': timeframe,
      'page': page,
      'sort': sort,
      'snapshot': snapshot,
    });

    // If a gate was registered for this call index, wait on it.
    if (callIdx < _completers.length) {
      await _completers[callIdx].future;
    }

    if (error != null) throw error!;
    return results[callIdx % results.length];
  }
}

void main() {
  group('OverviewViewModel', () {
    late _FakeGetOverview fakeGetOverview;
    late OverviewViewModel vm;
    late int notifyCount;

    setUp(() {
      notifyCount = 0;
    });

    test('initial state is OverviewState.initial()', () {
      fakeGetOverview = _FakeGetOverview(results: []);
      vm = OverviewViewModel(fakeGetOverview);

      final state = vm.state;
      expect(state.isLoading, false);
      expect(state.items, isEmpty);
      expect(state.page, 0);
      expect(state.hasMore, true);
      expect(state.sort, 'total');
      expect(state.snapshot, isNull);
      expect(state.error, isNull);
    });

    test('loadInitial sets loading then items', () async {
      final items = [
        OverviewItem(
          symbol: 'BTCUSDT',
          candles: [
            CandleDto(
              timestamp: DateTime.utc(2024, 1, 1),
              open: 100,
              high: 110,
              low: 90,
              close: 105,
              volume: 1000,
            ),
          ],
        ),
      ];

      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: items, hasMore: true, snapshot: 'snap1'),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      final states = <OverviewState>[];
      vm.onChanged = () => states.add(
            OverviewState(
              isLoading: vm.state.isLoading,
              items: vm.state.items,
              page: vm.state.page,
              hasMore: vm.state.hasMore,
              sort: vm.state.sort,
              snapshot: vm.state.snapshot,
              error: vm.state.error,
            ),
          );

      await vm.loadInitial('1h');

      // First notification: loading = true
      expect(states[0].isLoading, true);
      expect(states[0].items, isEmpty);

      // Second notification: loading = false, items populated
      expect(states[1].isLoading, false);
      expect(states[1].items.length, 1);
      expect(states[1].items[0].symbol, 'BTCUSDT');
      expect(states[1].page, 1);
      expect(states[1].hasMore, true);
      expect(states[1].snapshot, 'snap1');
    });

    test('loadInitial clears error', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      // First: cause an error
      fakeGetOverview.error = Exception('fail');
      await vm.loadInitial('1h');
      expect(vm.state.error, isNotNull);

      // Then: succeed
      fakeGetOverview.error = null;
      await vm.loadInitial('1h');
      expect(vm.state.error, isNull);
    });

    test('loadInitial sets error on failure', () async {
      fakeGetOverview = _FakeGetOverview(results: []);
      fakeGetOverview.error = Exception('network error');
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');

      expect(vm.state.isLoading, false);
      expect(vm.state.error, contains('network error'));
      expect(vm.state.items, isEmpty);
    });

    test('loadNext appends items', () async {
      final page1Items = [
        const OverviewItem(
          symbol: 'BTCUSDT',
          candles: [],
        ),
      ];
      final page2Items = [
        const OverviewItem(
          symbol: 'ETHUSDT',
          candles: [],
        ),
      ];

      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: page1Items, hasMore: true, snapshot: 'snap1'),
        OverviewResult(items: page2Items, hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');
      expect(vm.state.items.length, 1);
      expect(vm.state.page, 1);

      await vm.loadNext('1h');
      expect(vm.state.items.length, 2);
      expect(vm.state.items[0].symbol, 'BTCUSDT');
      expect(vm.state.items[1].symbol, 'ETHUSDT');
      expect(vm.state.page, 2);
      expect(vm.state.hasMore, false);
    });

    test('loadNext does nothing when already loading', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: true),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      // Simulate a load in progress without awaiting
      final future = vm.loadInitial('1h');
      // While loading, loadNext should be a no-op
      await vm.loadNext('1h');

      await future;

      // Only loadInitial's call should have been made
      expect(fakeGetOverview.calls.length, 1);
    });

    test('loadNext does nothing when hasMore is false', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');
      expect(vm.state.hasMore, false);

      await vm.loadNext('1h');

      // Only loadInitial's call
      expect(fakeGetOverview.calls.length, 1);
    });

    test('loadNext sets error on failure without clearing items', () async {
      final items = [
        const OverviewItem(symbol: 'BTCUSDT', candles: []),
      ];

      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: items, hasMore: true),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');
      expect(vm.state.items.length, 1);

      // Now make next page fail
      fakeGetOverview.error = Exception('page 2 error');
      await vm.loadNext('1h');

      expect(vm.state.error, contains('page 2 error'));
      // Existing items preserved
      expect(vm.state.items.length, 1);
      expect(vm.state.items[0].symbol, 'BTCUSDT');
    });

    test('changeSort resets state and reloads', () async {
      final totalItems = [
        const OverviewItem(symbol: 'BTCUSDT', candles: []),
      ];
      final gainItems = [
        const OverviewItem(symbol: 'ETHUSDT', candles: []),
      ];

      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: totalItems, hasMore: false),
        OverviewResult(items: gainItems, hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');
      expect(vm.state.sort, 'total');
      expect(vm.state.items[0].symbol, 'BTCUSDT');

      // Wait for changeSort to complete its loadInitial
      vm.changeSort('gain', '1h');
      // changeSort fires loadInitial asynchronously; wait for it
      await Future.delayed(Duration.zero);

      expect(vm.state.sort, 'gain');
      expect(vm.state.items[0].symbol, 'ETHUSDT');
    });

    test('changeSort is no-op when sort is same', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');

      vm.changeSort('total', '1h');

      // No additional call — sort was already 'total'
      expect(fakeGetOverview.calls.length, 1);
    });

    test('loadInitial passes sort to GetOverview', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('4h');

      expect(fakeGetOverview.calls[0]['timeframe'], '4h');
      expect(fakeGetOverview.calls[0]['page'], 1);
      expect(fakeGetOverview.calls[0]['sort'], 'total');
    });

    test('loadNext passes snapshot to GetOverview', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: true, snapshot: 'snap-abc'),
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);

      await vm.loadInitial('1h');
      await vm.loadNext('1h');

      expect(fakeGetOverview.calls[1]['snapshot'], 'snap-abc');
      expect(fakeGetOverview.calls[1]['page'], 2);
    });

    test('onChanged callback is invoked on state changes', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = OverviewViewModel(fakeGetOverview);
      vm.onChanged = () => notifyCount++;

      await vm.loadInitial('1h');

      // At least 2 notifications: loading true, then loading false
      expect(notifyCount, greaterThanOrEqualTo(2));
    });

    test('stale loadNext does not overwrite state after sort change', () async {
      // Scenario:
      // 1. loadInitial completes with page-1 items
      // 2. loadNext starts (gated — does not complete yet)
      // 3. changeSort resets state and fires a new loadInitial
      // 4. the old loadNext completes
      // 5. its results must be discarded (stale generation)

      final page1Items = [
        const OverviewItem(symbol: 'BTCUSDT', candles: []),
      ];
      final staleNextItems = [
        const OverviewItem(symbol: 'STALE', candles: []),
      ];
      final newSortItems = [
        const OverviewItem(symbol: 'ETHUSDT', candles: []),
      ];

      fakeGetOverview = _FakeGetOverview(results: [
        // call 0: loadInitial (page 1, sort=total)
        OverviewResult(items: page1Items, hasMore: true),
        // call 1: loadNext (page 2, sort=total) — will be gated
        OverviewResult(items: staleNextItems, hasMore: false),
        // call 2: changeSort → loadInitial (page 1, sort=gain)
        OverviewResult(items: newSortItems, hasMore: false),
      ]);

      // Set up gates: call 0 passes through, call 1 is gated.
      fakeGetOverview._completers.clear();
      fakeGetOverview._completers
          .add(Completer<void>()..complete()); // call 0: pass-through
      final loadNextGate = fakeGetOverview.addGate(); // call 1: gated

      vm = OverviewViewModel(fakeGetOverview);

      // Step 1: loadInitial completes
      await vm.loadInitial('1h');
      expect(vm.state.items.length, 1);
      expect(vm.state.items[0].symbol, 'BTCUSDT');
      expect(vm.state.hasMore, true);

      // Step 2: start loadNext — it will suspend on the gate
      final loadNextFuture = vm.loadNext('1h');

      // Let microtasks run so loadNext reaches the gate await
      await Future.delayed(Duration.zero);

      // Step 3: changeSort while loadNext is in-flight
      // This increments _generation, resets state, and fires loadInitial.
      vm.changeSort('gain', '1h');

      // Let the new loadInitial (unawaited, call 2) fully resolve.
      // We pump microtasks and event loop to ensure all async continuations run.
      for (var i = 0; i < 10; i++) {
        await Future.delayed(Duration.zero);
      }

      // Verify the new loadInitial has already updated state
      expect(vm.state.sort, 'gain');
      expect(vm.state.items.length, 1);
      expect(vm.state.items[0].symbol, 'ETHUSDT');

      // Step 4: now release the stale loadNext
      loadNextGate.complete();
      await loadNextFuture;

      // Step 5: assert — stale items must NOT have been appended
      expect(vm.state.sort, 'gain');
      expect(vm.state.items.length, 1);
      expect(vm.state.items[0].symbol, 'ETHUSDT');
    });
  });
}
