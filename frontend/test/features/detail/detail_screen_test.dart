import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/material.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';

class FakeOverviewViewModel extends OverviewViewModel {
  const FakeOverviewViewModel() : super(timeframe: const Timeframe('1h'));
  @override
  CandleSeriesResponse getCandleSeries(AppSymbol symbol, Timeframe timeframe) {
    return CandleSeriesResponse(
      symbol: symbol.value,
      timeframe: timeframe.value,
      candles: [
        CandleDto(
          timestamp: DateTime.now(),
          open: 100,
          high: 110,
          low: 90,
          close: 105,
          volume: 1000,
        ),
        CandleDto(
          timestamp: DateTime.now(),
          open: 105,
          high: 115,
          low: 100,
          close: 110,
          volume: 1200,
        ),
      ],
    );
  }
}

void main() {
  testWidgets('DetailScreen renders symbol, timeframe, chart, and favourite',
      (tester) async {
    const viewModel = FakeOverviewViewModel();
    await tester.pumpWidget(
      const MaterialApp(
        home: DetailScreen(
          symbol: AppSymbol('BTCUSDT'),
          viewModel: viewModel,
        ),
      ),
    );
    expect(find.text('BTCUSDT'), findsOneWidget);
    expect(find.text('1h'), findsOneWidget);
    expect(find.byIcon(Icons.star_border), findsOneWidget);
    // Chart is rendered via CustomPaint, so we check for CustomPaint widget
    expect(find.byType(CustomPaint), findsWidgets);
  });

  testWidgets('Tapping favourite toggles icon', (tester) async {
    const viewModel = FakeOverviewViewModel();
    await tester.pumpWidget(
      const MaterialApp(
        home: DetailScreen(
          symbol: AppSymbol('BTCUSDT'),
          viewModel: viewModel,
        ),
      ),
    );
    expect(find.byIcon(Icons.star_border), findsOneWidget);
    await tester.tap(find.byIcon(Icons.star_border));
    await tester.pumpAndSettle();
    expect(find.byIcon(Icons.star), findsOneWidget);
  });
}
