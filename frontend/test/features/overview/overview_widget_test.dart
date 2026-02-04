import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

class _FakeUseCase implements GetCandleSeries {
  final Duration delay;
  final List<CandleSeriesResponse> responses;

  _FakeUseCase({this.delay = Duration.zero, required this.responses});

  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    if (delay != Duration.zero) await Future.delayed(delay);
    // Return the next response in order based on symbol/timeframe matching
    final match = responses.firstWhere(
        (r) => r.symbol == input.symbol && r.timeframe == input.timeframe,
        orElse: () => responses.first);
    return match;
  }
}

Widget _wrap(Widget w) => MaterialApp(home: Scaffold(body: w));

void main() {
  testWidgets('OverviewScreen_showsLoadingState', (WidgetTester tester) async {
    final usecase = _FakeUseCase(
        delay: const Duration(milliseconds: 200),
        responses: [
          CandleSeriesResponse(symbol: 'BTCUSDT', timeframe: '1m', candles: [])
        ]);

    final widget = OverviewWidget(useCase: usecase, items: [
      GetCandleSeriesInput(
          symbol: 'BTCUSDT',
          timeframe: '1m',
          from: DateTime.utc(2024, 1, 1),
          to: DateTime.utc(2024, 1, 2))
    ]);

    await tester.pumpWidget(_wrap(widget));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpAndSettle();
  });

  testWidgets('OverviewScreen_rendersList', (WidgetTester tester) async {
    final resp1 =
        CandleSeriesResponse(symbol: 'BTCUSDT', timeframe: '1m', candles: [
      CandleDto(
          timestamp: DateTime.utc(2024, 1, 1),
          open: 1,
          high: 2,
          low: 0.5,
          close: 1.5,
          volume: 1)
    ]);
    final resp2 =
        CandleSeriesResponse(symbol: 'ETHUSD', timeframe: '5m', candles: [
      CandleDto(
          timestamp: DateTime.utc(2024, 1, 1),
          open: 2,
          high: 3,
          low: 1.5,
          close: 2.5,
          volume: 1)
    ]);
    final usecase = _FakeUseCase(responses: [resp1, resp2]);

    final widget = OverviewWidget(useCase: usecase, items: [
      GetCandleSeriesInput(
          symbol: 'BTCUSDT',
          timeframe: '1m',
          from: DateTime.utc(2024, 1, 1),
          to: DateTime.utc(2024, 1, 2)),
      GetCandleSeriesInput(
          symbol: 'ETHUSD',
          timeframe: '5m',
          from: DateTime.utc(2024, 1, 1),
          to: DateTime.utc(2024, 1, 2)),
    ]);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.textContaining('BTCUSDT'), findsOneWidget);
    expect(find.textContaining('ETHUSD'), findsOneWidget);
  });

  testWidgets('OverviewScreen_handlesEmptyCandles',
      (WidgetTester tester) async {
    final resp =
        CandleSeriesResponse(symbol: 'BTCUSDT', timeframe: '1m', candles: []);
    final usecase = _FakeUseCase(responses: [resp]);

    final widget = OverviewWidget(useCase: usecase, items: [
      GetCandleSeriesInput(
          symbol: 'BTCUSDT',
          timeframe: '1m',
          from: DateTime.utc(2024, 1, 1),
          to: DateTime.utc(2024, 1, 2)),
    ]);

    await tester.pumpWidget(_wrap(widget));
    await tester.pumpAndSettle();

    expect(find.text('No data'), findsOneWidget);
  });
}
