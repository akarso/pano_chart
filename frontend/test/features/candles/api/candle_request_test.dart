import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_request.dart';

void main() {
  test('CandleRequest_requiresAllFields', () {
    final now = DateTime.utc(2024, 1, 1);
    final later = now.add(const Duration(minutes: 1));

    // valid construction should not throw
    expect(() => CandleRequest(symbol: 'BTCUSDT', timeframe: '1m', from: now, to: later), returnsNormally);

    // empty symbol
    expect(() => CandleRequest(symbol: '  ', timeframe: '1m', from: now, to: later), throwsArgumentError);

    // empty timeframe
    expect(() => CandleRequest(symbol: 'BTCUSDT', timeframe: '', from: now, to: later), throwsArgumentError);

    // non-UTC times should throw
    final localFrom = DateTime(2024, 1, 1);
    expect(() => CandleRequest(symbol: 'BTCUSDT', timeframe: '1m', from: localFrom, to: later), throwsArgumentError);

    // from must be before to
    expect(() => CandleRequest(symbol: 'BTCUSDT', timeframe: '1m', from: later, to: now), throwsArgumentError);
  });
}
