import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

void main() {
  test('CandleSeriesResponse_handlesEmptyList', () {
    final json = {
      'symbol': 'BTCUSDT',
      'timeframe': '1m',
      'candles': <dynamic>[]
    };
    final resp = CandleSeriesResponse.fromJson(json);
    expect(resp.symbol, 'BTCUSDT');
    expect(resp.timeframe, '1m');
    expect(resp.candles, isEmpty);

    // Ensure immutability: attempt to modify should throw
    expect(
        () => resp.candles.add(CandleDto(
            timestamp: DateTime.utc(2024, 1, 1),
            open: 1,
            high: 1,
            low: 1,
            close: 1,
            volume: 1)),
        throwsUnsupportedError);
  });
}
