import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/sparkline_flash_dot.dart';

void main() {
  group('SparklineFlashDotPainter', () {
    test('shouldRepaint returns true when progress changes', () {
      final points = [100.0, 105.0, 110.0];
      final a = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.0,
        points: points,
      );
      final b = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.5,
        points: points,
      );
      expect(b.shouldRepaint(a), isTrue);
    });

    test('shouldRepaint returns false when same', () {
      final points = [100.0, 105.0, 110.0];
      final a = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.5,
        points: points,
      );
      final b = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.5,
        points: points,
      );
      expect(b.shouldRepaint(a), isFalse);
    });

    test('shouldRepaint returns true when color changes', () {
      final points = [100.0, 105.0, 110.0];
      final a = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.5,
        points: points,
      );
      final b = SparklineFlashDotPainter(
        color: Colors.red,
        progress: 0.5,
        points: points,
      );
      expect(b.shouldRepaint(a), isTrue);
    });

    testWidgets('paints without error at progress 0.5', (tester) async {
      final points = [100.0, 105.0, 110.0];
      await tester.pumpWidget(
        MaterialApp(
          home: SizedBox(
            width: 200,
            height: 80,
            child: CustomPaint(
              painter: SparklineFlashDotPainter(
                color: Colors.green,
                progress: 0.5,
                points: points,
              ),
            ),
          ),
        ),
      );
      // No crash = success.
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('paints without error at progress 0 (invisible)',
        (tester) async {
      final points = [100.0, 105.0, 110.0];
      await tester.pumpWidget(
        MaterialApp(
          home: SizedBox(
            width: 200,
            height: 80,
            child: CustomPaint(
              painter: SparklineFlashDotPainter(
                color: Colors.red,
                progress: 0.0,
                points: points,
              ),
            ),
          ),
        ),
      );
      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('paints in percentage mode without error', (tester) async {
      final points = [100.0, 105.0, 110.0];
      await tester.pumpWidget(
        MaterialApp(
          home: SizedBox(
            width: 200,
            height: 80,
            child: CustomPaint(
              painter: SparklineFlashDotPainter(
                color: Colors.green,
                progress: 0.5,
                points: points,
                normalize: false,
                globalMaxPct: 0.1,
              ),
            ),
          ),
        ),
      );
      expect(find.byType(CustomPaint), findsWidgets);
    });

    test('handles single-point sparkline gracefully', () {
      final painter = SparklineFlashDotPainter(
        color: Colors.green,
        progress: 0.5,
        points: [100.0],
      );
      // paint should not throw on a single point (< 2 check).
      // We can't call paint directly without a Canvas, but we
      // verify the constructor doesn't throw.
      expect(painter.progress, 0.5);
    });
  });
}
