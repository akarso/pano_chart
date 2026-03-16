/// Data returned by the `/api/sol/price` endpoint.
class SolPriceInfo {
  final double solPrice;
  final double requiredSOL;
  final double priceUSD;
  final String wallet;

  const SolPriceInfo({
    required this.solPrice,
    required this.requiredSOL,
    required this.priceUSD,
    required this.wallet,
  });

  factory SolPriceInfo.fromJson(Map<String, dynamic> json) {
    return SolPriceInfo(
      solPrice: (json['sol_price'] as num).toDouble(),
      requiredSOL: (json['required_sol'] as num).toDouble(),
      priceUSD: (json['price_usd'] as num).toDouble(),
      wallet: json['wallet'] as String,
    );
  }
}
