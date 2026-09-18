/// Data model for the market composite index API response.
class CompositeIndexData {
  final String timeframe;
  /// Equal-weight median path (outlier-resistant).
  final List<IndexPoint> points;
  /// Quote-volume-weighted mean path (money-flow weighted). Empty when absent.
  final List<IndexPoint> volumeWeightedPoints;
  final int symbolCount;

  const CompositeIndexData({
    required this.timeframe,
    required this.points,
    this.volumeWeightedPoints = const [],
    required this.symbolCount,
  });

  /// Preferred chart series: volume-weighted when available, else median.
  List<IndexPoint> get preferredPoints =>
      volumeWeightedPoints.length >= 2 ? volumeWeightedPoints : points;

  bool get hasVolumeWeighted => volumeWeightedPoints.length >= 2;

  factory CompositeIndexData.fromJson(Map<String, dynamic> json) {
    final rawPoints = json['points'] as List<dynamic>? ?? [];
    final rawVw = json['volumeWeightedPoints'] as List<dynamic>? ?? [];
    return CompositeIndexData(
      timeframe: json['timeframe'] as String,
      symbolCount: json['symbolCount'] as int,
      points: rawPoints
          .map((e) => IndexPoint.fromJson(e as Map<String, dynamic>))
          .toList(),
      volumeWeightedPoints: rawVw
          .map((e) => IndexPoint.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}

/// A single data point in the composite index time series.
class IndexPoint {
  final int timestamp;
  final double value;

  const IndexPoint({required this.timestamp, required this.value});

  factory IndexPoint.fromJson(Map<String, dynamic> json) {
    return IndexPoint(
      timestamp: (json['t'] as num).toInt(),
      value: (json['v'] as num).toDouble(),
    );
  }
}
