import 'dart:math' as math;

import 'sector_rotation_data.dart';

/// One rendered row of the "Sector rotation" card: a sector's RS bar,
/// normalized against the widest available bar so the strongest mover
/// always fills the row. Kept out of the widget per FRONTEND.md.
class SectorRotationRow {
  final String id;
  final String name;
  final int symbolCount;
  final double? rs;
  final bool rsAvailable;

  /// Bar fill fraction (0..1), magnitude relative to the widest RS bar.
  /// 0 when `rsAvailable` is false (no bar drawn).
  final double barFraction;

  const SectorRotationRow({
    required this.id,
    required this.name,
    required this.symbolCount,
    required this.rs,
    required this.rsAvailable,
    required this.barFraction,
  });
}

/// Builds display rows for the sector rotation card. Sectors already arrive
/// RS-available-first sorted by `rs` desc, then unavailable (COMMON.md
/// "Sector composites (PR-098)") — rows keep that backend order.
List<SectorRotationRow> buildSectorRotationRows(SectorRotationData data) {
  var maxAbs = 0.0;
  for (final s in data.sectors) {
    if (s.rsAvailable && s.rs != null) {
      maxAbs = math.max(maxAbs, s.rs!.abs());
    }
  }
  return data.sectors.map((s) {
    final frac = (s.rsAvailable && s.rs != null && maxAbs > 0)
        ? (s.rs!.abs() / maxAbs).clamp(0.0, 1.0)
        : 0.0;
    return SectorRotationRow(
      id: s.id,
      name: s.name,
      symbolCount: s.symbolCount,
      rs: s.rs,
      rsAvailable: s.rsAvailable,
      barFraction: frac,
    );
  }).toList();
}

/// `+2.1%` / `-1.4%` label (`rs × 100`, one decimal); `—` when unavailable.
String sectorRsLabel(SectorRotationRow row) {
  final value = row.rs;
  if (!row.rsAvailable || value == null) return '—';
  final pct = value * 100;
  final rounded = pct.toStringAsFixed(1);
  final isZero = rounded == '0.0' || rounded == '-0.0';
  return isZero ? '0.0%' : '${pct > 0 ? '+' : ''}$rounded%';
}
