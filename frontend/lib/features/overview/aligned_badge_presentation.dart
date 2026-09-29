/// Tooltip text for the aligned-badge border, e.g. "Aligned: trend (100%)".
///
/// The `?mtf=1` rankings overlay reports only the alignment ratio, not raw
/// frame counts (COMMON.md "Multi-timeframe regime stack (PR-099)"). A
/// fixed "N/4" phrasing would be ambiguous whenever a symbol has fewer than
/// 4 fresh frames — e.g. ratio 1.0 could mean 1/1, 2/2, 3/3, or 4/4, and
/// `round(ratio * 4)` would silently report the wrong denominator. The
/// percentage is always correct regardless of how many frames contributed.
String alignedTooltip(String alignedState, double alignment) {
  final pct = (alignment * 100).round();
  return 'Aligned: $alignedState ($pct%)';
}
