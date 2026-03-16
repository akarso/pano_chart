/// Data model for the market composite index API response.
class CompositeIndexData {
  final String timeframe;
  final List<IndexPoint> points;
  final int symbolCount;

  const CompositeIndexData({
    required this.timeframe,
    required this.points,
    required this.symbolCount,
  });

  factory CompositeIndexData.fromJson(Map<String, dynamic> json) {
    final rawPoints = json['points'] as List<dynamic>? ?? [];
    return CompositeIndexData(
      timeframe: json['timeframe'] as String,
      symbolCount: json['symbolCount'] as int,
      points: rawPoints
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
