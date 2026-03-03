import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/detail/trade/exchange_config.dart';

void main() {
  group('ExchangeConfig', () {
    test('buildUrl substitutes {SYMBOL} with uppercase base', () {
      const cfg = ExchangeConfig(
        id: 'binance',
        name: 'Binance',
        urlTemplate: 'https://www.binance.com/trade/{SYMBOL}_USDT',
      );
      final uri = cfg.buildUrl('eth');
      expect(uri.toString(), 'https://www.binance.com/trade/ETH_USDT');
    });

    test('buildUrl handles multiple {SYMBOL} occurrences', () {
      const cfg = ExchangeConfig(
        id: 'test',
        name: 'Test',
        urlTemplate: 'https://ex.com/{SYMBOL}/trade/{SYMBOL}',
      );
      final uri = cfg.buildUrl('sol');
      expect(uri.toString(), 'https://ex.com/SOL/trade/SOL');
    });

    test('fromYaml creates config from map', () {
      final cfg = ExchangeConfig.fromYaml({
        'id': 'mexc',
        'name': 'MEXC',
        'urlTemplate': 'https://www.mexc.com/futures/{SYMBOL}_USDT',
      });
      expect(cfg.id, 'mexc');
      expect(cfg.name, 'MEXC');
      expect(cfg.urlTemplate, contains('{SYMBOL}'));
    });

    test('equality is based on id', () {
      const a = ExchangeConfig(
        id: 'x',
        name: 'Exchange X',
        urlTemplate: 'https://x.com/{SYMBOL}',
      );
      const b = ExchangeConfig(
        id: 'x',
        name: 'Different Name',
        urlTemplate: 'https://y.com/{SYMBOL}',
      );
      expect(a, equals(b));
      expect(a.hashCode, b.hashCode);
    });

    test('inequality for different ids', () {
      const a = ExchangeConfig(
          id: 'a', name: 'A', urlTemplate: 'https://a.com/{SYMBOL}');
      const b = ExchangeConfig(
          id: 'b', name: 'B', urlTemplate: 'https://b.com/{SYMBOL}');
      expect(a, isNot(equals(b)));
    });

    test('toString includes id and name', () {
      const cfg = ExchangeConfig(
          id: 'phemex', name: 'Phemex', urlTemplate: 'https://phemex.com');
      expect(cfg.toString(), contains('phemex'));
      expect(cfg.toString(), contains('Phemex'));
    });
  });

  group('kDefaultExchanges', () {
    test('contains 5 exchanges', () {
      expect(kDefaultExchanges, hasLength(5));
    });

    test('includes binance, mexc, phemex, hyperliquid, bybit', () {
      final ids = kDefaultExchanges.map((e) => e.id).toSet();
      expect(ids, containsAll(['binance', 'mexc', 'phemex', 'hyperliquid', 'bybit']));
    });

    test('all templates contain {SYMBOL}', () {
      for (final cfg in kDefaultExchanges) {
        expect(cfg.urlTemplate, contains('{SYMBOL}'),
            reason: '${cfg.id} template must contain {SYMBOL}');
      }
    });
  });

  group('parseExchangeConfigs', () {
    test('parses valid YAML', () {
      const yaml = '''
exchanges:
  - id: alpha
    name: Alpha
    urlTemplate: "https://alpha.com/{SYMBOL}"
  - id: beta
    name: Beta
    urlTemplate: "https://beta.com/{SYMBOL}/trade"
''';
      final configs = parseExchangeConfigs(yaml);
      expect(configs, hasLength(2));
      expect(configs[0].id, 'alpha');
      expect(configs[1].id, 'beta');
    });

    test('returns empty list for missing exchanges key', () {
      const yaml = 'other_key: true';
      final configs = parseExchangeConfigs(yaml);
      expect(configs, isEmpty);
    });

    test('returns empty list for empty exchanges list', () {
      const yaml = 'exchanges: []';
      final configs = parseExchangeConfigs(yaml);
      expect(configs, isEmpty);
    });
  });

  group('extractBaseSymbol', () {
    test('extracts base from USDT pair', () {
      expect(extractBaseSymbol('BTCUSDT'), 'BTC');
      expect(extractBaseSymbol('ETHUSDT'), 'ETH');
    });

    test('extracts base from USDC pair', () {
      expect(extractBaseSymbol('SOLUSDC'), 'SOL');
    });

    test('extracts base from BUSD pair', () {
      expect(extractBaseSymbol('BNBBUSD'), 'BNB');
    });

    test('extracts base from BTC quote pair', () {
      expect(extractBaseSymbol('ETHBTC'), 'ETH');
    });

    test('extracts base from ETH quote pair', () {
      expect(extractBaseSymbol('LINKETH'), 'LINK');
    });

    test('extracts base from BNB quote pair', () {
      expect(extractBaseSymbol('CAKEBNB'), 'CAKE');
    });

    test('handles lowercase input', () {
      expect(extractBaseSymbol('btcusdt'), 'BTC');
    });

    test('returns uppercased input when no quote matches', () {
      expect(extractBaseSymbol('XYZABC'), 'XYZABC');
    });

    test('does not strip quote from too-short input', () {
      // 'USDT' alone — length equals quote length, no base
      expect(extractBaseSymbol('USDT'), 'USDT');
    });
  });
}
