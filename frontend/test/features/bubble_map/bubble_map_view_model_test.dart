import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_map_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';

class _FakeGetOverview extends GetOverview {
  final List<OverviewResult> results;
  final List<Map<String, dynamic>> calls = [];
  Exception? error;

  _FakeGetOverview({required this.results});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
  }) async {
    final idx = calls.length;
    calls.add({
      'timeframe': timeframe,
      'page': page,
      'sort': sort,
    });
    if (error != null) throw error!;
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
  group('BubbleMapViewModel', () {
    late _FakeGetOverview fakeGetOverview;
    late BubbleMapViewModel vm;
    late int notifyCount;

    setUp(() {
      notifyCount = 0;
    });

    test('initial state', () {
      fakeGetOverview = _FakeGetOverview(results: []);
      vm = BubbleMapViewModel(fakeGetOverview);
      expect(vm.state.isLoading, false);
      expect(vm.state.bubbles, isEmpty);
      expect(vm.state.timeframe, '15m');
      expect(vm.state.pageIndex, 0);
      expect(vm.state.sizeBy, 'change');
    });

    test('load sets loading then bubbles', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [
          _item('BTCUSDT', volume: 28000000000, sparkline: [100, 102]),
          _item('ETHUSDT', volume: 14000000000, sparkline: [100, 98]),
        ], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      bool wasLoading = false;
      vm.onChanged = () {
        notifyCount++;
        if (vm.state.isLoading) wasLoading = true;
      };

      await vm.load(
        timeframe: '15m',
        pageIndex: 0,
        width: 400,
        height: 600,
      );

      expect(wasLoading, true);
      expect(vm.state.isLoading, false);
      expect(vm.state.bubbles.length, 2);
      expect(
          vm.state.bubbles.map((b) => b.token.symbol).toSet(),
          containsAll(['BTCUSDT', 'ETHUSDT']));
    });

    test('load passes correct page (1-based)', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        const OverviewResult(items: [], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(
        timeframe: '1h',
        pageIndex: 2,
        width: 400,
        height: 600,
      );

      expect(fakeGetOverview.calls.first['page'], 3);
      expect(fakeGetOverview.calls.first['timeframe'], '1h');
    });

    test('load computes priceChange from sparkline', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [
          _item('X', sparkline: [100, 110]), // +10%
        ], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 600);

      expect(vm.state.bubbles.first.token.priceChange, closeTo(10.0, 0.01));
    });

    test('load error sets error state', () async {
      fakeGetOverview = _FakeGetOverview(results: []);
      fakeGetOverview.error = Exception('network');
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 600);

      expect(vm.state.error, contains('network'));
      expect(vm.state.bubbles, isEmpty);
    });

    test('changeSizeBy repacks with new metric', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [
          _item('A', volume: 100, sparkline: [100, 101]),
          _item('B', volume: 100000, sparkline: [100, 150]),
        ], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);

      final beforeA =
          vm.state.bubbles.firstWhere((b) => b.token.symbol == 'A').radius;
      final beforeB =
          vm.state.bubbles.firstWhere((b) => b.token.symbol == 'B').radius;

      vm.changeSizeBy('change');

      expect(vm.state.sizeBy, 'change');
      // After switching, B (50% change) should be larger than A (1% change)
      final afterA =
          vm.state.bubbles.firstWhere((b) => b.token.symbol == 'A').radius;
      final afterB =
          vm.state.bubbles.firstWhere((b) => b.token.symbol == 'B').radius;
      expect(afterB, greaterThan(afterA));
    });

    test('relayout repacks without fetching', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [
          _item('BTC', volume: 1000),
        ], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);
      expect(fakeGetOverview.calls.length, 1);

      vm.relayout(800, 800);
      // No additional API call was made.
      expect(fakeGetOverview.calls.length, 1);
      expect(vm.state.bubbles.length, 1);
    });

    test('relayout is no-op with same dimensions', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('X')], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      await vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);
      notifyCount = 0;
      vm.onChanged = () => notifyCount++;

      vm.relayout(400, 400);
      expect(notifyCount, 0);
    });

    test('stale load is discarded', () async {
      fakeGetOverview = _FakeGetOverview(results: [
        OverviewResult(items: [_item('OLD')], hasMore: false),
        OverviewResult(items: [_item('NEW')], hasMore: false),
      ]);
      vm = BubbleMapViewModel(fakeGetOverview);

      // Fire two loads; the second should supersede the first.
      // ignore: unawaited_futures
      vm.load(timeframe: '15m', pageIndex: 0, width: 400, height: 400);
      await vm.load(timeframe: '1h', pageIndex: 0, width: 400, height: 400);

      // The final state should reflect the second call.
      expect(vm.state.timeframe, '1h');
    });
  });
}
