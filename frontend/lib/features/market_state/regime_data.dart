/// Data model for the market regime API response.
class RegimeData {
  final String timeframe;
  final String regime;
  final double prevalence;
  final String bias;
  final RegimeScores scores;
  final RegimeMetrics metrics;
  final double effectiveTrend;
  final double breakdownRate;
  final String label;

  const RegimeData({
    required this.timeframe,
    required this.regime,
    required this.prevalence,
    this.bias = 'neutral',
    required this.scores,
    required this.metrics,
    this.effectiveTrend = 0.0,
    this.breakdownRate = 0.0,
    this.label = '',
  });

  factory RegimeData.fromJson(Map<String, dynamic> json) {
    return RegimeData(
      timeframe: json['timeframe'] as String,
      regime: json['regime'] as String,
      prevalence: (json['prevalence'] as num).toDouble(),
      bias: json['bias'] as String? ?? 'neutral',
      scores:
          RegimeScores.fromJson(json['scores'] as Map<String, dynamic>),
      metrics:
          RegimeMetrics.fromJson(json['metrics'] as Map<String, dynamic>),
      effectiveTrend: (json['effectiveTrend'] as num?)?.toDouble() ?? 0.0,
      breakdownRate: (json['breakdownRate'] as num?)?.toDouble() ?? 0.0,
      label: json['label'] as String? ?? '',
    );
  }
}

/// Soft regime scores that sum to ~100%.
class RegimeScores {
  final double expansion;
  final double compression;
  final double trend;
  final double sideways;

  const RegimeScores({
    required this.expansion,
    required this.compression,
    required this.trend,
    required this.sideways,
  });

  factory RegimeScores.fromJson(Map<String, dynamic> json) {
    return RegimeScores(
      expansion: (json['expansion'] as num).toDouble(),
      compression: (json['compression'] as num).toDouble(),
      trend: (json['trend'] as num).toDouble(),
      sideways: (json['sideways'] as num).toDouble(),
    );
  }
}

/// Computed market-level metrics used for regime detection.
class RegimeMetrics {
  final double trendBreadth;
  final double sidewaysBreadth;
  final double expansionBreadth;
  final double compressionBreadth;
  final double volatilityExpansion;
  final double dispersion;

  const RegimeMetrics({
    required this.trendBreadth,
    required this.sidewaysBreadth,
    required this.expansionBreadth,
    required this.compressionBreadth,
    required this.volatilityExpansion,
    required this.dispersion,
  });

  factory RegimeMetrics.fromJson(Map<String, dynamic> json) {
    return RegimeMetrics(
      trendBreadth: (json['trendBreadth'] as num).toDouble(),
      sidewaysBreadth: (json['sidewaysBreadth'] as num?)?.toDouble() ?? 0.0,
      expansionBreadth: (json['expansionBreadth'] as num?)?.toDouble() ?? 0.0,
      compressionBreadth: (json['compressionBreadth'] as num).toDouble(),
      volatilityExpansion: (json['volatilityExpansion'] as num).toDouble(),
      dispersion: (json['dispersion'] as num).toDouble(),
    );
  }
}
