import 'dart:math' as math;

import 'composite_chart_painter.dart';
import 'composite_index_data.dart';
import 'http_composite_index_api.dart';

/// Whether the selected chart series is the same path as [regimeSource].
bool seriesMatchesTape({
  required String regimeSource,
  required bool useVolumeWeighted,
  required bool hasVolumeWeighted,
}) {
  final showingVw = useVolumeWeighted && hasVolumeWeighted;
  if (regimeSource == 'composite_volume_weighted') return showingVw;
  if (regimeSource == 'composite_median') return !showingVw;
  return false;
}

/// Headline subline for the scored window (PR-116).
String headlineSubline({
  required String regimeSource,
  required int windowBars,
  required int chartLen,
  required String timeframe,
  required bool matchesTape,
  required bool hasScoredEvidence,
}) {
  if (!isKnownCompositeSource(regimeSource)) {
    return regimeSource == 'participation' || regimeSource.isEmpty
        ? 'From token participation (tape unavailable)'
        : 'From merged market tape (same structure as one chart)';
  }
  if (!matchesTape) {
    return 'Headline scored on the tape; chart shows another series';
  }
  if (windowBars <= 0) {
    return 'From merged market tape (same structure as one chart)';
  }
  if (chartLen <= 0) {
    return 'Scored on $windowBars bars • $timeframe';
  }
  if (!hasScoredEvidence && chartLen > windowBars) {
    return 'Headline scored on a $windowBars-bar tape · '
        'chart shows $chartLen-bar context • $timeframe';
  }
  final shown = math.min(windowBars, chartLen);
  if (windowBars > chartLen) {
    return 'Scored on $windowBars bars; chart shows $chartLen • $timeframe';
  }
  if (windowBars == chartLen) {
    return 'Scored on these $shown bars • $timeframe';
  }
  return 'Scored on the last $shown bars shown • $timeframe';
}

/// Percent change from [scoredStart] to the last point.
double compositePercentChange(List<IndexPoint> chartPoints, int scoredStart) {
  if (chartPoints.length <= 1) return 0.0;
  final start = chartPoints[scoredStart].value;
  if (start == 0.0) return 0.0;
  return (chartPoints.last.value / start - 1.0) * 100.0;
}

/// Appends [tape] as the scored suffix of [context], scaling so the join
/// matches. The tape series keeps its own first-close rebase (same as
/// `CalculateTape`); the prefix stays on the longer context rebase.
List<IndexPoint> mergeContextWithTape(
  List<IndexPoint> context,
  List<IndexPoint> tape,
) {
  if (tape.isEmpty) return context;
  if (context.isEmpty) return List<IndexPoint>.from(tape);
  if (context.length <= tape.length) return List<IndexPoint>.from(tape);

  final start = context.length - tape.length;
  final join = context[start].value;
  final tape0 = tape.first.value;
  final scale = tape0 == 0.0 ? 1.0 : join / tape0;
  return [
    ...context.sublist(0, start),
    for (final p in tape)
      IndexPoint(timestamp: p.timestamp, value: p.value * scale),
  ];
}

List<IndexPoint> _pickSeries(
  CompositeIndexData data, {
  required bool useVolumeWeighted,
}) {
  if (useVolumeWeighted && data.hasVolumeWeighted) {
    return data.volumeWeightedPoints;
  }
  return data.points;
}

/// Immutable chart-card decisions (kept out of the widget per FRONTEND.md).
class CompositeChartPresentation {
  final List<IndexPoint> chartPoints;
  final bool hasPoints;
  final bool isCompositeTape;
  final bool matchesTape;
  final int scoredWin;
  final double change;
  final String changeScope;
  final String seriesLabel;
  final bool showRegression;
  final bool hasScoredEvidence;

  const CompositeChartPresentation({
    required this.chartPoints,
    required this.hasPoints,
    required this.isCompositeTape,
    required this.matchesTape,
    required this.scoredWin,
    required this.change,
    required this.changeScope,
    required this.seriesLabel,
    required this.showRegression,
    required this.hasScoredEvidence,
  });

  /// Resolves what to draw from context (≤200) plus optional tape-window series.
  factory CompositeChartPresentation.resolve({
    required CompositeIndexData context,
    CompositeIndexData? tape,
    required String regimeSource,
    required int windowBars,
    required bool useVolumeWeighted,
  }) {
    final isCompositeTape = isKnownCompositeSource(regimeSource);
    final hasVw = context.hasVolumeWeighted ||
        (tape != null && tape.hasVolumeWeighted);
    final matchesTape = seriesMatchesTape(
      regimeSource: regimeSource,
      useVolumeWeighted: useVolumeWeighted,
      hasVolumeWeighted: hasVw,
    );

    final contextPts = _pickSeries(context, useVolumeWeighted: useVolumeWeighted);
    final tapePts = tape == null
        ? const <IndexPoint>[]
        : _pickSeries(tape, useVolumeWeighted: useVolumeWeighted);

    final hasTapeSeries = tapePts.length >= 2;
    final List<IndexPoint> chartPoints;
    final int scoredWin;
    final bool hasScoredEvidence;

    if (matchesTape && windowBars > 0 && hasTapeSeries) {
      chartPoints = mergeContextWithTape(contextPts, tapePts);
      scoredWin = math.min(windowBars, tapePts.length);
      hasScoredEvidence = scoredWin >= 2;
    } else if (matchesTape &&
        windowBars > 0 &&
        contextPts.length <= windowBars &&
        contextPts.length >= 2) {
      // Chart is already within the tape window (same rebase as a short fetch).
      chartPoints = contextPts;
      scoredWin = math.min(windowBars, contextPts.length);
      hasScoredEvidence = scoredWin >= 2;
    } else {
      chartPoints = contextPts;
      scoredWin = 0;
      hasScoredEvidence = false;
    }

    final scoredStart = scoredWindowStart(chartPoints.length, scoredWin);
    final change = compositePercentChange(chartPoints, scoredStart);
    final changeScope = isCompositeTape && !matchesTape
        ? 'series change'
        : (hasScoredEvidence && scoredWin < chartPoints.length
            ? 'scored-window change'
            : (matchesTape &&
                    chartPoints.length > windowBars &&
                    windowBars > 0 &&
                    !hasScoredEvidence
                ? 'context change'
                : 'window change'));

    return CompositeChartPresentation(
      chartPoints: chartPoints,
      hasPoints: chartPoints.isNotEmpty,
      isCompositeTape: isCompositeTape,
      matchesTape: matchesTape,
      scoredWin: scoredWin,
      change: change,
      changeScope: changeScope,
      seriesLabel: (useVolumeWeighted && hasVw)
          ? 'Volume weighted'
          : 'equal weight (median)',
      showRegression: hasScoredEvidence,
      hasScoredEvidence: hasScoredEvidence,
    );
  }
}
