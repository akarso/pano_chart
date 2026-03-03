import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/detail/chart_navigation.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';

// ---- Fake GetCandleSeries ----

class _FakeGetCandleSeries implements GetCandleSeries {
  final List<GetCandleSeriesInput> calls = [];
  CandleSeriesResponse? nextResult;
  bool shouldFail = false;

  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    calls.add(input);
    if (shouldFail) throw Exception('network error');
    return nextResult ?? _defaultSeries(input.timeframe);
  }

  CandleSeriesResponse _defaultSeries(String tf) {
    return CandleSeriesResponse(
      symbol: 'BTCUSDT',
      timeframe: tf,
      candles: List.generate(
        100,
        (i) => CandleDto(
          timestamp: DateTime.utc(2025, 1, 1, i),
          open: 100.0 + i,
          high: 110.0 + i,
          low: 90.0 + i,
          close: 105.0 + i,
          volume: 1000.0,
        ),
      ),
    );
  }
}

// ---- helpers ----

CandleSeriesResponse _fakeSeries({int count = 50, String tf = '1h'}) {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: tf,
    candles: List.generate(
      count,
      (i) => CandleDto(
        timestamp: DateTime.utc(2025, 1, 1, i),
        open: 100.0 + i,
        high: 110.0 + i,
        low: 90.0 + i,
        close: 105.0 + i,
        volume: 1000.0,
      ),
    ),
  );
}

Widget _app({
  GetCandleSeries? getCandleSeries,
  String timeframe = '1h',
  CandleSeriesResponse? series,
}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('BTCUSDT'),
      timeframe: Timeframe(timeframe),
      series: series ?? _fakeSeries(tf: timeframe),
      getCandleSeries: getCandleSeries,
    ),
  );
}

void main() {
  group('Timeframe dropdown — visibility', () {
    testWidgets('shows plain text badge when getCandleSeries is null',
        (tester) async {
      await tester.pumpWidget(_app());
      // Should display the timeframe as plain text, not a dropdown
      expect(find.text('1h'), findsOneWidget);
      expect(find.byType(DropdownButton<String>), findsNothing);
    });

    testWidgets('shows dropdown when getCandleSeries is provided',
        (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake));
      expect(find.byType(DropdownButton<String>), findsOneWidget);
    });

    testWidgets('dropdown initially shows the incoming timeframe',
        (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake, timeframe: '4h'));
      // The selected value is rendered as Text inside the dropdown
      expect(find.text('4h'), findsOneWidget);
    });
  });

  group('Timeframe dropdown — switching', () {
    testWidgets('selecting a new timeframe calls getCandleSeries.execute',
        (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake));

      // Open dropdown
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();

      // Select '5m'
      await tester.tap(find.text('5m').last);
      await tester.pumpAndSettle();

      expect(fake.calls, hasLength(1));
      expect(fake.calls.first.timeframe, '5m');
      expect(fake.calls.first.symbol, 'BTCUSDT');
    });

    testWidgets('selecting same timeframe does nothing', (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake, timeframe: '1h'));

      // Open dropdown
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();

      // Select '1h' (same as current)
      await tester.tap(find.text('1h').last);
      await tester.pumpAndSettle();

      expect(fake.calls, isEmpty);
    });

    testWidgets('after switching, time range label updates', (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake));

      // Initial label should mention '1h'
      expect(find.textContaining('1h'), findsWidgets);

      // Switch to '5m'
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('5m').last);
      await tester.pumpAndSettle();

      // After switching, the time range label should mention '5m'
      expect(find.textContaining('5m'), findsWidgets);
    });

    testWidgets('failed fetch does not change the current timeframe',
        (tester) async {
      final fake = _FakeGetCandleSeries()..shouldFail = true;
      await tester.pumpWidget(_app(getCandleSeries: fake, timeframe: '1h'));

      // Switch to '15m'
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('15m').last);
      await tester.pumpAndSettle();

      // Should still display '1h' since the fetch failed
      // The dropdown value should remain at '1h'
      expect(find.text('1h'), findsOneWidget);
    });
  });

  group('Timeframe dropdown — all timeframes present', () {
    testWidgets('dropdown contains all kTimeframes entries', (tester) async {
      final fake = _FakeGetCandleSeries();
      await tester.pumpWidget(_app(getCandleSeries: fake));

      // Open dropdown
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();

      for (final tf in kTimeframes) {
        expect(find.text(tf), findsWidgets,
            reason: 'Expected to find timeframe $tf in dropdown');
      }
    });
  });
}
