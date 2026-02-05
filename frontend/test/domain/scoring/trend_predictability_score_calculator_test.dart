import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/domain/scoring/trend_predictability_score_calculator.dart';

void main() {
  final calc = TrendPredictabilityScoreCalculator();
  group('TrendPredictabilityScoreCalculator', () {
    test('returns positive for uptrend', () {
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
              open: 2,
              high: 2,
              low: 2,
              close: 2,
              volume: 1),
        ],
      );
      expect(calc.score(series), greaterThan(0));
      expect(calc.explain(series), contains('Uptrend'));
    });
    test('returns negative for downtrend', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          CandleDto(
              timestamp: DateTime(2024),
              open: 2,
              high: 2,
              low: 2,
              close: 2,
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
      expect(calc.score(series), lessThan(0));
      expect(calc.explain(series), contains('Downtrend'));
    });
    test('returns near zero for flat', () {
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
      expect(calc.score(series).abs(), lessThan(1e-6));
      expect(calc.explain(series), contains('No trend'));
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
