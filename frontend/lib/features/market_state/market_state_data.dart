import 'data_quality.dart';
import 'participation_counts.dart';

/// Data model for the market state API response.
class MarketStateData {
  final String timeframe;
  final String state;
  final double confidence;
  final MarketBreadth breadth;
  final int symbolCount;
  final String bias;
  final double effectiveTrend;
  final double breakdownRate;
  final String label;
  final String dataQuality;
  /// Backend regime source (`composite_*` vs `participation`).
  final String regimeSource;
  final int windowBars;
  final double trendScore;
  final ParticipationCounts participation;

  const MarketStateData({
    required this.timeframe,
    required this.state,
    required this.confidence,
    required this.breadth,
    required this.symbolCount,
    this.bias = 'neutral',
    this.effectiveTrend = 0,
    this.breakdownRate = 0,
    this.label = '',
    this.dataQuality = 'ok',
    this.regimeSource = '',
    this.windowBars = 0,
    this.trendScore = 0,
    this.participation = ParticipationCounts.empty,
  });

  /// Whether this reading reflects a real market read, as opposed to a
  /// full evaluation-source outage — see PR-074. Without this check, an
  /// outage looks identical to a genuinely quiet market.
  bool get isDataUnavailable => isDataQualityUnavailable(dataQuality);

  /// Human label for [confidence] — tape vs participation share.
  String get confidenceLabel => regimeConfidenceLabel(regimeSource);

  factory MarketStateData.fromJson(Map<String, dynamic> json) {
    final symbolCount = json['symbolCount'] as int;
    // A response with no dataQuality field at all is a legacy (pre-PR-074)
    // response — most just mean "ok", but symbolCount == 0 is exactly the
    // shape the old empty-evaluations branch always returned, i.e. a real
    // outage. Treat that specific legacy shape as unavailable rather than
    // defaulting to ok; an explicit dataQuality value is never overridden.
    final dataQuality = json['dataQuality'] as String? ??
        (symbolCount == 0 ? dataQualityUnavailable : 'ok');
    return MarketStateData(
      timeframe: json['timeframe'] as String,
      state: json['state'] as String,
      confidence: (json['confidence'] as num).toDouble(),
      breadth:
          MarketBreadth.fromJson(
              json['breadth'] as Map<String, dynamic>), // glossary-ok
      symbolCount: symbolCount,
      bias: json['bias'] as String? ?? 'neutral',
      effectiveTrend: (json['effectiveTrend'] as num?)?.toDouble() ?? 0,
      breakdownRate: (json['breakdownRate'] as num?)?.toDouble() ?? 0,
      label: json['label'] as String? ?? '',
      dataQuality: dataQuality,
      regimeSource: json['regimeSource'] as String? ?? '',
      windowBars: (json['windowBars'] as num?)?.toInt() ?? 0,
      trendScore: (json['trendScore'] as num?)?.toDouble() ?? 0,
      participation: ParticipationCounts.fromJson(json['participation']),
    );
  }
}

/// Label for the confidence percentage given a backend [regimeSource].
String regimeConfidenceLabel(String regimeSource) {
  if (regimeSource.startsWith('composite')) {
    return 'tape confidence';
  }
  if (regimeSource == 'participation' || regimeSource.isEmpty) {
    // Empty: legacy clients / fallback card without source — do not claim tape.
    return 'participation share';
  }
  return 'confidence';
}

/// Breadth breakdown per market regime.
class MarketBreadth {
  final double sideways;
  final double compression;
  final double expansion;
  final double trend;

  const MarketBreadth({
    required this.sideways,
    required this.compression,
    required this.expansion,
    required this.trend,
  });

  factory MarketBreadth.fromJson(Map<String, dynamic> json) {
    return MarketBreadth(
      sideways: (json['sideways'] as num).toDouble(),
      compression: (json['compression'] as num).toDouble(),
      expansion: (json['expansion'] as num).toDouble(),
      trend: (json['trend'] as num).toDouble(),
    );
  }
}
