/// Data model for the retail behavior API response.
class BehaviorData {
  final String symbol;
  final String timeframe;
  final double greed;
  final double fear;
  final double patience;
  final double panic;
  final String summary;

  const BehaviorData({
    required this.symbol,
    required this.timeframe,
    required this.greed,
    required this.fear,
    required this.patience,
    required this.panic,
    required this.summary,
  });

  factory BehaviorData.fromJson(Map<String, dynamic> json) {
    return BehaviorData(
      symbol: json['symbol'] as String,
      timeframe: json['timeframe'] as String,
      greed: (json['greed'] as num).toDouble(),
      fear: (json['fear'] as num).toDouble(),
      patience: (json['patience'] as num).toDouble(),
      panic: (json['panic'] as num).toDouble(),
      summary: json['summary'] as String,
    );
  }

  /// All four dimensions as a map for iteration in the UI.
  Map<String, double> get dimensions => {
        'greed': greed,
        'fear': fear,
        'patience': patience,
        'panic': panic,
      };

  /// Human-readable label for a dimension key.
  static String dimensionLabel(String key) {
    switch (key) {
      case 'greed':
        return 'Greed';
      case 'fear':
        return 'Fear';
      case 'patience':
        return 'Patience';
      case 'panic':
        return 'Panic';
      default:
        return key;
    }
  }
}
