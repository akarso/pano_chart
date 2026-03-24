import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_painter.dart';

void main() {
  group('VolatilityPainter', () {
    test('shouldRepaint returns true when data changes', () {
      final a = VolatilityPainter(data: const [], start: 0, count: 0);
      final b = VolatilityPainter(
        data: const [VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)],
        start: 0,
        count: 1,
      );
      expect(a.shouldRepaint(b), isTrue);
    });

    test('shouldRepaint returns false for identical params', () {
      const data = [VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0)];
      final a = VolatilityPainter(data: data, start: 0, count: 1);
      final b = VolatilityPainter(data: data, start: 0, count: 1);
      expect(a.shouldRepaint(b), isFalse);
    });

    test('shouldRepaint detects start change', () {
      const data = [
        VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0),
        VolatilityBucket(minute: 1, normalized: 1.0, spikeProb: 0),
      ];
      final a = VolatilityPainter(data: data, start: 0, count: 1);
      final b = VolatilityPainter(data: data, start: 1, count: 1);
      expect(a.shouldRepaint(b), isTrue);
    });

    test('paint does not throw with empty data', () {
      final painter = VolatilityPainter(data: const [], start: 0, count: 0);
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      // Should simply return without error.
      painter.paint(canvas, const Size(300, 60));
      recorder.endRecording();
    });

    test('paint does not throw with valid data', () {
      const data = [
        VolatilityBucket(minute: 0, normalized: 0.5, spikeProb: 0.1),
        VolatilityBucket(minute: 1, normalized: 1.5, spikeProb: 0.5),
        VolatilityBucket(minute: 2, normalized: 2.5, spikeProb: 0.9),
      ];
      final painter = VolatilityPainter(data: data, start: 0, count: 3);
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      painter.paint(canvas, const Size(300, 60));
      recorder.endRecording();
    });
  });
}
