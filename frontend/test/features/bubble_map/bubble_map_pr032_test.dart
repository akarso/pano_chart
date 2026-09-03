import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_map_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';

class _FakeGetOverview extends GetOverview {
  final List<OverviewResult> results;
  final List<Map<String, dynamic>> calls = [];

  _FakeGetOverview({required this.results});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
  }) async {
    final idx = calls.length;
    calls.add({
      'timeframe': timeframe,
      'page': page,
      'sort': sort,
    });
    return results[idx % results.length];
  }
}

OverviewItem _item(String symbol,
        {double volume = 1000,
        List<double> sparkline = const [100, 110]}) =>
    OverviewItem(
      symbol: symbol,
      volume: volume,
      sparkline: sparkline,
    );

void main() {
  group('BubbleMapViewModel PR-032 — same timeframe navigation', () {
    test('state.timeframe reflects the loaded timeframe', () async {
      final fakeOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('BTC')], hasMore: false),
      ]);
      final vm = BubbleMapViewModel(fakeOverview);

      await vm.load(timeframe: '4h', pageIndex: 0, width: 400, height: 400);

      // The state should expose '4h' so the screen can pass it to
      // the detail view without stepping down.
      expect(vm.state.timeframe, '4h');
    });

    test('switching timeframe updates state.timeframe', () async {
      final fakeOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('ETH')], hasMore: false),
        OverviewResult(items: [_item('ETH')], hasMore: false),
      ]);
      final vm = BubbleMapViewModel(fakeOverview);

      await vm.load(timeframe: '1h', pageIndex: 0, width: 400, height: 400);
      expect(vm.state.timeframe, '1h');

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);
      expect(vm.state.timeframe, '15m');
    });

    test('API call uses the exact same timeframe string', () async {
      final fakeOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('BTC')], hasMore: false),
      ]);
      final vm = BubbleMapViewModel(fakeOverview);

      await vm.load(timeframe: '4h', pageIndex: 0, width: 400, height: 400);

      // Verify the API was called with '4h', not a stepped-down timeframe
      expect(fakeOverview.calls.first['timeframe'], '4h');
    });
  });

  group('BubbleMapViewModel PR-032 — loading overlay state', () {
    test('isLoading is true during second load (timeframe switch)', () async {
      bool capturedIsLoading = false;
      bool capturedHasBubbles = false;

      final fakeOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('BTC'), _item('ETH')], hasMore: false),
        OverviewResult(items: [_item('SOL')], hasMore: false),
      ]);
      final vm = BubbleMapViewModel(fakeOverview);

      // First load to populate bubbles
      await vm.load(timeframe: '1h', pageIndex: 0, width: 400, height: 400);
      expect(vm.state.bubbles, isNotEmpty);
      expect(vm.state.isLoading, false);

      // Now switch timeframe — capture the intermediate state
      vm.onChanged = () {
        if (vm.state.isLoading && vm.state.bubbles.isNotEmpty) {
          capturedIsLoading = true;
          capturedHasBubbles = true;
        }
      };

      await vm.load(timeframe: '4h', pageIndex: 0, width: 400, height: 400);

      // During the second load, there should have been a state where
      // isLoading=true AND bubbles were not empty → overlay condition met.
      expect(capturedIsLoading, true);
      expect(capturedHasBubbles, true);
    });

    test('isLoading is false after load completes', () async {
      final fakeOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('BTC')], hasMore: false),
      ]);
      final vm = BubbleMapViewModel(fakeOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);
      expect(vm.state.isLoading, false);
      expect(vm.state.bubbles, isNotEmpty);
    });
  });
}
