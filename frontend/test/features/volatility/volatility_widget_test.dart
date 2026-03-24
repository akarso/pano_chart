import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_widget.dart';

void main() {
  group('VolatilityWidget', () {
    testWidgets('renders with correct height', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: VolatilityWidget(
              bars: [
                VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0),
                VolatilityBucket(minute: 1, normalized: 1.5, spikeProb: 0.5),
              ],
            ),
          ),
        ),
      );

      final sizedBox = tester.widget<SizedBox>(find.byType(SizedBox).first);
      expect(sizedBox.height, 60);
    });

    testWidgets('contains CustomPaint', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: VolatilityWidget(
              bars: [
                VolatilityBucket(minute: 0, normalized: 1.0, spikeProb: 0),
              ],
            ),
          ),
        ),
      );

      expect(find.byType(CustomPaint), findsWidgets);
    });

    testWidgets('renders with empty bars', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: VolatilityWidget(bars: []),
          ),
        ),
      );

      // Should render without error.
      expect(find.byType(VolatilityWidget), findsOneWidget);
    });
  });
}
