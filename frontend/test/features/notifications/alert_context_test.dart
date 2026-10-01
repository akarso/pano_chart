import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/notifications/alert_context.dart';
import 'package:pano_chart_frontend/features/notifications/sparkline_png.dart';

void main() {
  group('AlertContext', () {
    test('fromJson reads all fields', () {
      final ctx = AlertContext.fromJson({
        'tapeRegime': 'trend',
        'tapeBias': 'up',
        'tapeConfidence': 0.62,
        'symbolScore': 0.81,
        'rs': 0.032,
        'alignment': 0.75,
        'sparkline': [1, 2, 3],
      });
      expect(ctx.tapeRegime, 'trend');
      expect(ctx.tapeBias, 'up');
      expect(ctx.tapeConfidence, 0.62);
      expect(ctx.symbolScore, 0.81);
      expect(ctx.rs, 0.032);
      expect(ctx.alignment, 0.75);
      expect(ctx.sparkline, [1.0, 2.0, 3.0]);
      expect(ctx.hasSparkline, isTrue);
    });

    test('fromDataString tolerates missing and bad JSON', () {
      expect(AlertContext.fromDataString(null).hasSparkline, isFalse);
      expect(AlertContext.fromDataString('').hasSparkline, isFalse);
      expect(AlertContext.fromDataString('not-json').hasSparkline, isFalse);
      expect(AlertContext.fromDataString('[]').hasSparkline, isFalse);
    });

    test('fromPayload reads stringified context', () {
      final ctx = AlertContext.fromPayload({
        'type': 'watchlist_transition',
        'context':
            '{"sparkline":[10,11,12],"tapeRegime":"trend","symbolScore":0.5}',
      });
      expect(ctx.tapeRegime, 'trend');
      expect(ctx.symbolScore, 0.5);
      expect(ctx.sparkline, [10.0, 11.0, 12.0]);
    });

    test('fromPayload tolerates map-shaped context and type skew', () {
      final ctx = AlertContext.fromPayload({
        'context': {
          'tapeRegime': 'trend',
          'tapeBias': 123, // wrong type → ignored
          'sparkline': [1, 2, 3],
        },
      });
      expect(ctx.tapeRegime, 'trend');
      expect(ctx.tapeBias, isNull);
      expect(ctx.sparkline, [1.0, 2.0, 3.0]);
    });

    test('fromJson caps sparkline to the last 30 points', () {
      final pts = List<double>.generate(50, (i) => i.toDouble());
      final ctx = AlertContext.fromJson({'sparkline': pts});
      expect(ctx.sparkline.length, AlertContext.maxSparklinePoints);
      expect(ctx.sparkline.first, 20);
      expect(ctx.sparkline.last, 49);
    });
  });

  group('renderSparklinePng', () {
    testWidgets('returns non-empty PNG for a 30-point series', (tester) async {
      final points = List<double>.generate(30, (i) => 100.0 + i);
      final bytes = await tester.runAsync(() => renderSparklinePng(points));
      expect(bytes, isNotNull);
      expect(bytes!.length, greaterThan(100));
      // PNG magic number.
      expect(bytes[0], 0x89);
      expect(bytes[1], 0x50);
      expect(bytes[2], 0x4E);
      expect(bytes[3], 0x47);
    });

    testWidgets('returns null for fewer than two points', (tester) async {
      expect(
        await tester.runAsync(() => renderSparklinePng(const [])),
        isNull,
      );
      expect(
        await tester.runAsync(() => renderSparklinePng(const [1])),
        isNull,
      );
    });
  });
}
