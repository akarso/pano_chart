import '../../features/candles/api/candle_response.dart';

/// Abstract interface for symbol scoring heuristics.
abstract class SymbolScoreCalculator {
  /// Name of the scoring heuristic.
  String get name;

  /// Returns a normalized score for the given series.
  /// Throws [ArgumentError] for invalid input.
  double score(CandleSeriesResponse series);

  /// Returns a plain-language explanation for the score.
  String explain(CandleSeriesResponse series);
}
