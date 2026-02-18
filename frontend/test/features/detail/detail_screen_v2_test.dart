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

  group('DetailScreen v2 — score breakdown', () {
    testWidgets('shows sideways score bar', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      // The score label in the breakdown row (not the header "Sideways v2")
      expect(find.text('Sideways'), findsOneWidget);
      expect(find.textContaining('0.82'), findsOneWidget);
    });

    testWidgets('shows trend score bar', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      expect(find.textContaining('Trend'), findsOneWidget);
      expect(find.textContaining('0.12'), findsOneWidget);
    });

    testWidgets('shows gain score bar', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pumpAndSettle();
      expect(find.textContaining('Gain'), findsOneWidget);
      expect(find.textContaining('0.04'), findsOneWidget);
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
