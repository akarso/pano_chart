import '../../domain/candle_series.dart';
import 'dart:math' as math;
import '../../features/candles/api/candle_response.dart';
import 'symbol_score_calculator.dart';

/// Trend Predictability score: slope_normalized * R^2
class TrendPredictabilityScoreCalculator implements SymbolScoreCalculator {
  @override
  double scoreSeries(CandleSeries series) {
    return score(CandleSeriesResponse(
        symbol: '', timeframe: '', candles: series.candles));
  }

  @override
  String get name => 'Trend Predictability';

  @override
  double score(CandleSeriesResponse series) {
    final closes = series.candles.map((c) => c.close).toList();
    if (closes.length < 2) {
      throw ArgumentError('At least 2 candles required');
    }
    final n = closes.length;
    final x = List.generate(n, (i) => i.toDouble());
    final meanX = x.reduce((a, b) => a + b) / n;
    final meanY = closes.reduce((a, b) => a + b) / n;
    double num = 0, den = 0;
    for (var i = 0; i < n; i++) {
      num += (x[i] - meanX) * (closes[i] - meanY);
      den += math.pow(x[i] - meanX, 2);
    }
    final slope = den == 0 ? 0.0 : num / den;
    // Normalize slope by dividing by mean close (avoid division by zero)
    final slopeNorm = meanY == 0 ? 0.0 : slope / meanY;
    // Compute R^2
    double ssTot = 0, ssRes = 0;
    for (var i = 0; i < n; i++) {
      final fit = meanY + slope * (x[i] - meanX);
      ssTot += math.pow(closes[i] - meanY, 2);
      ssRes += math.pow(closes[i] - fit, 2);
    }
    final r2 = ssTot == 0 ? 0.0 : 1 - (ssRes / ssTot);
    return slopeNorm * r2;
  }

  @override
  String explain(CandleSeriesResponse series) {
    try {
      final s = score(series);
      if (s > 0) {
        return 'Uptrend, predictability: ${s.toStringAsFixed(3)}';
      } else if (s < 0) {
        return 'Downtrend, predictability: ${s.abs().toStringAsFixed(3)}';
      } else {
        return 'No trend.';
      }
    } catch (e) {
      return 'Not enough data to score.';
    }
  }
}
