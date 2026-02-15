import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

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
  }) async {
    if (delay != Duration.zero) await Future.delayed(delay);
    return result;
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

    final widget = OverviewWidget(viewModel: vm);

    await tester.pumpWidget(_wrap(widget));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpAndSettle();
  });

  testWidgets('OverviewScreen_rendersList', (WidgetTester tester) async {
    final items = [
      OverviewItem(
        symbol: 'BTCUSDT',
        candles: [
          CandleDto(
            timestamp: DateTime.utc(2024, 1, 1),
            open: 1,
            high: 2,
            low: 0.5,
            close: 1.5,
            volume: 1,
          ),
        ],
      ),
      OverviewItem(
        symbol: 'ETHUSD',
        candles: [
          CandleDto(
            timestamp: DateTime.utc(2024, 1, 1),
            open: 2,
            high: 3,
            low: 1.5,
            close: 2.5,
            volume: 1,
          ),
        ],
      ),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(viewModel: vm);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.textContaining('BTCUSDT'), findsOneWidget);
    expect(find.textContaining('ETHUSD'), findsOneWidget);
  });

  testWidgets('OverviewScreen_handlesEmptyCandles',
      (WidgetTester tester) async {
    final items = [
      const OverviewItem(symbol: 'BTCUSDT', candles: []),
    ];

    final getOverview = _FakeGetOverview(
      result: OverviewResult(items: items, hasMore: false),
    );
    final vm = OverviewViewModel(getOverview);

    final widget = OverviewWidget(viewModel: vm);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.text('No data'), findsOneWidget);
  });
}
