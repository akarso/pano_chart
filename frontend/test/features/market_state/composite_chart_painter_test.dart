import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/market_state/composite_chart_painter.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';

void main() {
  group('scoredWindowStart', () {
    test('returns 0 when window covers whole series or is unset', () {
      expect(scoredWindowStart(110, 110), 0);
      expect(scoredWindowStart(110, 0), 0);
      expect(scoredWindowStart(50, 80), 0);
    });

    test('returns prefix length when window is a suffix', () {
      expect(scoredWindowStart(200, 110), 90);
      expect(scoredWindowStart(10, 4), 6);
    });
  });

  group('olsFit', () {
    test('returns null for fewer than 2 points', () {
      expect(olsFit(const [1.0]), isNull);
      expect(olsFit(const []), isNull);
    });

    test('fits a rising series with positive slope', () {
      final fit = olsFit([100.0, 101.0, 102.0, 103.0]);
      expect(fit, isNotNull);
      final (slope, intercept) = fit!;
      expect(slope, closeTo(1.0, 1e-9));
      expect(intercept, closeTo(100.0, 1e-9));
    });

    test('fits a flat series with zero slope', () {
      final fit = olsFit([100.0, 100.0, 100.0]);
      expect(fit, isNotNull);
      final (slope, _) = fit!;
      expect(slope, closeTo(0.0, 1e-9));
    });
  });

  group('CompositeChartPainter recorded draws', () {
    test('shouldRepaint when windowBars or regression style change', () {
      final points = [
        const IndexPoint(timestamp: 1, value: 100),
        const IndexPoint(timestamp: 2, value: 101),
        const IndexPoint(timestamp: 3, value: 102),
      ];
      final a = CompositeChartPainter(
        points: points,
        lineColor: Colors.tealAccent,
        regressionColor: Colors.tealAccent,
        windowBars: 2,
        solidRegression: true,
      );
      final b = CompositeChartPainter(
        points: points,
        lineColor: Colors.tealAccent,
        regressionColor: Colors.tealAccent,
        windowBars: 2,
        solidRegression: false,
      );
      expect(a.shouldRepaint(b), isTrue);
      expect(a.shouldRepaint(a), isFalse);
    });

    test('110-bar rising / 40-bar window: fill under stroke, dim prefix, OLS',
        () {
      final points = List.generate(
        110,
        (i) => IndexPoint(timestamp: i * 1000, value: 100.0 + i * 0.2),
      );
      expect(scoredWindowStart(points.length, 40), 70);

      final log = _paintLog(
        CompositeChartPainter(
          points: points,
          lineColor: const Color(0xFF00FF00),
          regressionColor: const Color(0xFF00FFFF),
          windowBars: 40,
          solidRegression: true,
        ),
      );

      expect(log.first, 'save');
      expect(log[1], 'clipRect');
      expect(log.last, 'restore');

      final fillIdx = <int>[];
      final strokeIdx = <int>[];
      for (var i = 0; i < log.length; i++) {
        if (log[i].startsWith('path:fill')) fillIdx.add(i);
        if (log[i].startsWith('path:stroke')) strokeIdx.add(i);
      }

      // Dim prefix fill + scored fill, both before any stroke path.
      expect(fillIdx.length, 2);
      expect(strokeIdx.length, greaterThanOrEqualTo(2));
      expect(fillIdx.last, lessThan(strokeIdx.first));

      // Dim prefix stroke (alpha 102) before full-opacity scored stroke.
      final dimStroke = log.indexWhere(
        (e) => e.startsWith('path:stroke') && e.contains('a=102'),
      );
      final fullStroke = log.indexWhere(
        (e) => e.startsWith('path:stroke') && e.contains('a=255'),
      );
      expect(dimStroke, greaterThanOrEqualTo(0));
      expect(fullStroke, greaterThan(dimStroke));

      // Baseline + solid OLS in regression color.
      final lines = log.where((e) => e.startsWith('line:')).toList();
      expect(lines.length, greaterThanOrEqualTo(2));
      expect(lines.any((l) => l.contains('0xFF00FFFF')), isTrue);
    });

    test('flat series still draws fill, baseline, and OLS', () {
      final points = List.generate(
        20,
        (i) => IndexPoint(timestamp: i * 1000, value: 100.0),
      );
      final log = _paintLog(
        CompositeChartPainter(
          points: points,
          lineColor: const Color(0xFF00FF00),
          regressionColor: const Color(0xFFFF00FF),
          windowBars: 10,
          solidRegression: true,
        ),
      );

      expect(log.first, 'save');
      expect(log.contains('clipRect'), isTrue);
      expect(log.where((e) => e.startsWith('path:fill')), isNotEmpty);
      expect(log.where((e) => e.startsWith('line:')), isNotEmpty);
      expect(log.any((e) => e.contains('0xFFFF00FF')), isTrue);
      expect(log.last, 'restore');
    });

    test('OLS outside data range still draws inside clip sandwich', () {
      // Flat prefix + late spike: OLS over the last 5 bars overshoots below
      // minV (intercept 80 vs min 100), so endpoint y maps outside [0, height].
      final window = <double>[100, 100, 100, 100, 200];
      final fit = olsFit(window)!;
      final (slope, intercept) = fit;
      expect(intercept, lessThan(100));
      expect(intercept + slope * (window.length - 1), lessThanOrEqualTo(200));

      final points = [
        ...List.generate(
          15,
          (i) => IndexPoint(timestamp: i * 1000, value: 100.0),
        ),
        ...window.asMap().entries.map(
          (e) => IndexPoint(
            timestamp: (15 + e.key) * 1000,
            value: e.value,
          ),
        ),
      ];
      const size = Size(300, 200);
      final log = _paintLog(
        CompositeChartPainter(
          points: points,
          lineColor: const Color(0xFF00FF00),
          regressionColor: const Color(0xFFDEAD00),
          windowBars: 5,
          solidRegression: true,
        ),
        size: size,
      );

      final clipIdx = log.indexOf('clipRect');
      final restoreIdx = log.lastIndexOf('restore');
      final olsIdx = log.indexWhere(
        (e) => e.startsWith('line:') && e.contains('0xFFDEAD00'),
      );
      expect(clipIdx, greaterThanOrEqualTo(0));
      expect(olsIdx, greaterThan(clipIdx));
      expect(olsIdx, lessThan(restoreIdx));

      // Endpoint y must leave the paint rect so clipRect is doing work.
      final ols = log[olsIdx];
      final coords = RegExp(r'y=([-0-9.]+),([-0-9.]+)').firstMatch(ols);
      expect(coords, isNotNull);
      final y0 = double.parse(coords!.group(1)!);
      final y1 = double.parse(coords.group(2)!);
      expect(
        y0 < 0 || y0 > size.height || y1 < 0 || y1 > size.height,
        isTrue,
        reason: 'expected OLS endpoint outside [0, ${size.height}], got $y0,$y1',
      );
    });

    test('dashed OLS draws more regression segments than solid', () {
      final points = List.generate(
        30,
        (i) => IndexPoint(timestamp: i * 1000, value: 100.0 + i.toDouble()),
      );
      final solid = _paintLog(
        CompositeChartPainter(
          points: points,
          lineColor: Colors.green,
          regressionColor: const Color(0xFFABCDEF),
          windowBars: 10,
          solidRegression: true,
        ),
      );
      final dashed = _paintLog(
        CompositeChartPainter(
          points: points,
          lineColor: Colors.green,
          regressionColor: const Color(0xFFABCDEF),
          windowBars: 10,
          solidRegression: false,
        ),
      );

      final solidReg =
          solid.where((e) => e.contains('0xFFABCDEF')).length;
      final dashedReg =
          dashed.where((e) => e.contains('0xFFABCDEF')).length;
      expect(solidReg, 1);
      expect(dashedReg, greaterThan(1));
    });
  });
}

List<String> _paintLog(
  CompositeChartPainter painter, {
  Size size = const Size(300, 200),
}) {
  final canvas = _RecordingCanvas();
  painter.paint(canvas, size);
  return canvas.log;
}

/// Logs the Canvas calls CompositeChartPainter makes.
class _RecordingCanvas implements Canvas {
  final log = <String>[];

  String _tag(Paint paint) {
    final argb = paint.color.toARGB32();
    final alpha = (argb >> 24) & 0xff;
    return 'a=$alpha:0x${argb.toRadixString(16).padLeft(8, '0').toUpperCase()}';
  }

  @override
  void save() => log.add('save');

  @override
  void restore() => log.add('restore');

  @override
  void clipRect(
    ui.Rect rect, {
    ui.ClipOp clipOp = ui.ClipOp.intersect,
    bool doAntiAlias = true,
  }) =>
      log.add('clipRect');

  @override
  void drawPath(ui.Path path, Paint paint) {
    final style = paint.style == PaintingStyle.fill ? 'fill' : 'stroke';
    log.add('path:$style:${_tag(paint)}');
  }

  @override
  void drawLine(ui.Offset p1, ui.Offset p2, Paint paint) {
    log.add('line:${_tag(paint)}:y=${p1.dy},${p2.dy}');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}
