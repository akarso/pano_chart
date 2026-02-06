import '../../domain/candle_series.dart';
import 'dart:math' as math;
import '../../features/candles/api/candle_response.dart';
import 'symbol_score_calculator.dart';

/// Sideways Consistency score: 1 - (max deviation from mean / mean)
class SidewaysConsistencyScoreCalculator implements SymbolScoreCalculator {
  static const int _rangeWindowSize = 5;

  @override
  double scoreSeries(CandleSeries series) {
    return score(CandleSeriesResponse(
        symbol: '', timeframe: '', candles: series.candles));
  }

  @override
  String get name => 'Sideways Consistency';

  @override
  double score(CandleSeriesResponse series) {
    final closes = series.candles.map((c) => c.close).toList();
    final n = closes.length;
    if (n < _rangeWindowSize + 1) {
      throw ArgumentError('At least ${_rangeWindowSize + 1} candles required');
    }
    // 1. Net Displacement Ratio (NDR)
    final p0 = closes.first;
    final pn = closes.last;
    final minPrice = closes.reduce(math.min);
    final maxPrice = closes.reduce(math.max);
    final range = maxPrice - minPrice;
    double ndr = 0.0;
    if (range > 0) {
      ndr = ((pn - p0).abs()) / range;
    }
    ndr = ndr.clamp(0.0, 1.0);

    // 2. Range Stability Score (RSS)
    final windowRanges = <double>[];
    for (int i = 0; i <= n - _rangeWindowSize; i++) {
      final window = closes.sublist(i, i + _rangeWindowSize);
      final wMin = window.reduce(math.min);
      final wMax = window.reduce(math.max);
      windowRanges.add(wMax - wMin);
    }
    final meanRange =
        windowRanges.reduce((a, b) => a + b) / windowRanges.length;
    double stddevRange = 0.0;
    if (windowRanges.length > 1) {
      final mean = meanRange;
      stddevRange = math.sqrt(windowRanges
              .map((r) => math.pow(r - mean, 2))
              .reduce((a, b) => a + b) /
          (windowRanges.length));
    }
    double rss = 1.0;
    if (meanRange > 0) {
      rss = 1 - (stddevRange / meanRange);
    }
    rss = rss.clamp(0.0, 1.0);

    // 3. Oscillation Density Score (ODS)
    int extremaCount = 0;
    for (int i = 1; i < closes.length - 1; i++) {
      if ((closes[i] > closes[i - 1] && closes[i] > closes[i + 1]) ||
          (closes[i] < closes[i - 1] && closes[i] < closes[i + 1])) {
        extremaCount++;
      }
    }
    double ods = 0.0;
    if (n > 2) {
      ods = extremaCount / (n - 2);
    }
    ods = ods.clamp(0.0, 1.0);

    // Final sideways score
    double sidewaysScore = (1 - ndr) * rss * ods;
    sidewaysScore = sidewaysScore.clamp(0.0, 1.0);
    return sidewaysScore;
  }

  @override
  String explain(CandleSeriesResponse series) {
    try {
      final s = score(series);
      if (s > 0.7) {
        return 'Highly consistent sideways action.';
      } else if (s > 0.2) {
        return 'Some sideways action.';
      } else {
        return 'Not consistent sideways.';
      }
    } catch (e) {
      return 'Not enough data to score.';
    }
  }
}
