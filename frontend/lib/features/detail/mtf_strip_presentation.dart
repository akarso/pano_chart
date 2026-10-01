import 'mtf_regimes_data.dart';

/// Fixed pill order for the MTF strip (PR-100) — matches the backend's
/// timeframe stack (`application/evaluation.DefaultTimeframes`).
const kMtfStripTimeframes = ['15m', '1h', '4h', '1d'];

/// [value] when it is one of [kMtfStripTimeframes], otherwise `'1h'`.
///
/// Shared by notification settings and the prefs setter so an unknown
/// watchlist timeframe cannot be stored or shown in the dropdown.
String acceptedWatchlistTimeframe(String? value) {
  if (value != null && kMtfStripTimeframes.contains(value)) return value;
  return '1h';
}

/// One rendered pill of the MTF strip. [dominant] is null when the backend
/// omitted that timeframe (missing/stale/algo-mismatched frame) — the pill
/// still renders, just as a neutral placeholder rather than disappearing,
/// so the strip always shows all four slots.
class MtfPill {
  final String timeframe;
  final String? dominant;
  final String bias;

  const MtfPill({
    required this.timeframe,
    this.dominant,
    this.bias = 'neutral',
  });
}

/// Builds the four strip pills from [data] (null when not yet loaded / the
/// fetch failed) in [kMtfStripTimeframes] order.
List<MtfPill> buildMtfPills(MtfRegimesData? data) {
  final byTimeframe = {
    for (final f in data?.frames ?? const <MtfFrame>[]) f.timeframe: f,
  };
  return kMtfStripTimeframes.map((tf) {
    final frame = byTimeframe[tf];
    return MtfPill(
      timeframe: tf,
      dominant: frame?.dominant,
      bias: frame?.bias ?? 'neutral',
    );
  }).toList();
}
