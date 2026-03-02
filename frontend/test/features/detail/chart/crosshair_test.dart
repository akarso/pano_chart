import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart/chart_config.dart';
import 'package:pano_chart_frontend/features/detail/chart/crosshair_overlay.dart';
import 'package:pano_chart_frontend/features/detail/chart/interactive_chart.dart';

CandleSeriesResponse _series({int count = 60}) {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: List.generate(
      count,
      (i) => CandleDto(
        timestamp: DateTime.utc(2025, 3, 1, i),
        open: 100.0 + i * 0.5,
        high: 105.0 + i * 0.5,
        low: 95.0 + i * 0.5,
        close: 102.0 + i * 0.5,
        volume: 1000.0 + i * 10,
      ),
    ),
  );
}

Widget _wrap(Widget child) {
  return MaterialApp(home: Scaffold(body: child));
}

void main() {
  group('CrosshairState', () {
    test('stores all candle and indicator values', () {
      final candle = CandleDto(
        timestamp: DateTime.utc(2025, 3, 1, 12),
        open: 100,
        high: 110,
        low: 90,
        close: 105,
        volume: 5000,
      );
      final state = CrosshairState(
        candleIndex: 5,
        x: 75.0,
        touchY: 100.0,
        candle: candle,
        emaFast: 102.5,
        emaSlow: 101.0,
        rsi: 62.4,
        atr: 420.0,
      );
      expect(state.candleIndex, 5);
      expect(state.x, 75.0);
      expect(state.touchY, 100.0);
      expect(state.candle.close, 105);
      expect(state.emaFast, 102.5);
      expect(state.emaSlow, 101.0);
      expect(state.rsi, 62.4);
      expect(state.atr, 420.0);
    });

    test('indicator values can be null', () {
      final candle = CandleDto(
        timestamp: DateTime.utc(2025, 3, 1),
        open: 100,
        high: 110,
        low: 90,
        close: 105,
        volume: 5000,
      );
      final state = CrosshairState(
        candleIndex: 0,
        x: 10.0,
        touchY: 50.0,
        candle: candle,
      );
      expect(state.emaFast, isNull);
      expect(state.emaSlow, isNull);
      expect(state.rsi, isNull);
      expect(state.atr, isNull);
    });
  });

  group('CrosshairOverlay widget', () {
    CrosshairState _makeState() {
      return CrosshairState(
        candleIndex: 5,
        x: 100.0,
        touchY: 80.0,
        candle: CandleDto(
          timestamp: DateTime.utc(2025, 3, 3, 14, 45),
          open: 63240,
          high: 63820,
          low: 62980,
          close: 63510,
          volume: 18200,
        ),
        emaFast: 63100,
        emaSlow: 62800,
        rsi: 62.4,
        atr: 420,
      );
    }

    testWidgets('renders tooltip with OHLC data', (tester) async {
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 400,
          height: 400,
          child: CrosshairOverlay(
            state: _makeState(),
            symbol: 'BTCUSDT',
            timeframe: '1h',
            priceHeight: 250,
            volumeHeight: 50,
            oscillatorHeight: 80,
            chartWidth: 360,
            priceLo: 62000,
            priceHi: 64000,
            rsiPeriod: 14,
            atrPeriod: 14,
            emaFastPeriod: 20,
            emaSlowPeriod: 50,
          ),
        ),
      ));

      // Symbol in tooltip
      expect(find.text('BTCUSDT'), findsOneWidget);
      // OHLC values
      expect(find.textContaining('O:'), findsOneWidget);
      expect(find.textContaining('H:'), findsOneWidget);
      expect(find.textContaining('L:'), findsOneWidget);
      expect(find.textContaining('C:'), findsOneWidget);
      expect(find.textContaining('Vol:'), findsOneWidget);
      // Indicators
      expect(find.textContaining('RSI(14)'), findsOneWidget);
      expect(find.textContaining('ATR(14)'), findsOneWidget);
      expect(find.textContaining('EMA(20)'), findsOneWidget);
      expect(find.textContaining('EMA(50)'), findsOneWidget);
    });

    testWidgets('hides indicator lines when values are null', (tester) async {
      final state = CrosshairState(
        candleIndex: 5,
        x: 100.0,
        touchY: 80.0,
        candle: CandleDto(
          timestamp: DateTime.utc(2025, 3, 3),
          open: 100,
          high: 110,
          low: 90,
          close: 105,
          volume: 5000,
        ),
      );

      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 400,
          height: 400,
          child: CrosshairOverlay(
            state: state,
            symbol: 'TEST',
            timeframe: '4h',
            priceHeight: 250,
            volumeHeight: 50,
            oscillatorHeight: 0,
            chartWidth: 360,
            priceLo: 80,
            priceHi: 120,
          ),
        ),
      ));

      expect(find.textContaining('RSI'), findsNothing);
      expect(find.textContaining('ATR'), findsNothing);
      expect(find.textContaining('EMA'), findsNothing);
    });

    testWidgets('renders CustomPaint for crosshair lines', (tester) async {
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 400,
          height: 400,
          child: CrosshairOverlay(
            state: _makeState(),
            symbol: 'X',
            timeframe: '1h',
            priceHeight: 250,
            volumeHeight: 50,
            oscillatorHeight: 80,
            chartWidth: 360,
            priceLo: 62000,
            priceHi: 64000,
          ),
        ),
      ));

      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  group('InteractiveChart crosshair integration', () {
    testWidgets('no crosshair overlay initially', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));

      // CrosshairOverlay should not be present initially
      expect(find.byType(CrosshairOverlay), findsNothing);
    });

    testWidgets('long press activates crosshair', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));

      // Find the chart area
      final chartFinder = find.byType(InteractiveChart);
      expect(chartFinder, findsOneWidget);

      // Perform a long press in the chart area
      final center = tester.getCenter(chartFinder);
      final gesture = await tester.startGesture(center);
      await tester.pump(const Duration(milliseconds: 600));

      // Crosshair overlay should now be present
      expect(find.byType(CrosshairOverlay), findsOneWidget);

      // Release dismisses crosshair
      await gesture.up();
      await tester.pump();
      expect(find.byType(CrosshairOverlay), findsNothing);
    });

    testWidgets('long press shows OHLC tooltip with symbol', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));

      final center = tester.getCenter(find.byType(InteractiveChart));
      final gesture = await tester.startGesture(center);
      await tester.pump(const Duration(milliseconds: 600));

      // Symbol should appear in the tooltip
      expect(find.text('BTCUSDT'), findsWidgets); // symbol in tooltip
      expect(find.textContaining('O:'), findsOneWidget);
      expect(find.textContaining('C:'), findsOneWidget);

      await gesture.up();
      await tester.pump();
    });

    testWidgets('crosshair updates on drag after long press', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));

      final center = tester.getCenter(find.byType(InteractiveChart));
      final gesture = await tester.startGesture(center);
      await tester.pump(const Duration(milliseconds: 600));

      // Move finger to the left
      await gesture.moveBy(const Offset(-50, 0));
      await tester.pump();

      // Crosshair should still be active
      expect(find.byType(CrosshairOverlay), findsOneWidget);

      await gesture.up();
      await tester.pump();
      expect(find.byType(CrosshairOverlay), findsNothing);
    });

    testWidgets('crosshair shows indicator values when enabled', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(
            showRsi: true,
            showAtr: true,
            showEmaFast: true,
            showEmaSlow: true,
          ),
        ),
      ));

      final center = tester.getCenter(find.byType(InteractiveChart));
      final gesture = await tester.startGesture(center);
      await tester.pump(const Duration(milliseconds: 600));

      // RSI and ATR should be visible in tooltip (for a candle that has
      // enough warm-up data)
      expect(find.textContaining('RSI'), findsWidgets);

      await gesture.up();
      await tester.pump();
    });
  });
}
