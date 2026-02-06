import '../../domain/candle_series.dart';
import 'dart:math' as math;
import '../../features/candles/api/candle_response.dart';
import 'symbol_score_calculator.dart';

/// Sideways Consistency score: 1 - (max deviation from mean / mean)
class SidewaysConsistencyScoreCalculator implements SymbolScoreCalculator {
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
    if (closes.length < 2) {
      throw ArgumentError('At least 2 candles required');
    }
    final mean = closes.reduce((a, b) => a + b) / closes.length;
    if (mean == 0) return 0.0;
    final maxDev = closes.map((c) => (c - mean).abs()).reduce(math.max);
    // Score is 1 for perfectly flat, approaches 0 as deviation increases
    final score = 1 - (maxDev / mean).clamp(0.0, 1.0);
    return score;
  }

  @override
  String explain(CandleSeriesResponse series) {
    try {
      final s = score(series);
      if (s > 0.8) {
        return 'Highly consistent sideways action.';
      } else if (s > 0.5) {
        return 'Moderate sideways action.';
      } else {
        return 'Not consistent sideways.';
      }
    } catch (e) {
      return 'Not enough data to score.';
    }
  }
}
