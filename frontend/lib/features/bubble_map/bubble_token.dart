/// Immutable domain model representing a single token in the bubble map.
class BubbleToken {
  final String symbol;
  final double volume;
  final double priceChange;
  final double totalScore;
  final double trendScore;
  final double sidewaysScore;
  final double gainScore;
  final double compressionScore;
  final double breakoutUpScore;
  final double breakoutDownScore;
  final String badgeComponent;

  const BubbleToken({
    required this.symbol,
    required this.volume,
    required this.priceChange,
    this.totalScore = 0.0,
    this.trendScore = 0.0,
    this.sidewaysScore = 0.0,
    this.gainScore = 0.0,
    this.compressionScore = 0.0,
    this.breakoutUpScore = 0.0,
    this.breakoutDownScore = 0.0,
    this.badgeComponent = '',
  });

  /// Computes percentage price change from a sparkline (first→last close).
  ///
  /// Returns 0.0 when the sparkline has fewer than 2 data points or the
  /// first value is zero.
  static double priceChangeFromSparkline(List<double> sparkline) {
    if (sparkline.length < 2) return 0.0;
    final first = sparkline.first;
    if (first == 0) return 0.0;
    return ((sparkline.last - first) / first) * 100.0;
  }
}
