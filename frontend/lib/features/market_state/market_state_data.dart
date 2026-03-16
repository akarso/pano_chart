/// Data model for the market state API response.
class MarketStateData {
  final String timeframe;
  final String state;
  final double confidence;
  final MarketBreadth breadth;
  final int symbolCount;

  const MarketStateData({
    required this.timeframe,
    required this.state,
    required this.confidence,
    required this.breadth,
    required this.symbolCount,
  });

  factory MarketStateData.fromJson(Map<String, dynamic> json) {
    return MarketStateData(
      timeframe: json['timeframe'] as String,
      state: json['state'] as String,
      confidence: (json['confidence'] as num).toDouble(),
      breadth:
          MarketBreadth.fromJson(json['breadth'] as Map<String, dynamic>),
      symbolCount: json['symbolCount'] as int,
    );
  }
}

/// Breadth breakdown per market regime.
class MarketBreadth {
  final double sideways;
  final double compression;
  final double breakout;
  final double trend;

  const MarketBreadth({
    required this.sideways,
    required this.compression,
    required this.breakout,
    required this.trend,
  });

  factory MarketBreadth.fromJson(Map<String, dynamic> json) {
    return MarketBreadth(
      sideways: (json['sideways'] as num).toDouble(),
      compression: (json['compression'] as num).toDouble(),
      breakout: (json['breakout'] as num).toDouble(),
      trend: (json['trend'] as num).toDouble(),
    );
  }
}
