import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/infrastructure/stablecoin_config.dart';

void main() {
  group('StablecoinConfig', () {
    test('isStablecoin returns true for listed symbols', () {
      final config = StablecoinConfig({'USDCUSDT', 'DAIUSDT'});
      expect(config.isStablecoin('USDCUSDT'), isTrue);
      expect(config.isStablecoin('DAIUSDT'), isTrue);
      expect(config.isStablecoin('BTCUSDT'), isFalse);
    });

    test('count returns number of stablecoin symbols', () {
      final config = StablecoinConfig({'USDCUSDT', 'DAIUSDT', 'BUSDUSDT'});
      expect(config.count, 3);
    });

    test('empty config matches nothing', () {
      const config = StablecoinConfig({});
      expect(config.count, 0);
      expect(config.isStablecoin('USDCUSDT'), isFalse);
    });
  });

  group('parseStablecoinConfig', () {
    test('parses YAML ignore list and appends USDT', () {
      const yaml = '"ignore": USDC, DAI, BUSD';
      final config = parseStablecoinConfig(yaml);
      expect(config.count, 3);
      expect(config.isStablecoin('USDCUSDT'), isTrue);
      expect(config.isStablecoin('DAIUSDT'), isTrue);
      expect(config.isStablecoin('BUSDUSDT'), isTrue);
      expect(config.isStablecoin('BTCUSDT'), isFalse);
    });

    test('handles extra spaces and trailing commas', () {
      const yaml = '"ignore": " USDC , DAI , "';
      final config = parseStablecoinConfig(yaml);
      // Trimmed entries with content
      expect(config.isStablecoin('USDCUSDT'), isTrue);
      expect(config.isStablecoin('DAIUSDT'), isTrue);
    });

    test('returns empty set when ignore key is missing', () {
      const yaml = 'other: stuff';
      final config = parseStablecoinConfig(yaml);
      expect(config.count, 0);
    });

    test('parses real stablecoins.yaml format', () {
      const yaml =
          '"ignore": USDC, USD1, FDUSD, DAI, TUSD, BUSD, USDP, EUR, GBP, AEUR, USTC, PYUSD';
      final config = parseStablecoinConfig(yaml);
      expect(config.count, 12);
      expect(config.isStablecoin('USDCUSDT'), isTrue);
      expect(config.isStablecoin('FDUSDUSDT'), isTrue);
      expect(config.isStablecoin('EURUSDT'), isTrue);
      expect(config.isStablecoin('PYUSDUSDT'), isTrue);
      expect(config.isStablecoin('BTCUSDT'), isFalse);
    });
  });
}
