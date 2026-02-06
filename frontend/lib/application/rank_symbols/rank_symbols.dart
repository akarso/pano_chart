import '../../domain/symbol.dart';
import '../../domain/candle_series.dart';
import '../../domain/scoring/symbol_score_calculator.dart';
import 'ranked_symbol.dart';

/// Weight configuration for a score calculator.
class ScoreWeight {
  final SymbolScoreCalculator calculator;
  final double weight;
  const ScoreWeight({required this.calculator, required this.weight});
}

/// Interface for symbol ranking use case.
abstract class RankSymbols {
  List<RankedSymbol> rank(Map<AppSymbol, CandleSeries> seriesBySymbol);
}

/// Default implementation: deterministic, weighted, stable.
class RankSymbolsImpl implements RankSymbols {
  final List<ScoreWeight> scoreWeights;
  const RankSymbolsImpl({required this.scoreWeights});

  @override
  List<RankedSymbol> rank(Map<AppSymbol, CandleSeries> seriesBySymbol) {
    final List<RankedSymbol> ranked = [];
    for (final entry in seriesBySymbol.entries) {
      final symbol = entry.key;
      final series = entry.value;
      final Map<String, double> scores = {};
      double total = 0.0;
      for (final sw in scoreWeights) {
        if (sw.weight == 0) continue;
        final score = sw.calculator.scoreSeries(series);
        scores[sw.calculator.name] = score;
        total += score * sw.weight;
      }
      ranked
          .add(RankedSymbol(symbol: symbol, scores: scores, totalScore: total));
    }
    ranked.sort((a, b) {
      final cmp = b.totalScore.compareTo(a.totalScore);
      if (cmp != 0) return cmp;
      return a.symbol.value.compareTo(b.symbol.value);
    });
    return ranked;
  }
}
