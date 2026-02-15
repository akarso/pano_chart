import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
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

    final widget = OverviewWidget(viewModel: vm);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.textContaining('BTCUSDT'), findsOneWidget);
    expect(find.textContaining('ETHUSD'), findsOneWidget);
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

    final widget = OverviewWidget(viewModel: vm);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.text('No data'), findsOneWidget);
  });
}
