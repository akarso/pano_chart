import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_packer.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_token.dart';

BubbleToken _token(String symbol,
        {double volume = 1000, double priceChange = 1.0}) =>
    BubbleToken(symbol: symbol, volume: volume, priceChange: priceChange);

void main() {
  late BubblePacker packer;

  setUp(() {
    packer = BubblePacker();
  });

  group('BubblePacker', () {
    test('returns empty list for empty tokens', () {
      final result = packer.pack([], width: 400, height: 600);
      expect(result, isEmpty);
    });

    test('returns empty list for zero-size viewport', () {
      final tokens = [_token('BTC', volume: 10000)];
      expect(packer.pack(tokens, width: 0, height: 600), isEmpty);
      expect(packer.pack(tokens, width: 400, height: 0), isEmpty);
    });

    test('single token is placed at center', () {
      final tokens = [_token('BTC', volume: 10000)];
      final result = packer.pack(tokens, width: 400, height: 600);

      expect(result.length, 1);
      expect(result.first.token.symbol, 'BTC');
      // After centring, the single bubble should be roughly centred.
      expect(result.first.x, closeTo(200, result.first.radius + 2));
      expect(result.first.y, closeTo(300, result.first.radius + 2));
    });

    test('no overlapping bubbles', () {
      final tokens = List.generate(
          20, (i) => _token('T$i', volume: 1000.0 * (20 - i)));
      final result = packer.pack(tokens, width: 800, height: 800);

      for (var i = 0; i < result.length; i++) {
        for (var j = i + 1; j < result.length; j++) {
          final a = result[i];
          final b = result[j];
          final dx = a.x - b.x;
          final dy = a.y - b.y;
          final dist = (dx * dx + dy * dy);
          final minDist = a.radius + b.radius;
          // Allow 1px tolerance for floating point after viewport scaling.
          expect(dist, greaterThanOrEqualTo(minDist * minDist - 2.0),
              reason:
                  '${a.token.symbol} overlaps ${b.token.symbol}');
        }
      }
    });

    test('all tokens are represented in output', () {
      final tokens = [
        _token('A', volume: 100),
        _token('B', volume: 200),
        _token('C', volume: 300),
      ];
      final result = packer.pack(tokens, width: 400, height: 400);

      expect(result.length, 3);
      final symbols = result.map((b) => b.token.symbol).toSet();
      expect(symbols, containsAll(['A', 'B', 'C']));
    });

    test('higher volume produces larger radius', () {
      final tokens = [
        _token('SMALL', volume: 10),
        _token('BIG', volume: 100000000),
      ];
      final result = packer.pack(tokens, width: 400, height: 400);

      final small = result.firstWhere((b) => b.token.symbol == 'SMALL');
      final big = result.firstWhere((b) => b.token.symbol == 'BIG');
      expect(big.radius, greaterThan(small.radius));
    });

    test('radii respect minimum', () {
      final tokens = [_token('TINY', volume: 0)];
      final result = packer.pack(tokens, width: 400, height: 400);

      expect(result.first.radius, greaterThanOrEqualTo(BubblePacker.minRadius));
    });

    test('sizeBy change uses priceChange for sizing', () {
      final tokens = [
        _token('A', volume: 100, priceChange: 0.5),
        _token('B', volume: 100, priceChange: 10.0),
      ];
      final result =
          packer.pack(tokens, width: 400, height: 400, sizeBy: 'change');

      final a = result.firstWhere((b) => b.token.symbol == 'A');
      final b = result.firstWhere((b) => b.token.symbol == 'B');
      expect(b.radius, greaterThan(a.radius));
    });

    test('packs 50 tokens without overlap', () {
      final tokens = List.generate(
          50, (i) => _token('T$i', volume: 1000.0 * (50 - i)));
      final result = packer.pack(tokens, width: 800, height: 800);

      expect(result.length, 50);

      for (var i = 0; i < result.length; i++) {
        for (var j = i + 1; j < result.length; j++) {
          final a = result[i];
          final b = result[j];
          final dx = a.x - b.x;
          final dy = a.y - b.y;
          final dist = dx * dx + dy * dy;
          final minDist = a.radius + b.radius;
          // Allow 1px tolerance for floating point after viewport scaling.
          expect(dist, greaterThanOrEqualTo(minDist * minDist - 2.0),
              reason:
                  '${a.token.symbol} overlaps ${b.token.symbol}');
        }
      }
    });

    test('all 50 bubbles fit inside the viewport', () {
      final tokens = List.generate(
          50, (i) => _token('T$i', volume: 1000.0 * (50 - i)));
      final result = packer.pack(tokens, width: 400, height: 600);

      for (final b in result) {
        expect(b.x - b.radius, greaterThanOrEqualTo(-1.0),
            reason: '${b.token.symbol} extends beyond left edge');
        expect(b.x + b.radius, lessThanOrEqualTo(401.0),
            reason: '${b.token.symbol} extends beyond right edge');
        expect(b.y - b.radius, greaterThanOrEqualTo(-1.0),
            reason: '${b.token.symbol} extends beyond top edge');
        expect(b.y + b.radius, lessThanOrEqualTo(601.0),
            reason: '${b.token.symbol} extends beyond bottom edge');
      }
    });

    test('volume mode: colorValue is rank-normalised -10..+10', () {
      final tokens = [
        _token('LOW', volume: 10),
        _token('MID', volume: 500),
        _token('HIGH', volume: 100000),
      ];
      final result = packer.pack(tokens, width: 400, height: 400);

      final low = result.firstWhere((b) => b.token.symbol == 'LOW');
      final mid = result.firstWhere((b) => b.token.symbol == 'MID');
      final high = result.firstWhere((b) => b.token.symbol == 'HIGH');

      expect(low.colorValue, closeTo(-10.0, 0.01));
      expect(mid.colorValue, closeTo(0.0, 0.01));
      expect(high.colorValue, closeTo(10.0, 0.01));
    });

    test('change mode: colorValue equals priceChange', () {
      final tokens = [
        _token('A', priceChange: 5.0),
        _token('B', priceChange: -3.0),
      ];
      final result =
          packer.pack(tokens, width: 400, height: 400, sizeBy: 'change');

      final a = result.firstWhere((b) => b.token.symbol == 'A');
      final bBubble = result.firstWhere((b) => b.token.symbol == 'B');

      expect(a.colorValue, closeTo(5.0, 0.01));
      expect(bBubble.colorValue, closeTo(-3.0, 0.01));
    });

    test('single token in volume mode gets colorValue 0', () {
      final tokens = [_token('SOLO', volume: 1000)];
      final result = packer.pack(tokens, width: 400, height: 400);
      expect(result.first.colorValue, closeTo(0.0, 0.01));
    });
  });
}
