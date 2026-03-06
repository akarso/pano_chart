import 'package:flutter/services.dart' show rootBundle;
import 'package:yaml/yaml.dart';

/// Set of stablecoin trading pair symbols (e.g. "USDCUSDT") that should
/// be hidden from the overview grid by default.
///
/// Built by appending "USDT" to each base symbol listed in the asset.
class StablecoinConfig {
  final Set<String> symbols;

  const StablecoinConfig(this.symbols);

  int get count => symbols.length;

  bool isStablecoin(String symbol) => symbols.contains(symbol);
}

/// Hardcoded fallback used when the YAML asset cannot be loaded.
const _kFallbackBases = [
  'USDC', 'USD1', 'FDUSD', 'DAI', 'TUSD', 'BUSD',
  'USDP', 'EUR', 'GBP', 'AEUR', 'USTC', 'PYUSD',
];

/// Loads the stablecoin list from [assets/stablecoins.yaml].
///
/// Falls back to a hardcoded list on error.
Future<StablecoinConfig> loadStablecoinConfig() async {
  try {
    final yamlString =
        await rootBundle.loadString('assets/stablecoins.yaml');
    return parseStablecoinConfig(yamlString);
  } catch (_) {
    return StablecoinConfig(
      _kFallbackBases.map((b) => '${b}USDT').toSet(),
    );
  }
}

/// Parses a stablecoin config from a YAML string.  Exposed for testing.
StablecoinConfig parseStablecoinConfig(String yamlString) {
  final doc = loadYaml(yamlString);
  final raw = (doc['ignore'] as String?) ?? '';
  final bases = raw
      .split(',')
      .map((s) => s.trim())
      .where((s) => s.isNotEmpty)
      .toList();
  return StablecoinConfig(bases.map((b) => '${b}USDT').toSet());
}
