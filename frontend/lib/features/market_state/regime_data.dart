/// Data model for the market regime API response.
class RegimeData {
  final String timeframe;
  final String regime;
  final double confidence;
  final RegimeMetrics metrics;

  const RegimeData({
    required this.timeframe,
    required this.regime,
    required this.confidence,
    required this.metrics,
  });

  factory RegimeData.fromJson(Map<String, dynamic> json) {
    return RegimeData(
      timeframe: json['timeframe'] as String,
      regime: json['regime'] as String,
      confidence: (json['confidence'] as num).toDouble(),
      metrics:
          RegimeMetrics.fromJson(json['metrics'] as Map<String, dynamic>),
    );
  }
}

/// Computed market-level metrics used for regime detection.
class RegimeMetrics {
  final double trendBreadth;
  final double compressionBreadth;
  final double volatilityExpansion;
  final double dispersion;

  const RegimeMetrics({
    required this.trendBreadth,
    required this.compressionBreadth,
    required this.volatilityExpansion,
    required this.dispersion,
  });

  factory RegimeMetrics.fromJson(Map<String, dynamic> json) {
    return RegimeMetrics(
      trendBreadth: (json['trendBreadth'] as num).toDouble(),
      compressionBreadth: (json['compressionBreadth'] as num).toDouble(),
      volatilityExpansion: (json['volatilityExpansion'] as num).toDouble(),
      dispersion: (json['dispersion'] as num).toDouble(),
    );
  }
}
