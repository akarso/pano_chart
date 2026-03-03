import 'package:flutter/services.dart' show rootBundle;
import 'package:yaml/yaml.dart';

/// Configuration for a single exchange deep link.
///
/// [urlTemplate] contains a `{SYMBOL}` placeholder that is replaced
/// with the base currency at runtime (e.g. "BTC", "ETH").
class ExchangeConfig {
  final String id;
  final String name;
  final String urlTemplate;

  const ExchangeConfig({
    required this.id,
    required this.name,
    required this.urlTemplate,
  });

  /// Build the final URL by substituting {SYMBOL} with [baseSymbol].
  Uri buildUrl(String baseSymbol) {
    final url = urlTemplate.replaceAll('{SYMBOL}', baseSymbol.toUpperCase());
    return Uri.parse(url);
  }

  /// Creates from a YAML map entry.
  factory ExchangeConfig.fromYaml(Map yaml) {
    return ExchangeConfig(
      id: yaml['id'] as String,
      name: yaml['name'] as String,
      urlTemplate: yaml['urlTemplate'] as String,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is ExchangeConfig && id == other.id);

  @override
  int get hashCode => id.hashCode;

  @override
  String toString() => 'ExchangeConfig($id, $name)';
}

/// Hardcoded defaults used when the YAML asset cannot be loaded.
const List<ExchangeConfig> kDefaultExchanges = [
  ExchangeConfig(
    id: 'binance',
    name: 'Binance',
    urlTemplate: 'https://www.binance.com/trade/{SYMBOL}_USDT',
  ),
  ExchangeConfig(
    id: 'mexc',
    name: 'MEXC',
    urlTemplate:
        'https://www.mexc.com/futures/{SYMBOL}_USDT?type=linear_swap',
  ),
  ExchangeConfig(
    id: 'phemex',
    name: 'Phemex',
    urlTemplate: 'https://phemex.com/futures/{SYMBOL}-USDT',
  ),
  ExchangeConfig(
    id: 'hyperliquid',
    name: 'Hyperliquid',
    urlTemplate: 'https://app.hyperliquid.xyz/trade/{SYMBOL}',
  ),
  ExchangeConfig(
    id: 'bybit',
    name: 'Bybit',
    urlTemplate: 'https://www.bybit.com/trade/spot/{SYMBOL}USDT',
  ),
];

/// Loads exchange configurations from the bundled YAML asset.
///
/// Falls back to [kDefaultExchanges] on any error.
Future<List<ExchangeConfig>> loadExchangeConfigs() async {
  try {
    final yamlString =
        await rootBundle.loadString('assets/exchanges.yaml');
    return parseExchangeConfigs(yamlString);
  } catch (_) {
    return kDefaultExchanges;
  }
}

/// Parses exchange configs from a YAML string.  Exposed for testing.
List<ExchangeConfig> parseExchangeConfigs(String yamlString) {
  final doc = loadYaml(yamlString);
  final list = (doc['exchanges'] as YamlList?) ?? [];
  return list.map((e) => ExchangeConfig.fromYaml(e as Map)).toList();
}

/// Extracts the base symbol from an internal pair string.
///
/// E.g. "BTCUSDT" → "BTC", "ETHUSDC" → "ETH".
String extractBaseSymbol(String pair) {
  final s = pair.toUpperCase();
  for (final quote in ['USDT', 'USDC', 'BUSD', 'BTC', 'ETH', 'BNB']) {
    if (s.endsWith(quote) && s.length > quote.length) {
      return s.substring(0, s.length - quote.length);
    }
  }
  return s; // fallback
}
