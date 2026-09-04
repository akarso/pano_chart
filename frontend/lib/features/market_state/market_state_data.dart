/// Data model for the market state API response.
class MarketStateData {
  final String timeframe;
  final String state;
  final double confidence;
  final MarketBreadth breadth;
  final int symbolCount;
  final String bias;
  final double effectiveTrend;
  final double breakdownRate;
  final String label;
  final String dataQuality;

  const MarketStateData({
    required this.timeframe,
    required this.state,
    required this.confidence,
    required this.breadth,
    required this.symbolCount,
    this.bias = 'neutral',
    this.effectiveTrend = 0,
    this.breakdownRate = 0,
    this.label = '',
    this.dataQuality = 'ok',
  });

  /// Whether this reading reflects a real market read, as opposed to a
  /// full evaluation-source outage — see PR-074. Without this check, an
  /// outage looks identical to a genuinely quiet market.
  bool get isDataUnavailable => dataQuality == 'unavailable';

  factory MarketStateData.fromJson(Map<String, dynamic> json) {
    return MarketStateData(
      timeframe: json['timeframe'] as String,
      state: json['state'] as String,
      confidence: (json['confidence'] as num).toDouble(),
      breadth:
          MarketBreadth.fromJson(json['breadth'] as Map<String, dynamic>),
      symbolCount: json['symbolCount'] as int,
      bias: json['bias'] as String? ?? 'neutral',
      effectiveTrend: (json['effectiveTrend'] as num?)?.toDouble() ?? 0,
      breakdownRate: (json['breakdownRate'] as num?)?.toDouble() ?? 0,
      label: json['label'] as String? ?? '',
      dataQuality: json['dataQuality'] as String? ?? 'ok',
    );
  }
}

/// Breadth breakdown per market regime.
class MarketBreadth {
  final double sideways;
  final double compression;
  final double expansion;
  final double trend;

  const MarketBreadth({
    required this.sideways,
    required this.compression,
    required this.expansion,
    required this.trend,
  });

  factory MarketBreadth.fromJson(Map<String, dynamic> json) {
    return MarketBreadth(
      sideways: (json['sideways'] as num).toDouble(),
      compression: (json['compression'] as num).toDouble(),
      expansion: (json['expansion'] as num).toDouble(),
      trend: (json['trend'] as num).toDouble(),
    );
  }
}
