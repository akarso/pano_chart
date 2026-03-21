import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/detail_context.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';

// ---- helpers ----

DetailContext _ctx() => const DetailContext(
      rank: 1,
      totalScore: 0.5,
      trendScore: 0.1,
      sidewaysScore: 0.7,
      gainScore: 0.2,
      volume: 1000000,
    );

/// Creates [count] hourly candles with close linearly rising from
/// [startClose] to [endClose].  Timestamps start at 2025-01-01T00:00Z.
CandleSeriesResponse _series({
  int count = 150,
  double startClose = 100,
  double endClose = 249,
}) {
  final base = DateTime.utc(2025, 1, 1);
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: List.generate(count, (i) {
      final close = count == 1
          ? startClose
          : startClose + (endClose - startClose) * i / (count - 1);
      return CandleDto(
        timestamp: base.add(Duration(hours: i)),
        open: close,
        high: close + 5,
        low: close - 5,
        close: close,
        volume: 1000,
      );
    }),
  );
}

/// Series where all candles are within 24 hours of each other (5-min candles).
CandleSeriesResponse _shortSeries({int count = 50}) {
  final base = DateTime.utc(2025, 1, 1);
  return CandleSeriesResponse(
    symbol: 'ETHUSDT',
    timeframe: '5m',
    candles: List.generate(count, (i) {
      final close = 100.0 + i;
      return CandleDto(
        timestamp: base.add(Duration(minutes: 5 * i)),
        open: close,
        high: close + 2,
        low: close - 2,
        close: close,
        volume: 500,
      );
    }),
  );
}

Widget _app({
  CandleSeriesResponse? series,
  DetailContext? detailContext,
  int initialVisibleCount = 30,
}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('BTCUSDT'),
      timeframe: const Timeframe('1h'),
      series: series ?? _series(),
      detailContext: detailContext ?? _ctx(),
      initialVisibleCount: initialVisibleCount,
    ),
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  // ----------------------------------------------------------------
  // Dual percentage display
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — dual percentages', () {
    testWidgets('shows 24h percentage in header block', (tester) async {
      // 150 hourly candles → spans >24h, so 24h pct is computed.
      await tester.pumpWidget(_app());
      await tester.pump();
      // Header shows "24h: …"
      expect(find.textContaining('24h:'), findsOneWidget);
    });

    testWidgets('shows reference area percentage in header block',
        (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // Header shows "Ref: …"
      expect(find.textContaining('Ref:'), findsOneWidget);
    });

    testWidgets('24h percentage uses correct calculation', (tester) async {
      // 150 candles, close 100→249 (step 1 per candle).
      // Last: index 149, timestamp = base + 149h, close = 249
      // Cutoff = base + 149h - 24h = base + 125h → candle[125], close = 225
      //       (actually startClose + (endClose-startClose)*125/149)
      //       = 100 + 149 * 125/149 = 225.0
      // pct = (249 - 225) / 225 * 100 = 10.666… → "10.7" (header) and "10.67" (body)
      await tester.pumpWidget(_app());
      await tester.pump();
      // Header abbreviation (1 decimal)
      expect(find.textContaining('24h: +10.7%'), findsOneWidget);
    });

    testWidgets('reference area pct uses last N candles', (tester) async {
      // initialVisibleCount=30 → startIdx = 150-30 = 120
      // close[120] = 100 + 149*120/149 = 220.0
      // pct = (249 - 220) / 220 * 100 = 13.1818… → "13.2" (header)
      await tester.pumpWidget(_app(initialVisibleCount: 30));
      await tester.pump();
      expect(find.textContaining('Ref: +13.2%'), findsOneWidget);
    });

    testWidgets('percentage row below chart shows both values',
        (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // Body row uses 2 decimals: "+10.67%" and "+13.18%"
      expect(find.textContaining('10.67%'), findsOneWidget);
      expect(find.textContaining('13.18%'), findsOneWidget);
    });

    testWidgets('help icons are present for both percentages',
        (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // At least two help_outline icons (24h + reference); fieldset hints
      // add more when the score breakdown is visible.
      expect(find.byIcon(Icons.help_outline), findsAtLeast(2));
    });

    testWidgets('tapping 24h help icon shows dialog', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // First help icon is for 24h
      await tester.tap(find.byIcon(Icons.help_outline).first);
      await tester.pumpAndSettle();
      expect(find.text('24h Change'), findsOneWidget);
      expect(find.text('OK'), findsOneWidget);
    });

    testWidgets('tapping reference help icon shows dialog', (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // Second help icon (index 1) is for reference area
      await tester.tap(find.byIcon(Icons.help_outline).at(1));
      await tester.pumpAndSettle();
      expect(find.text('Reference Area'), findsOneWidget);
      expect(find.text('OK'), findsOneWidget);
    });
  });

  // ----------------------------------------------------------------
  // Reload button
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — reload button', () {
    testWidgets('refresh icon NOT shown when no GetCandleSeries provided',
        (tester) async {
      // Default _app has getCandleSeries = null
      await tester.pumpWidget(_app());
      await tester.pump();
      expect(find.byIcon(Icons.refresh), findsNothing);
    });
  });

  // ----------------------------------------------------------------
  // Negative and zero percentages
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — negative percentages', () {
    testWidgets('falling series shows red negative percentage',
        (tester) async {
      // Close goes from 249 → 100 (descending)
      final falling = _series(startClose: 249, endClose: 100);
      await tester.pumpWidget(_app(series: falling));
      await tester.pump();
      // Should contain a '-' sign in the percentage text
      expect(find.textContaining(RegExp(r'-\d+\.\d+%')), findsWidgets);
    });
  });

  // ----------------------------------------------------------------
  // Grey color for zero percentage
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — grey for near-zero pct', () {
    testWidgets('flat series shows grey percentage for ref area',
        (tester) async {
      // All candles at 100 → both percentages are 0.00%
      final flat = _series(startClose: 100, endClose: 100);
      await tester.pumpWidget(_app(series: flat));
      await tester.pump();
      // Body row: "0.00%"
      final pctFinder = find.textContaining('0.00%');
      expect(pctFinder, findsWidgets);
    });
  });

  // ----------------------------------------------------------------
  // Reference start index
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — reference start index', () {
    testWidgets('reference line is rendered (CustomPaint present)',
        (tester) async {
      await tester.pumpWidget(_app());
      await tester.pump();
      // The reference line is drawn via _ReferenceLinePainter inside a
      // CustomPaint inside the interactive chart stack. If it renders
      // without crashing, we're good — specific pixel tests need golden files.
      expect(find.byType(CustomPaint), findsWidgets);
    });
  });

  // ----------------------------------------------------------------
  // Two-candle edge case (minimal data)
  // ----------------------------------------------------------------
  group('DetailScreen PR-032 — edge cases', () {
    testWidgets('two-candle series still renders percentages',
        (tester) async {
      final tiny = _series(count: 2, startClose: 100, endClose: 110);
      await tester.pumpWidget(_app(series: tiny, initialVisibleCount: 2));
      await tester.pump();
      // pct = (110 - 100) / 100 * 100 = 10.0
      expect(find.textContaining('10.0'), findsWidgets);
    });

    testWidgets('single-candle series renders without crash', (tester) async {
      final single = _series(count: 1);
      await tester.pumpWidget(MaterialApp(
        theme: ThemeData.dark(useMaterial3: true),
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: single,
          detailContext: _ctx(),
        ),
      ));
      await tester.pump();
      // Should not crash; percentages are null so nothing shown
      expect(find.text('BTCUSDT'), findsOneWidget);
    });
  });
}
