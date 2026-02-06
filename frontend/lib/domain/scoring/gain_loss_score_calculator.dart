import '../../domain/candle_series.dart';
import '../../features/candles/api/candle_response.dart';
import 'symbol_score_calculator.dart';

/// Gain/Loss score: (lastClose - firstClose) / firstClose
class GainLossScoreCalculator implements SymbolScoreCalculator {
  @override
  double scoreSeries(CandleSeries series) {
    return score(CandleSeriesResponse(
        symbol: '', timeframe: '', candles: series.candles));
  }

  @override
  String get name => 'Gain/Loss';

  @override
  double score(CandleSeriesResponse series) {
    final candles = series.candles;
    if (candles.length < 2) {
      throw ArgumentError('At least 2 candles required');
    }
    final first = candles.first.close;
    final last = candles.last.close;
    if (first == 0) {
      throw ArgumentError('First close price cannot be zero');
    }
    return (last - first) / first;
  }

  @override
  String explain(CandleSeriesResponse series) {
    final candles = series.candles;
    if (candles.length < 2) return 'Not enough data to score.';
    final first = candles.first.close;
    final last = candles.last.close;
    final pct = ((last - first) / first) * 100;
    if (pct > 0) {
      return 'Gainer: +${pct.toStringAsFixed(2)}%';
    } else if (pct < 0) {
      return 'Loser: ${pct.toStringAsFixed(2)}%';
    } else {
      return 'Unchanged.';
    }
  }
}
