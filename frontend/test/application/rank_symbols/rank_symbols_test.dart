import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/application/rank_symbols/rank_symbols.dart';
// ...existing code...
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/candle_series.dart';
import 'package:pano_chart_frontend/domain/scoring/symbol_score_calculator.dart';

class DummyScoreCalculator implements SymbolScoreCalculator {
  @override
  String get name => 'dummy';
  final double Function(CandleSeries) fn;
  DummyScoreCalculator(this.fn);
  @override
  double score(series) => throw UnimplementedError();
  @override
  String explain(series) => '';
  @override
  double scoreSeries(CandleSeries series) => fn(series);
}

void main() {
  const symbolA = AppSymbol('A');
  const symbolB = AppSymbol('B');
  const symbolC = AppSymbol('C');
  const flat = CandleSeries(candles: []);
  group('RankSymbolsImpl', () {
    test('ranks by weighted sum', () {
      final calc1 = DummyScoreCalculator((_) => 1.0);
      final calc2 = DummyScoreCalculator((_) => 2.0);
      final ranker = RankSymbolsImpl(scoreWeights: [
        ScoreWeight(calculator: calc1, weight: 2.0),
        ScoreWeight(calculator: calc2, weight: 1.0),
      ]);
      final result = ranker.rank({
        symbolA: flat,
        symbolB: flat,
        symbolC: flat,
      });
      expect(result, hasLength(3));
      expect(
          result.every((r) => r.totalScore == 1.0 * 2.0 + 2.0 * 1.0), isTrue);
    });
    test('sorts by totalScore descending, then symbol ascending', () {
      final ranker = RankSymbolsImpl(scoreWeights: [
        ScoreWeight(
            calculator:
                DummyScoreCalculator((s) => s.candles.length.toDouble()),
            weight: 1.0),
      ]);
      CandleDto candle() => CandleDto(
            timestamp: DateTime(2024),
            open: 1,
            high: 1,
            low: 1,
            close: 1,
            volume: 1,
          );
      final result = ranker.rank({
        symbolB: CandleSeries(candles: [candle(), candle(), candle()]),
        symbolA: CandleSeries(candles: [candle(), candle()]),
        symbolC: CandleSeries(candles: [candle()]),
      });
      expect(result[0].symbol.value, 'B');
      expect(result[1].symbol.value, 'A');
      expect(result[2].symbol.value, 'C');
    });
    test('ignores zero-weight calculators', () {
      final ranker = RankSymbolsImpl(scoreWeights: [
        ScoreWeight(calculator: DummyScoreCalculator((_) => 1.0), weight: 0.0),
      ]);
      final result = ranker.rank({symbolA: flat});
      expect(result[0].totalScore, 0.0);
    });
  });
}
