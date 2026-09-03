/// Data model for the position crowding / fragility API response.
class FragilityData {
  final String symbol;
  final String timeframe;
  final double fragilityScore;
  final String riskLevel;
  final String dominantSide;
  final String squeezeRisk;
  final FragilityComponents components;

  const FragilityData({
    required this.symbol,
    required this.timeframe,
    required this.fragilityScore,
    required this.riskLevel,
    required this.dominantSide,
    required this.squeezeRisk,
    required this.components,
  });

  factory FragilityData.fromJson(Map<String, dynamic> json) {
    final comps = json['components'] as Map<String, dynamic>? ?? {};
    return FragilityData(
      symbol: json['symbol'] as String,
      timeframe: json['timeframe'] as String,
      fragilityScore: (json['fragilityScore'] as num).toDouble(),
      riskLevel: json['riskLevel'] as String,
      dominantSide: json['dominantSide'] as String? ?? 'neutral',
      squeezeRisk: json['squeezeRisk'] as String? ?? 'none',
      components: FragilityComponents.fromJson(comps),
    );
  }

  /// Human-readable label for the risk level.
  static String riskLabel(String level) {
    switch (level) {
      case 'high':
        return 'High Risk';
      case 'medium':
        return 'Medium Risk';
      case 'low':
        return 'Low Risk';
      default:
        return level;
    }
  }

  /// Human-readable label for the squeeze risk.
  static String squeezeLabel(String squeeze) {
    switch (squeeze) {
      case 'long_squeeze':
        return 'Long Squeeze';
      case 'short_squeeze':
        return 'Short Squeeze';
      default:
        return 'None';
    }
  }

  /// Human-readable label for the dominant side.
  static String sideLabel(String side) {
    switch (side) {
      case 'long':
        return 'Crowded Long';
      case 'short':
        return 'Crowded Short';
      default:
        return 'Neutral';
    }
  }
}

/// Individual sub-scores making up the fragility composite.
class FragilityComponents {
  final double fundingExtremeness;
  final double oiExpansion;
  final double longShortImbalance;
  final double liquidationProximity;

  const FragilityComponents({
    required this.fundingExtremeness,
    required this.oiExpansion,
    required this.longShortImbalance,
    required this.liquidationProximity,
  });

  factory FragilityComponents.fromJson(Map<String, dynamic> json) {
    return FragilityComponents(
      fundingExtremeness: (json['fundingExtremeness'] as num? ?? 0).toDouble(),
      oiExpansion: (json['oiExpansion'] as num? ?? 0).toDouble(),
      longShortImbalance: (json['longShortImbalance'] as num? ?? 0).toDouble(),
      liquidationProximity:
          (json['liquidationProximity'] as num? ?? 0).toDouble(),
    );
  }

  /// Human-readable display name for a component key.
  static String displayName(String key) {
    switch (key) {
      case 'fundingExtremeness':
        return 'Funding';
      case 'oiExpansion':
        return 'OI Expansion';
      case 'longShortImbalance':
        return 'L/S Imbalance';
      case 'liquidationProximity':
        return 'Liq. Proximity';
      default:
        return key;
    }
  }
}
