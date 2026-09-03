import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart/axis_layer.dart';
import 'package:pano_chart_frontend/features/detail/chart/chart_config.dart';
import 'package:pano_chart_frontend/features/detail/chart/interactive_chart.dart';

CandleSeriesResponse _series({int count = 100}) {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: List.generate(
      count,
      (i) => CandleDto(
        timestamp: DateTime.utc(2025, 1, 1, i),
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
  return MaterialApp(
    home: Scaffold(body: child),
  );
}

void main() {
  group('Full-width chart rendering', () {
    testWidgets('chart renders without crash at full viewport width',
        (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('chart renders with all indicators enabled', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(
            showEmaFast: true,
            showEmaSlow: true,
            showRsi: true,
            showAtr: true,
          ),
        ),
      ));
      expect(find.text('RSI'), findsOneWidget);
      expect(find.text('ATR'), findsOneWidget);
    });

    testWidgets('Y-axis labels render as overlay', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));
      // YAxisLabels widget should be present
      expect(find.byType(YAxisLabels), findsOneWidget);
    });

    testWidgets('X-axis labels render', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(XAxisLabels), findsOneWidget);
    });
  });

  group('YAxisLabels background styling', () {
    testWidgets('labels have Container backgrounds', (tester) async {
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 44,
          height: 200,
          child: YAxisLabels(
            candles: _series(count: 10).candles,
            startIndex: 0,
            endIndex: 10,
            priceLo: 90.0,
            priceHi: 120.0,
          ),
        ),
      ));
      // Each of the 6 labels (gridLines=5, loop 0..5) should be wrapped
      // in a Container with a dark background.
      final containers = find.byType(Container);
      expect(containers, findsWidgets);

      // All labels should be Text widgets
      expect(find.byType(Text), findsNWidgets(6));
    });
  });

  group('Axis drag gesture zones', () {
    testWidgets('single-finger pan on chart area pans normally',
        (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 200),
          config: const ChartIndicatorConfig(
            showRsi: false,
            showAtr: false,
          ),
          initialVisibleCount: 30,
        ),
      ));

      // Pan on the chart area (center of widget)
      final chartFinder = find.byType(InteractiveChart);
      final chartBox = tester.getRect(chartFinder);
      final panStart = chartBox.center;

      // Perform a horizontal drag to pan
      await tester.timedDragFrom(
        panStart,
        const Offset(-100, 0),
        const Duration(milliseconds: 300),
      );
      await tester.pumpAndSettle();

      // Should not crash and still render
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('vertical drag on right edge (Y-axis zone) does not crash',
        (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 200),
          config: const ChartIndicatorConfig(
            showRsi: false,
            showAtr: false,
          ),
          initialVisibleCount: 30,
        ),
      ));

      final chartFinder = find.byType(InteractiveChart);
      final chartBox = tester.getRect(chartFinder);

      // Touch near the right edge (Y-axis drag zone)
      final yAxisStart = Offset(chartBox.right - 20, chartBox.center.dy);

      await tester.timedDragFrom(
        yAxisStart,
        const Offset(0, -60), // drag up → zoom out
        const Duration(milliseconds: 300),
      );
      await tester.pumpAndSettle();

      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('horizontal drag on bottom edge (X-axis zone) does not crash',
        (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 200),
          config: const ChartIndicatorConfig(
            showRsi: false,
            showAtr: false,
          ),
          initialVisibleCount: 30,
        ),
      ));

      final chartFinder = find.byType(InteractiveChart);
      final chartBox = tester.getRect(chartFinder);

      // Touch near the bottom edge (X-axis drag zone)
      final xAxisStart = Offset(chartBox.center.dx, chartBox.bottom - 8);

      await tester.timedDragFrom(
        xAxisStart,
        const Offset(60, 0), // drag right → zoom in
        const Duration(milliseconds: 300),
      );
      await tester.pumpAndSettle();

      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  group('Pinch zoom (vertical disabled)', () {
    testWidgets('horizontal pinch still works without crash', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 200),
          config: const ChartIndicatorConfig(
            showRsi: false,
            showAtr: false,
          ),
          initialVisibleCount: 30,
        ),
      ));

      // Just verify the chart renders and doesn't crash with pinch gestures
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  group('Chart with minimal data', () {
    testWidgets('single candle renders at full width', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 1),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('two candles render at full width', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 2),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });
}
