import 'composite_index_data.dart';

/// Data model for the `GET /api/market/sectors` response (PR-098).
class SectorRotationData {
  final String timeframe;
  final int marketSymbolCount;
  final List<SectorIndexData> sectors;

  const SectorRotationData({
    required this.timeframe,
    required this.marketSymbolCount,
    required this.sectors,
  });

  factory SectorRotationData.fromJson(Map<String, dynamic> json) {
    final raw = json['sectors'] as List<dynamic>? ?? [];
    return SectorRotationData(
      timeframe: json['timeframe'] as String,
      marketSymbolCount: json['marketSymbolCount'] as int? ?? 0,
      sectors: raw
          .map((e) => SectorIndexData.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}

/// One sector's composite index and relative strength vs the market
/// (COMMON.md "Sector composites (PR-098)"). `points`/`return`/`rs` share the
/// clamped market-overlap window when `rsAvailable`; otherwise `points` is
/// the sector's own (unclamped) series and `rs` is omitted.
class SectorIndexData {
  final String id;
  final String name;
  final int symbolCount;
  final List<IndexPoint> points;
  final double returnValue;
  final double? rs;
  final bool rsAvailable;

  const SectorIndexData({
    required this.id,
    required this.name,
    required this.symbolCount,
    required this.points,
    required this.returnValue,
    this.rs,
    required this.rsAvailable,
  });

  factory SectorIndexData.fromJson(Map<String, dynamic> json) {
    final rawPoints = json['points'] as List<dynamic>? ?? [];
    return SectorIndexData(
      id: json['id'] as String,
      name: json['name'] as String,
      symbolCount: json['symbolCount'] as int? ?? 0,
      points: rawPoints
          .map((e) => IndexPoint.fromJson(e as Map<String, dynamic>))
          .toList(),
      returnValue: (json['return'] as num?)?.toDouble() ?? 0.0,
      rs: (json['rs'] as num?)?.toDouble(),
      rsAvailable: json['rsAvailable'] as bool? ?? false,
    );
  }
}
