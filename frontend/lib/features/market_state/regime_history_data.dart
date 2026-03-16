/// Data model for the regime history API response.
class RegimeHistoryData {
  final String timeframe;
  final List<RegimePeriodData> history;
  final int currentAge;

  const RegimeHistoryData({
    required this.timeframe,
    required this.history,
    required this.currentAge,
  });

  factory RegimeHistoryData.fromJson(Map<String, dynamic> json) {
    final historyList = (json['history'] as List<dynamic>?) ?? [];
    return RegimeHistoryData(
      timeframe: json['timeframe'] as String,
      history: historyList
          .map((e) =>
              RegimePeriodData.fromJson(e as Map<String, dynamic>))
          .toList(),
      currentAge: json['currentAge'] as int,
    );
  }
}

/// A single regime period within the history timeline.
class RegimePeriodData {
  final String regime;
  final int start;
  final int? end;
  final int durationCandles;

  const RegimePeriodData({
    required this.regime,
    required this.start,
    this.end,
    required this.durationCandles,
  });

  factory RegimePeriodData.fromJson(Map<String, dynamic> json) {
    return RegimePeriodData(
      regime: json['regime'] as String,
      start: json['start'] as int,
      end: json['end'] as int?,
      durationCandles: json['durationCandles'] as int,
    );
  }
}
