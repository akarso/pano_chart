/// Data model for the `GET /api/symbol/{symbol}/regimes` response (PR-099/100).
class MtfRegimesData {
  final String symbol;
  final List<MtfFrame> frames;
  final double alignment;
  final String alignedState;

  const MtfRegimesData({
    required this.symbol,
    required this.frames,
    required this.alignment,
    required this.alignedState,
  });

  factory MtfRegimesData.fromJson(Map<String, dynamic> json) {
    final rawFrames = json['frames'] as List<dynamic>? ?? [];
    return MtfRegimesData(
      symbol: json['symbol'] as String,
      frames: rawFrames
          .map((e) => MtfFrame.fromJson(e as Map<String, dynamic>))
          .toList(),
      alignment: (json['alignment'] as num?)?.toDouble() ?? 0.0,
      alignedState: json['alignedState'] as String? ?? 'indecisive',
    );
  }
}

/// One timeframe's dominant regime reading. `structure` (the four-way score
/// mix) isn't parsed — the MTF strip only needs `dominant`/`bias` per pill.
class MtfFrame {
  final String timeframe;
  final String dominant;
  final String bias;
  final double score;

  const MtfFrame({
    required this.timeframe,
    required this.dominant,
    required this.bias,
    required this.score,
  });

  factory MtfFrame.fromJson(Map<String, dynamic> json) {
    return MtfFrame(
      timeframe: json['timeframe'] as String,
      dominant: json['dominant'] as String? ?? 'indecisive',
      bias: json['bias'] as String? ?? 'neutral',
      score: (json['score'] as num?)?.toDouble() ?? 0.0,
    );
  }
}
