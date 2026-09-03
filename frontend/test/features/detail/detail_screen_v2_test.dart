import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/detail_context.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';

DetailContext _fakeContext() => const DetailContext(
      rank: 3,
      totalScore: 0.78,
      trendScore: 0.12,
      sidewaysScore: 0.82,
      gainScore: 0.04,
      volume: 18400000,
    );

CandleSeriesResponse _fakeSeries({int count = 150}) {
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

Widget _app({
  DetailContext? detailContext,
  CandleSeriesResponse? series,
}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('ETHUSDT'),
      timeframe: const Timeframe('1h'),
      series: series ?? _fakeSeries(),
      detailContext: detailContext ?? _fakeContext(),
    ),
  );
}

void main() {
  group('DetailScreen v2 — header block', () {
    testWidgets('shows symbol name', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.text('ETHUSDT'), findsOneWidget);
    });

    testWidgets('shows rank position', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.textContaining('Rank #3'), findsOneWidget);
    });

    testWidgets('shows timeframe badge', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.text('1h'), findsOneWidget);
    });
  });

  group('DetailScreen v2 — time context', () {
    testWidgets('shows candle count and timeframe', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.textContaining('150'), findsOneWidget);
      expect(find.textContaining('1h'), findsWidgets);
    });
  });

  group('DetailScreen v2 — metrics breakdown', () {
    testWidgets('shows sideways metric bar', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      // Label above bar with colon suffix
      expect(find.text('Sideways:'), findsOneWidget);
      // (0.82 / 0.94) * 0.78 ≈ 68%  (gain is separate in Price Action)
      expect(find.text('68%'), findsOneWidget);
    });

    testWidgets('shows trend metric bar', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      expect(find.text('Trend:'), findsOneWidget);
      // (0.12 / 0.94) * 0.78 ≈ 10%
      expect(find.text('10%'), findsOneWidget);
    });

    testWidgets('shows price action (gain)', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      // Gain is now in "Price Action" fieldset as a percentage
      expect(find.text('Price Action'), findsOneWidget);
      // 0.04 → 4%
      expect(find.text('4%'), findsOneWidget);
    });

    testWidgets('shows Metrics Breakdown header', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      expect(find.text('Metrics Breakdown'), findsOneWidget);
    });

    testWidgets('shows scoring window info above metrics', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      // 110 candles × 1h = 110h ≈ "4 d 14h"
      expect(
        find.textContaining('Scores computed over the last 110 candles'),
        findsOneWidget,
      );
      expect(find.textContaining('1h'), findsWidgets);
    });
  });

  group('DetailScreen v2 — chart', () {
    testWidgets('renders candlestick chart', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  group('DetailScreen v2 — favourite toggle', () {
    testWidgets('favourite toggles on tap', (tester) async {
      await tester.pumpWidget(_app());
      expect(find.byIcon(Icons.star_border), findsOneWidget);
      await tester.tap(find.byIcon(Icons.star_border));
      await tester.pumpAndSettle();
      expect(find.byIcon(Icons.star), findsOneWidget);
    });
  });

  group('DetailScreen v2 — null context graceful', () {
    testWidgets('renders without DetailContext', (tester) async {
      await tester.pumpWidget(MaterialApp(
        theme: ThemeData.dark(useMaterial3: true),
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('4h'),
          series: _fakeSeries(count: 2),
        ),
      ));
      // Should still render without crashing
      expect(find.text('BTCUSDT'), findsOneWidget);
      // No rank shown when context is null
      expect(find.textContaining('Rank'), findsNothing);
    });
  });
}
