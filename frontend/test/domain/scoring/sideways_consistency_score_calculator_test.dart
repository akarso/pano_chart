import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/domain/scoring/sideways_consistency_score_calculator.dart';

void main() {
  final calc = SidewaysConsistencyScoreCalculator();
  group('SidewaysConsistencyScoreCalculator', () {
    test('returns 1 for perfectly flat', () {
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
          CandleDto(
              timestamp: DateTime(2024, 1, 2),
              open: 1,
              high: 1,
              low: 1,
              close: 1,
              volume: 1),
        ],
      );
      expect(calc.score(series), closeTo(1.0, 1e-6));
      expect(calc.explain(series), contains('Highly consistent'));
    });
    test('returns <1 for deviation', () {
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
          CandleDto(
              timestamp: DateTime(2024, 1, 2),
              open: 1,
              high: 2,
              low: 1,
              close: 2,
              volume: 1),
        ],
      );
      expect(calc.score(series), lessThan(1.0));
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
