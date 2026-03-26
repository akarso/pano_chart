/// Data model for the setup quality scoring API response.
class SetupData {
  final String symbol;
  final String timeframe;
  final String bestSetup;
  final double score;
  final Map<String, double> scores;
  final double trendHealth;
  final String regime;

  const SetupData({
    required this.symbol,
    required this.timeframe,
    required this.bestSetup,
    required this.score,
    required this.scores,
    required this.trendHealth,
    required this.regime,
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
    );
  }

  /// Human-readable health label based on trend health value.
  String get healthLabel {
    if (trendHealth > 0.8) return 'Healthy';
    if (trendHealth > 0.6) return 'OK';
    if (trendHealth > 0.4) return 'Weak \u2193';
    return 'Breaking \u2193\u2193';
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
