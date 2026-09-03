/// A single intraday volatility bucket as produced by the backend.
class VolatilityBucket {
  /// Minute-of-day (0–1439).
  final int minute;

  /// Normalized activity relative to daily average (1.0 = average).
  final double normalized;

  /// Probability of a spike move (0.0–1.0).
  final double spikeProb;

  const VolatilityBucket({
    required this.minute,
    required this.normalized,
    required this.spikeProb,
  });

  factory VolatilityBucket.fromJson(Map<String, dynamic> json) {
    return VolatilityBucket(
      minute: json['minute'] as int,
      normalized: (json['normalized'] as num).toDouble(),
      spikeProb: (json['spike_prob'] as num).toDouble(),
    );
  }
}
