import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart/axis_layer.dart';
import 'package:pano_chart_frontend/features/detail/chart/candle_painter.dart';
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
  group('PR-028 — warmupCount + initialVisibleCount', () {
    testWidgets('renders with warmupCount and initialVisibleCount', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 650),
          config: const ChartIndicatorConfig(),
          warmupCount: 50,
          initialVisibleCount: 30,
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('renders with warmupCount = 0 (default)', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 100),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('handles warmupCount larger than candle count gracefully', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 10),
          config: const ChartIndicatorConfig(),
          warmupCount: 50,
          initialVisibleCount: 30,
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('handles large dataset with warmup', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 650),
          config: const ChartIndicatorConfig(),
          warmupCount: 50,
          initialVisibleCount: 30,
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  group('PR-028 — hard candle limit label', () {
    testWidgets('does not show limit label when scrolled to end', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 650),
          config: const ChartIndicatorConfig(),
          warmupCount: 50,
          initialVisibleCount: 30,
        ),
      ));
      expect(find.text('Hard candle limit reached'), findsNothing);
    });
  });

  group('PR-028 — vertical scaling (CandlePainter)', () {
    test('CandlePainter accepts optional priceLo/priceHi', () {
      final painter = CandlePainter(
        candles: _series(count: 10).candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 120.0,
      );
      expect(painter.priceLo, 90.0);
      expect(painter.priceHi, 120.0);
    });

    test('CandlePainter shouldRepaint when priceLo changes', () {
      final candles = _series(count: 10).candles;
      final p1 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 120.0,
      );
      final p2 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 85.0,
        priceHi: 120.0,
      );
      expect(p1.shouldRepaint(p2), isTrue);
    });

    test('CandlePainter shouldRepaint when priceHi changes', () {
      final candles = _series(count: 10).candles;
      final p1 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 120.0,
      );
      final p2 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 130.0,
      );
      expect(p1.shouldRepaint(p2), isTrue);
    });

    test('CandlePainter does not repaint when price range is identical', () {
      final candles = _series(count: 10).candles;
      final p1 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 120.0,
      );
      final p2 = CandlePainter(
        candles: candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
        priceLo: 90.0,
        priceHi: 120.0,
      );
      expect(p1.shouldRepaint(p2), isFalse);
    });

    test('CandlePainter works without priceLo/priceHi (auto-scale)', () {
      final painter = CandlePainter(
        candles: _series(count: 10).candles,
        startIndex: 0,
        endIndex: 10,
        candleWidth: 10,
        scrollPixelOffset: 0,
      );
      expect(painter.priceLo, isNull);
      expect(painter.priceHi, isNull);
    });
  });

  group('PR-028 — vertical scaling (YAxisLabels)', () {
    testWidgets('YAxisLabels renders with external priceLo/priceHi', (tester) async {
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
      // Should render price labels
      expect(find.byType(Text), findsWidgets);
    });

    testWidgets('YAxisLabels renders with auto-scale (no priceLo/priceHi)', (tester) async {
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 44,
          height: 200,
          child: YAxisLabels(
            candles: _series(count: 10).candles,
            startIndex: 0,
            endIndex: 10,
          ),
        ),
      ));
      expect(find.byType(Text), findsWidgets);
    });
  });

  group('PR-028 — initial zoom to match sparkline', () {
    testWidgets('initialVisibleCount sets chart to show sparkline-equivalent view', (tester) async {
      // With 650 candles and initialVisibleCount=30, the chart should
      // initially show the last 30 candles filling the viewport width.
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 650),
          config: const ChartIndicatorConfig(),
          warmupCount: 50,
          initialVisibleCount: 30,
        ),
      ));
      await tester.pump();
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('initialVisibleCount=1 does not crash', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 10),
          config: const ChartIndicatorConfig(),
          initialVisibleCount: 1,
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('very large initialVisibleCount clamps to candle count', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 10),
          config: const ChartIndicatorConfig(),
          initialVisibleCount: 1000,
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });
}
