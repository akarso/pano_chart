import 'package:flutter/widgets.dart';
import '../../domain/series_view_mode.dart';
import '../candles/api/candle_response.dart';

/// Abstraction for rendering a time series as a chart.
///
/// Implementations must support both [SeriesViewMode.line] and [SeriesViewMode.candles].
///
/// - Overview screen: [SeriesViewMode.line] (default)
/// - Detail/deep-dive: [SeriesViewMode.candles] (default)
abstract class SeriesChartRenderer {
  Widget build(
    BuildContext context, {
    required CandleSeriesResponse series,
    required SeriesViewMode viewMode,
  });
}
