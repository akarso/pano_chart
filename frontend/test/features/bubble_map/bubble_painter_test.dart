import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_painter.dart';

void main() {
  group('BubblePainter.colorForChange', () {
    test('positive change returns green-ish colour', () {
      final c = BubblePainter.colorForChange(5.0);
      expect(c.green, greaterThan(c.red));
    });

    test('negative change returns red-ish colour', () {
      final c = BubblePainter.colorForChange(-5.0);
      expect(c.red, greaterThan(c.green));
    });

    test('near-zero change returns grey', () {
      final c = BubblePainter.colorForChange(0.0);
      expect(c, const Color(0xFF555555));
    });

    test('clamped at +10%', () {
      final c10 = BubblePainter.colorForChange(10.0);
      final c20 = BubblePainter.colorForChange(20.0);
      expect(c10, c20); // both clamped to same max green
    });

    test('clamped at -10%', () {
      final c10 = BubblePainter.colorForChange(-10.0);
      final c20 = BubblePainter.colorForChange(-20.0);
      expect(c10, c20);
    });
  });

  group('BubblePainter.regimeBorderColor', () {
    test('sideways → blue', () {
      expect(BubblePainter.regimeBorderColor('sideways'),
          const Color(0xFF42A5F5));
    });

    test('compression → yellow', () {
      expect(BubblePainter.regimeBorderColor('compression'),
          const Color(0xFFFFD600));
    });

    test('breakout → purple', () {
      expect(BubblePainter.regimeBorderColor('breakout'),
          const Color(0xFFAB47BC));
    });

    test('empty string → null', () {
      expect(BubblePainter.regimeBorderColor(''), isNull);
    });

    test('unknown badge → null', () {
      expect(BubblePainter.regimeBorderColor('other'), isNull);
    });
  });
}
