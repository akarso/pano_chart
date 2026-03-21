import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';

class _FakeGetOverview extends GetOverview {
  final Duration delay;
  final OverviewResult result;

  _FakeGetOverview({this.delay = Duration.zero, required this.result});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
  }) async {
    if (delay != Duration.zero) await Future.delayed(delay);
    return result;
  }
}

class _FakeGetCandleSeries implements GetCandleSeries {
  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    return CandleSeriesResponse(
      symbol: input.symbol,
      timeframe: input.timeframe,
      candles: [],
    );
  }
}

Widget _wrap(Widget w) => MaterialApp(home: Scaffold(body: w));

void main() {
  testWidgets('OverviewScreen_showsLoadingState', (WidgetTester tester) async {
    final getOverview = _FakeGetOverview(
      delay: const Duration(milliseconds: 200),
      result: const OverviewResult(items: [], hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpAndSettle();
  });

  testWidgets('OverviewScreen_rendersList', (WidgetTester tester) async {
    final items = [
      const OverviewItem(
        symbol: 'BTCUSDT',
        totalScore: 2.75,
        sparkline: [100.0, 105.0, 110.0],
      ),
      const OverviewItem(
        symbol: 'ETHUSD',
        totalScore: -1.5,
        sparkline: [200.0, 195.0, 190.0],
      ),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.textContaining('BTC'), findsOneWidget);
    expect(find.textContaining('ETH'), findsOneWidget);
  });

  testWidgets('OverviewScreen_handlesEmptySparkline',
      (WidgetTester tester) async {
    final items = [
      const OverviewItem(symbol: 'BTCUSDT'),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(
      viewModel: vm,
      getCandleSeries: _FakeGetCandleSeries(),
    );

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.text('No data'), findsOneWidget);
  });

  group('weak-signal banner', () {
    testWidgets('shows disclaimer when sort-relevant scores are below threshold',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.15,
          sidewaysScore: 0.15, // below 0.30 threshold for sideways sort
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('sideways');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsOneWidget,
      );
    });

    testWidgets('hides disclaimer when sort-relevant scores are above threshold',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.80,
          sidewaysScore: 0.80, // above threshold
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('sideways');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsNothing,
      );
    });

    testWidgets('checks trend score when sorting by trend',
        (WidgetTester tester) async {
      // High totalScore but low trendScore → banner should show.
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.80,
          sidewaysScore: 0.75,
          trendScore: 0.10, // weak trend despite high total
          sparkline: const [100.0, 101.0],
        ),
      );

      final vm = OverviewViewModel(_FakeGetOverview(
        result: OverviewResult(items: items, hasMore: false),
      ));
      vm.changeSortSilent('trend');

      await tester.pumpWidget(_wrap(
        OverviewWidget(viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
      ));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('weakly represented'),
        findsOneWidget,
      );
    });

    testWidgets('suppressed for volume/gain/losers sorts',
        (WidgetTester tester) async {
      final items = List.generate(
        5,
        (i) => OverviewItem(
          symbol: 'SYM${i}USDT',
          totalScore: 0.10, // very weak
          sparkline: const [100.0, 101.0],
        ),
      );

      for (final sort in ['volume', 'gain', 'losers']) {
        final vm = OverviewViewModel(_FakeGetOverview(
          result: OverviewResult(items: items, hasMore: false),
        ));
        vm.changeSortSilent(sort);

        await tester.pumpWidget(_wrap(
          OverviewWidget(
              viewModel: vm, getCandleSeries: _FakeGetCandleSeries()),
        ));
        await tester.pumpAndSettle();

        expect(
          find.textContaining('weakly represented'),
          findsNothing,
          reason: 'banner should be hidden for sort=$sort',
        );
      }
    });
  });
}
