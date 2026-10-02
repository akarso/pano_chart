/// Data model for `GET /api/symbol/{symbol}/plan` (PR-110 / PR-111).
class PlanData {
  final String symbol;
  final String timeframe;
  final double low;
  final double high;
  final double mid;
  final double atr;
  final double price;
  final double longEntry;
  final double longStop;
  final double longTarget;
  final double longTargetFull;
  final double shortEntry;
  final double shortStop;
  final double shortTarget;
  final double shortTargetFull;
  final double riskReward;
  final double rangeQuality;
  final double position;
  final bool valid;
  final String reason;
  final double size;
  final double shortSize;

  const PlanData({
    required this.symbol,
    required this.timeframe,
    required this.low,
    required this.high,
    required this.mid,
    required this.atr,
    required this.price,
    required this.longEntry,
    required this.longStop,
    required this.longTarget,
    required this.longTargetFull,
    required this.shortEntry,
    required this.shortStop,
    required this.shortTarget,
    required this.shortTargetFull,
    required this.riskReward,
    required this.rangeQuality,
    required this.position,
    required this.valid,
    this.reason = '',
    this.size = 0,
    this.shortSize = 0,
  });

  factory PlanData.fromJson(Map<String, dynamic> json) {
    double n(String key) => (json[key] as num?)?.toDouble() ?? 0.0;
    return PlanData(
      symbol: json['symbol'] as String? ?? '',
      timeframe: json['timeframe'] as String? ?? '',
      low: n('low'),
      high: n('high'),
      mid: n('mid'),
      atr: n('atr'),
      price: n('price'),
      longEntry: n('longEntry'),
      longStop: n('longStop'),
      longTarget: n('longTarget'),
      longTargetFull: n('longTargetFull'),
      shortEntry: n('shortEntry'),
      shortStop: n('shortStop'),
      shortTarget: n('shortTarget'),
      shortTargetFull: n('shortTargetFull'),
      riskReward: n('riskReward'),
      rangeQuality: n('rangeQuality'),
      position: n('position'),
      valid: json['valid'] as bool? ?? false,
      reason: json['reason'] as String? ?? '',
      size: n('size'),
      shortSize: n('shortSize'),
    );
  }

  /// Local size for [risk] USDT: `risk / |entry − stop|`. Returns 0 when
  /// invalid or non-finite — matches backend `plan.Size`.
  static double sizeFor(double risk, double entry, double stop) {
    final dist = (entry - stop).abs();
    if (dist <= 0 || !risk.isFinite || risk <= 0) return 0;
    final out = risk / dist;
    return out.isFinite ? out : 0;
  }
}

/// Chart overlay levels. When [valid] is false, only channel diagnostics
/// may be present — painters must not draw trade ticks.
class PlanChartLevels {
  final double low;
  final double mid;
  final double high;
  final bool valid;
  final double? entry;
  final double? stop;
  final double? target;

  const PlanChartLevels({
    required this.low,
    required this.mid,
    required this.high,
    required this.valid,
    this.entry,
    this.stop,
    this.target,
  });

  /// Absorbs channel (and trade) prices into a visible Y-range so overlays
  /// are not clipped when the candle window is tighter than the plan.
  void expandPriceRange(void Function(double price) absorb) {
    absorb(low);
    absorb(mid);
    absorb(high);
    if (!valid) return;
    if (entry != null) absorb(entry!);
    if (stop != null) absorb(stop!);
    if (target != null) absorb(target!);
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is PlanChartLevels &&
          low == other.low &&
          mid == other.mid &&
          high == other.high &&
          valid == other.valid &&
          entry == other.entry &&
          stop == other.stop &&
          target == other.target;

  @override
  int get hashCode => Object.hash(low, mid, high, valid, entry, stop, target);
}
