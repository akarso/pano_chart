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
          // Allow 0.5px tolerance for floating point.
          expect(dist, greaterThanOrEqualTo(minDist * minDist - 1.0),
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
          expect(dist, greaterThanOrEqualTo(minDist * minDist - 1.0),
              reason:
                  '${a.token.symbol} overlaps ${b.token.symbol}');
        }
      }
    });
  });
}
