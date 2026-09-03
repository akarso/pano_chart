import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_painter.dart';

VolatilityPainter _make({
  List<VolatilityBucket?> aligned = const [],
  int startIndex = 0,
  int endIndex = 0,
  double candleWidth = 10.0,
  double scrollPixelOffset = 0,
}) {
  return VolatilityPainter(
    aligned: aligned,
    startIndex: startIndex,
    endIndex: endIndex,
    candleWidth: candleWidth,
    scrollPixelOffset: scrollPixelOffset,
  );
}

void main() {
  group('VolatilityPainter', () {
    test('shouldRepaint returns true when data changes', () {
      final a = _make();
      final b = _make(
        aligned: const [VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)],
        endIndex: 1,
      );
      expect(a.shouldRepaint(b), isTrue);
    });

    test('shouldRepaint returns false for identical params', () {
      const data = [VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)];
      final a = _make(aligned: data, endIndex: 1);
      final b = _make(aligned: data, endIndex: 1);
      expect(a.shouldRepaint(b), isFalse);
    });

    test('shouldRepaint detects startIndex change', () {
      const data = [
        VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0),
        VolatilityBucket(minute: 1, normalized: 1.0, spikeProb: 0),
      ];
      final a = _make(aligned: data, startIndex: 0, endIndex: 2);
      final b = _make(aligned: data, startIndex: 1, endIndex: 2);
      expect(a.shouldRepaint(b), isTrue);
    });

    test('paint does not throw with empty data', () {
      final painter = _make();
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      painter.paint(canvas, const Size(300, 60));
      recorder.endRecording();
    });

    test('paint does not throw with valid data', () {
      const data = <VolatilityBucket?>[
        VolatilityBucket(minute: 0, normalized: 0.5, spikeProb: 0.1),
        VolatilityBucket(minute: 1, normalized: 1.5, spikeProb: 0.5),
        VolatilityBucket(minute: 2, normalized: 2.5, spikeProb: 0.9),
      ];
      final painter = _make(
        aligned: data,
        startIndex: 0,
        endIndex: 3,
        candleWidth: 100,
      );
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      painter.paint(canvas, const Size(300, 60));
      recorder.endRecording();
    });

    test('paint handles null entries gracefully', () {
      const data = <VolatilityBucket?>[
        VolatilityBucket(minute: 0, normalized: 1.2, spikeProb: 0.4),
        null,
        VolatilityBucket(minute: 2, normalized: 0.8, spikeProb: 0.7),
      ];
      final painter = _make(
        aligned: data,
        startIndex: 0,
        endIndex: 3,
        candleWidth: 100,
      );
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      painter.paint(canvas, const Size(300, 60));
      recorder.endRecording();
    });
  });
}
