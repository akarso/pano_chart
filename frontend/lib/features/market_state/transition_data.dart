/// Data model for the market transition probability API response.
class TransitionData {
  final String timeframe;
  final String currentRegime;
  final TransitionProbabilities probabilities;
  final String horizon;

  const TransitionData({
    required this.timeframe,
    required this.currentRegime,
    required this.probabilities,
    required this.horizon,
  });

  factory TransitionData.fromJson(Map<String, dynamic> json) {
    return TransitionData(
      timeframe: json['timeframe'] as String,
      currentRegime: json['currentRegime'] as String,
      probabilities: TransitionProbabilities.fromJson(
          json['probabilities'] as Map<String, dynamic>),
      horizon: json['horizon'] as String,
    );
  }
}

/// Transition probabilities for each target regime.
class TransitionProbabilities {
  final double trend;
  final double sideways;
  final double expansion;

  const TransitionProbabilities({
    required this.trend,
    required this.sideways,
    required this.expansion,
  });

  factory TransitionProbabilities.fromJson(Map<String, dynamic> json) {
    return TransitionProbabilities(
      trend: (json['trend'] as num).toDouble(),
      sideways: (json['sideways'] as num).toDouble(),
      expansion: (json['expansion'] as num).toDouble(),
    );
  }
}
