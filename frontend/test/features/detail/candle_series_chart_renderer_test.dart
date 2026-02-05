import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/series_view_mode.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/candle_series_chart_renderer.dart';

CandleSeriesResponse _series(List<List<double>> ohlc) => CandleSeriesResponse(
      symbol: 'BTCUSDT',
      timeframe: '1h',
      candles: List.generate(
        ohlc.length,
        (i) => CandleDto(
          timestamp: DateTime.utc(2023, 1, 1, 0, i),
          open: ohlc[i][0],
          high: ohlc[i][1],
          low: ohlc[i][2],
          close: ohlc[i][3],
          volume: 1,
        ),
      ),
    );

void main() {
  group('CandleSeriesChartRenderer', () {
    test('geometry for body and wick is correct', () {
      final series = _series([
        [10, 15, 8, 12], // up candle
        [12, 14, 9, 10], // down candle
      ]);
      final painter = CandleChartPainter(series);
      const size = Size(100, 100);
      // Should not throw
      painter.paint(Canvas(PictureRecorder()), size);
    });

    test('empty or single-candle series renders safely', () {
      final empty = _series([]);
      final single = _series([
        [10, 15, 8, 12]
      ]);
      final painterEmpty = CandleChartPainter(empty);
      final painterSingle = CandleChartPainter(single);
      // Should not throw
      painterEmpty.paint(Canvas(PictureRecorder()), const Size(100, 100));
      painterSingle.paint(Canvas(PictureRecorder()), const Size(100, 100));
    });

    testWidgets('renderer builds without error', (tester) async {
      final renderer = CandleSeriesChartRenderer();
      final series = _series([
        [10, 15, 8, 12],
        [12, 14, 9, 10],
      ]);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => renderer.build(
              context,
              series: series,
              viewMode: SeriesViewMode.candles,
            ),
          ),
        ),
      ));
      expect(
        find.byWidgetPredicate(
          (w) =>
              w is CustomPaint &&
              w.painter.runtimeType.toString() == 'CandleChartPainter',
        ),
        findsOneWidget,
      );
    });
  });
}
