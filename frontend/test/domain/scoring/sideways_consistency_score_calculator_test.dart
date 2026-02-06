import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/domain/scoring/sideways_consistency_score_calculator.dart';

void main() {
  final calc = SidewaysConsistencyScoreCalculator();
  group('SidewaysConsistencyScoreCalculator', () {
    test('perfect oscillation in fixed band → score > 0.7', () {
      // Zig-zag: 1,2,1,2,1,2,1,2,1,1 (p0 == pn)
      final prices = [1.0, 2.0, 1.0, 2.0, 1.0, 2.0, 1.0, 2.0, 1.0, 1.0];
      final candles = List.generate(
          prices.length,
          (i) => CandleDto(
              timestamp: DateTime(2024, 1, 1, 0, i),
              open: prices[i],
              high: prices[i],
              low: prices[i],
              close: prices[i],
              volume: 1));
      final series = CandleSeriesResponse(
          symbol: 'ZIG', timeframe: '1h', candles: candles);
      final score = calc.score(series);
      expect(score, greaterThan(0.7));
    });

    test('flat price → score == 0', () {
      final prices = List.filled(10, 1.0);
      final candles = List.generate(
          prices.length,
          (i) => CandleDto(
              timestamp: DateTime(2024, 1, 1, 0, i),
              open: 1,
              high: 1,
              low: 1,
              close: 1,
              volume: 1));
      final series = CandleSeriesResponse(
          symbol: 'FLAT', timeframe: '1h', candles: candles);
      expect(calc.score(series), closeTo(0.0, 1e-6));
    });

    test('linear trend → score < 0.2', () {
      // 1,2,3,4,5,6,7,8,9,10
      final prices = List.generate(10, (i) => (i + 1).toDouble());
      final candles = List.generate(
          prices.length,
          (i) => CandleDto(
              timestamp: DateTime(2024, 1, 1, 0, i),
              open: prices[i],
              high: prices[i],
              low: prices[i],
              close: prices[i],
              volume: 1));
      final series =
          CandleSeriesResponse(symbol: 'UP', timeframe: '1h', candles: candles);
      expect(calc.score(series), lessThan(0.2));
    });

    test('expanding volatility → score < stable case', () {
      // Stable zig-zag, p0 == pn
      final stable = [1.0, 2.0, 1.0, 2.0, 1.0, 2.0, 1.0, 2.0, 1.0, 1.0];
      final stableCandles = List.generate(
          stable.length,
          (i) => CandleDto(
              timestamp: DateTime(2024, 1, 1, 0, i),
              open: stable[i],
              high: stable[i],
              low: stable[i],
              close: stable[i],
              volume: 1));
      final stableSeries = CandleSeriesResponse(
          symbol: 'STABLE', timeframe: '1h', candles: stableCandles);
      final stableScore = calc.score(stableSeries);

      // Expanding zig-zag: amplitude grows over time
      final expanding = [1.0, 1.5, 1.0, 2.0, 1.0, 2.5, 1.0, 3.0, 1.0, 1.0];
      final expandingCandles = List.generate(
          expanding.length,
          (i) => CandleDto(
              timestamp: DateTime(2024, 1, 1, 0, i),
              open: expanding[i],
              high: expanding[i],
              low: expanding[i],
              close: expanding[i],
              volume: 1));
      final expandingSeries = CandleSeriesResponse(
          symbol: 'EXPAND', timeframe: '1h', candles: expandingCandles);
      final expandingScore = calc.score(expandingSeries);

      expect(expandingScore, lessThan(stableScore));
    });

    test('throws on insufficient data', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          CandleDto(
              timestamp: DateTime(2024),
              open: 1,
              high: 1,
              low: 1,
              close: 1,
              volume: 1),
        ],
      );
      expect(() => calc.score(series), throwsArgumentError);
    });
  });
}
