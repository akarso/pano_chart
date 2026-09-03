import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_token.dart';

void main() {
  group('BubbleToken', () {
    test('constructor stores all fields', () {
      const token = BubbleToken(
        symbol: 'BTCUSDT',
        volume: 1000.0,
        priceChange: 2.5,
        totalScore: 0.8,
        trendScore: 0.7,
        sidewaysScore: 0.3,
        gainScore: 0.9,
        badgeComponent: 'breakout',
      );

      expect(token.symbol, 'BTCUSDT');
      expect(token.volume, 1000.0);
      expect(token.priceChange, 2.5);
      expect(token.totalScore, 0.8);
      expect(token.trendScore, 0.7);
      expect(token.sidewaysScore, 0.3);
      expect(token.gainScore, 0.9);
      expect(token.badgeComponent, 'breakout');
    });

    test('defaults for optional fields', () {
      const token = BubbleToken(
        symbol: 'ETHUSDT',
        volume: 500.0,
        priceChange: -1.0,
      );

      expect(token.totalScore, 0.0);
      expect(token.trendScore, 0.0);
      expect(token.sidewaysScore, 0.0);
      expect(token.gainScore, 0.0);
      expect(token.badgeComponent, '');
    });
  });

  group('priceChangeFromSparkline', () {
    test('positive change', () {
      final change =
          BubbleToken.priceChangeFromSparkline([100.0, 105.0, 110.0]);
      expect(change, closeTo(10.0, 0.01));
    });

    test('negative change', () {
      final change =
          BubbleToken.priceChangeFromSparkline([100.0, 95.0, 90.0]);
      expect(change, closeTo(-10.0, 0.01));
    });

    test('zero change', () {
      final change =
          BubbleToken.priceChangeFromSparkline([100.0, 110.0, 100.0]);
      expect(change, closeTo(0.0, 0.01));
    });

    test('empty sparkline returns 0', () {
      expect(BubbleToken.priceChangeFromSparkline([]), 0.0);
    });

    test('single-element sparkline returns 0', () {
      expect(BubbleToken.priceChangeFromSparkline([42.0]), 0.0);
    });

    test('first value is zero returns 0', () {
      expect(BubbleToken.priceChangeFromSparkline([0.0, 10.0, 20.0]), 0.0);
    });
  });
}
