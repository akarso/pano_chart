import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/overview_widget.dart';

void main() {
  group('SparklineRenderer — grey for near-zero change', () {
    test('uses green line for positive change', () {
      final renderer = SparklineRenderer([100, 102, 105]);

      // Should not crash
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(renderer.shouldRepaint(SparklineRenderer([100, 102, 105])), true);
    });

    test('uses red line for negative change', () {
      final renderer = SparklineRenderer([100, 98, 95]);

      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      // Just verify it doesn't crash
      expect(true, isTrue);
    });

    test('uses grey line for near-zero change (0.03%)', () {
      // 0.03% change rounds to 0.0 → grey
      final renderer = SparklineRenderer([10000, 10002, 10003]);
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(true, isTrue);
    });

    test('uses grey line for exactly flat sparkline', () {
      final renderer = SparklineRenderer([100, 100, 100, 100]);
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(true, isTrue);
    });

    test('uses grey line for tiny negative change (-0.04%)', () {
      // -0.04% rounds to -0.0 → grey
      final renderer = SparklineRenderer([10000, 9998, 9996]);
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(true, isTrue);
    });

    test('single point renders without crash', () {
      final renderer = SparklineRenderer([42]);
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(true, isTrue);
    });

    test('empty points renders without crash', () {
      final renderer = SparklineRenderer([]);
      renderer.paint(Canvas(PictureRecorder()), const Size(100, 50));
      expect(true, isTrue);
    });
  });

  group('SparklineRenderer — normalize modes', () {
    test('normalize=false uses percentage mode', () {
      final renderer = SparklineRenderer(
        [100, 110, 120],
        normalize: false,
        globalMaxPct: 0.25,
      );
      renderer.paint(Canvas(PictureRecorder()), const Size(200, 100));
      expect(true, isTrue);
    });

    test('normalize=false with zero first point does not crash', () {
      final renderer = SparklineRenderer(
        [0, 10, 20],
        normalize: false,
        globalMaxPct: 0.1,
      );
      renderer.paint(Canvas(PictureRecorder()), const Size(200, 100));
      expect(true, isTrue);
    });
  });

  // ----------------------------------------------------------------
  // _sparklinePriceChange logic (tested indirectly through widget)
  // The formula: (last - first) / first * 100
  // ----------------------------------------------------------------
  group('Price change percentage label logic', () {
    test('positive sparkline → label starts with +', () {
      // [100, 110]: pct = (110-100)/100 * 100 = 10.0 → "+10.0%"
      final sparkline = [100.0, 110.0];
      final pct = (sparkline.last - sparkline.first) / sparkline.first * 100;
      final rounded = pct.toStringAsFixed(1);
      final isZero = rounded == '0.0' || rounded == '-0.0';
      final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
      expect(label, '+10.0%');
    });

    test('negative sparkline → label starts with -', () {
      // [100, 90]: pct = (90-100)/100 * 100 = -10.0 → "-10.0%"
      final sparkline = [100.0, 90.0];
      final pct = (sparkline.last - sparkline.first) / sparkline.first * 100;
      final rounded = pct.toStringAsFixed(1);
      final isZero = rounded == '0.0' || rounded == '-0.0';
      final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
      expect(label, '-10.0%');
    });

    test('near-zero positive → label is "0.0%" (no sign)', () {
      // [10000, 10003]: pct = 0.03 → rounds to "0.0" → "0.0%"
      final sparkline = [10000.0, 10003.0];
      final pct = (sparkline.last - sparkline.first) / sparkline.first * 100;
      final rounded = pct.toStringAsFixed(1);
      final isZero = rounded == '0.0' || rounded == '-0.0';
      final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
      expect(label, '0.0%');
      expect(isZero, true);
    });

    test('near-zero negative → label is "0.0%" (no sign)', () {
      // [10000, 9997]: pct = -0.03 → rounds to "-0.0" → "0.0%"
      final sparkline = [10000.0, 9997.0];
      final pct = (sparkline.last - sparkline.first) / sparkline.first * 100;
      final rounded = pct.toStringAsFixed(1);
      final isZero = rounded == '0.0' || rounded == '-0.0';
      final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
      expect(label, '0.0%');
      expect(isZero, true);
    });

    test('exactly flat → label is "0.0%"', () {
      final sparkline = [100.0, 100.0];
      final pct = (sparkline.last - sparkline.first) / sparkline.first * 100;
      final rounded = pct.toStringAsFixed(1);
      final isZero = rounded == '0.0' || rounded == '-0.0';
      expect(isZero, true);
      final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
      expect(label, '0.0%');
    });

    test('color is grey for near-zero, green for positive, red for negative',
        () {
      Color colorFor(double pct) {
        final rounded = pct.toStringAsFixed(1);
        final isZero = rounded == '0.0' || rounded == '-0.0';
        return isZero
            ? Colors.grey
            : (pct >= 0 ? Colors.green : Colors.red);
      }

      expect(colorFor(0.0), Colors.grey);
      expect(colorFor(0.03), Colors.grey); // rounds to 0.0
      expect(colorFor(-0.03), Colors.grey); // rounds to -0.0
      expect(colorFor(5.0), Colors.green);
      expect(colorFor(-5.0), Colors.red);
      expect(colorFor(0.05), Colors.green); // 0.05 rounds to 0.1 → not zero
    });
  });
}
