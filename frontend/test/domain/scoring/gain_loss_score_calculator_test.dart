import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/domain/scoring/gain_loss_score_calculator.dart';

void main() {
  final calc = GainLossScoreCalculator();
  group('GainLossScoreCalculator', () {
    test('returns correct score for gainer', () {
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
      expect(calc.score(series), closeTo(1.0, 1e-6));
      expect(calc.explain(series), contains('Gainer'));
    });
    test('returns correct score for loser', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          CandleDto(
              timestamp: DateTime(2024),
              open: 2,
              high: 2,
              low: 1,
              close: 2,
              volume: 1),
          CandleDto(
              timestamp: DateTime(2024, 1, 2),
              open: 2,
              high: 2,
              low: 1,
              close: 1,
              volume: 1),
        ],
      );
      expect(calc.score(series), closeTo(-0.5, 1e-6));
      expect(calc.explain(series), contains('Loser'));
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
    test('throws on zero first close', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          CandleDto(
              timestamp: DateTime(2024),
              open: 0,
              high: 0,
              low: 0,
              close: 0,
              volume: 1),
          CandleDto(
              timestamp: DateTime(2024, 1, 2),
              open: 0,
              high: 0,
              low: 0,
              close: 1,
              volume: 1),
        ],
      );
      expect(() => calc.score(series), throwsArgumentError);
    });
  });
}
