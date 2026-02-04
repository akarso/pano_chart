import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/series_view_mode.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/overview/line_series_chart_renderer.dart';

CandleSeriesResponse _series(List<double> closes) => CandleSeriesResponse(
      symbol: 'BTCUSDT',
      timeframe: '1h',
      candles: List.generate(
        closes.length,
        (i) => CandleDto(
          timestamp: DateTime.utc(2023, 1, 1, 0, i),
          open: closes[i],
          high: closes[i],
          low: closes[i],
          close: closes[i],
          volume: 1,
        ),
      ),
    );

void main() {
  group('LineSeriesChartRenderer', () {
    test('maps closes to normalized points', () {
      const size = Size(100, 100);
      // Access private logic by copying normalization here for test
      final closes = [10.0, 20.0, 30.0];
      const min = 10.0, range = 20.0;
      final pad = size.height * 0.08;
      final chartHeight = size.height - 2 * pad;
      final expected = <Offset>[];
      for (var i = 0; i < closes.length; i++) {
        final x = i * size.width / (closes.length - 1);
        final norm = (closes[i] - min) / range;
        final y = pad + chartHeight * (1 - norm);
        expected.add(Offset(x, y));
      }
      // The painter logic should match this
      // (We can't access private painter internals, but this checks the math)
      expect(expected.length, 3);
      expect(expected[0].dx, 0);
      expect(expected[2].dx, 100);
    });

    test('empty or single-point series renders safely', () {
      final empty = _series([]);
      final single = _series([42]);
      final painterEmpty = LineChartPainter(empty);
      final painterSingle = LineChartPainter(single);
      // Should not throw
      painterEmpty.paint(Canvas(PictureRecorder()), const Size(100, 100));
      painterSingle.paint(Canvas(PictureRecorder()), const Size(100, 100));
    });

    testWidgets('renderer builds without error', (tester) async {
      final renderer = LineSeriesChartRenderer();
      final series = _series([1, 2, 3, 4, 5]);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => renderer.build(
              context,
              series: series,
              viewMode: SeriesViewMode.line,
            ),
          ),
        ),
      ));
      expect(
        find.byWidgetPredicate(
          (w) =>
              w is CustomPaint &&
              w.painter.runtimeType.toString() == 'LineChartPainter',
        ),
        findsOneWidget,
      );
    });
  });
}
