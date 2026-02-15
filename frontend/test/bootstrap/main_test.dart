import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/app/app.dart';
import 'package:pano_chart_frontend/core/config/config.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/overview/get_overview.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';

class _FakeGetOverview extends GetOverview {
  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
  }) async {
    return const OverviewResult(items: [], hasMore: false);
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

void main() {
  testWidgets('App with OverviewWidget home starts without exceptions',
      (WidgetTester tester) async {
    const config = AppConfig(apiBaseUrl: 'https://example', flavor: 'test');
    final vm = OverviewViewModel(_FakeGetOverview());
    final widget = App(
      config: config,
      home: OverviewWidget(
        viewModel: vm,
        getCandleSeries: _FakeGetCandleSeries(),
      ),
    );
    await tester.pumpWidget(widget);
    await tester.pumpAndSettle();
    // No exceptions during build — the overview widget renders
    expect(find.byType(OverviewWidget), findsOneWidget);
  });
}
