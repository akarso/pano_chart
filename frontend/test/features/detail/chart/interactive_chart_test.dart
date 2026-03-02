import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart/chart_config.dart';
import 'package:pano_chart_frontend/features/detail/chart/interactive_chart.dart';

CandleSeriesResponse _series({int count = 60}) {
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
  group('InteractiveChart', () {
    testWidgets('renders with default config', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(),
        ),
      ));
      // Should find CustomPaint widgets (candle, volume, oscillator painters)
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('shows "No data" for empty series', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: CandleSeriesResponse(
            symbol: 'X',
            timeframe: '1h',
            candles: const [],
          ),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.text('No data'), findsOneWidget);
    });

    testWidgets('renders with all indicators disabled', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(
            showEmaFast: false,
            showEmaSlow: false,
            showRsi: false,
            showAtr: false,
          ),
        ),
      ));
      // Still renders candles
      expect(find.byType(CustomPaint), findsWidgets);
      // No oscillator labels
      expect(find.text('RSI'), findsNothing);
      expect(find.text('ATR'), findsNothing);
    });

    testWidgets('shows RSI label when enabled', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(showRsi: true),
        ),
      ));
      expect(find.text('RSI'), findsOneWidget);
    });

    testWidgets('shows ATR label when enabled', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(),
          config: const ChartIndicatorConfig(showAtr: true),
        ),
      ));
      expect(find.text('ATR'), findsOneWidget);
    });

    testWidgets('responds to config changes', (tester) async {
      var cfg = const ChartIndicatorConfig(showRsi: false, showAtr: false);
      await tester.pumpWidget(_wrap(
        InteractiveChart(series: _series(), config: cfg),
      ));
      expect(find.text('RSI'), findsNothing);

      // Rebuild with RSI enabled
      cfg = cfg.copyWith(showRsi: true);
      await tester.pumpWidget(_wrap(
        InteractiveChart(series: _series(), config: cfg),
      ));
      expect(find.text('RSI'), findsOneWidget);
    });

    testWidgets('handles single candle without crash', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 1),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('handles large dataset without crash', (tester) async {
      await tester.pumpWidget(_wrap(
        InteractiveChart(
          series: _series(count: 500),
          config: const ChartIndicatorConfig(),
        ),
      ));
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });
}
