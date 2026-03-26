/// Data model for the setup quality scoring API response.
class SetupData {
  final String symbol;
  final String timeframe;
  final String bestSetup;
  final double score;
  final Map<String, double> scores;
  final double trendHealth;
  final String regime;
  final double marketEffective;
  final double confidence;
  final double breakoutUp;
  final double breakoutDown;

  const SetupData({
    required this.symbol,
    required this.timeframe,
    required this.bestSetup,
    required this.score,
    required this.scores,
    required this.trendHealth,
    required this.regime,
    required this.marketEffective,
    required this.confidence,
    required this.breakoutUp,
    required this.breakoutDown,
  });

  factory SetupData.fromJson(Map<String, dynamic> json) {
    final rawScores = json['scores'] as Map<String, dynamic>? ?? {};
    return SetupData(
      symbol: json['symbol'] as String,
      timeframe: json['timeframe'] as String,
      bestSetup: json['bestSetup'] as String,
      score: (json['score'] as num).toDouble(),
      scores: rawScores.map((k, v) => MapEntry(k, (v as num).toDouble())),
      trendHealth: (json['trendHealth'] as num?)?.toDouble() ?? 0.0,
      regime: json['regime'] as String? ?? '',
      marketEffective: (json['marketEffective'] as num?)?.toDouble() ?? 0.0,
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
      breakoutUp: (json['breakoutUp'] as num?)?.toDouble() ?? 0.0,
      breakoutDown: (json['breakoutDown'] as num?)?.toDouble() ?? 0.0,
    );
  }

  /// Human-readable health label based on trend health value.
  String get healthLabel {
    if (trendHealth > 0.8) return 'Healthy';
    if (trendHealth > 0.6) return 'OK';
    if (trendHealth > 0.4) return 'Weak \u2193';
    return 'Breaking \u2193\u2193';
  }

  /// Human-readable market context label.
  String get marketLabel {
    if (marketEffective > 0.6) return 'Favorable market';
    if (marketEffective >= 0.4) return 'Neutral market';
    return 'Unfavorable market';
  }

  /// Human-readable confidence label.
  String get confidenceLabel {
    if (confidence > 0.75) return 'High';
    if (confidence > 0.55) return 'Medium';
    return 'Low';
  }

  /// Confidence dot color indicator.
  /// Green (>0.75), Yellow (>0.55), Red (<=0.55).
  String get confidenceDot {
    if (confidence > 0.75) return '\u25CF'; // ● green
    if (confidence > 0.55) return '\u25CF'; // ● yellow
    return '\u25CF'; // ● red
  }

  /// Human-readable display name for a setup type key.
  static String displayName(String setupType) {
    switch (setupType) {
      case 'compression_breakout':
        return 'Compression Breakout';
      case 'trend_continuation':
        return 'Trend Continuation';
      case 'range_reversion':
        return 'Range Reversion';
      default:
        return setupType;
    }
  }
}
