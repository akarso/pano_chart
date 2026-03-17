/// Data model for the setup quality scoring API response.
class SetupData {
  final String symbol;
  final String timeframe;
  final String bestSetup;
  final double score;
  final Map<String, double> scores;

  const SetupData({
    required this.symbol,
    required this.timeframe,
    required this.bestSetup,
    required this.score,
    required this.scores,
  });

  factory SetupData.fromJson(Map<String, dynamic> json) {
    final rawScores = json['scores'] as Map<String, dynamic>? ?? {};
    return SetupData(
      symbol: json['symbol'] as String,
      timeframe: json['timeframe'] as String,
      bestSetup: json['bestSetup'] as String,
      score: (json['score'] as num).toDouble(),
      scores: rawScores.map((k, v) => MapEntry(k, (v as num).toDouble())),
    );
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
