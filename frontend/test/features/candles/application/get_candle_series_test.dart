import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_api.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_request.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

class _FakeCandleApi implements CandleApi {
  CandleRequest? lastRequest;
  CandleSeriesResponse? toReturn;
  Exception? toThrow;

  @override
  Future<CandleSeriesResponse> fetchCandles(CandleRequest request) async {
    lastRequest = request;
    if (toThrow != null) throw toThrow!;
    return toReturn!;
  }
}

void main() {
  test('GetCandleSeries_delegatesToApi', () async {
    final fake = _FakeCandleApi();
    fake.toReturn =
        CandleSeriesResponse(symbol: 'BTCUSDT', timeframe: '1m', candles: []);
    final usecase = GetCandleSeriesImpl(fake);

    final input = GetCandleSeriesInput(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2));
    await usecase.execute(input);

    expect(fake.lastRequest, isNotNull);
    expect(fake.lastRequest!.symbol, 'BTCUSDT');
    expect(fake.lastRequest!.timeframe, '1m');
    expect(fake.lastRequest!.from, input.from);
    expect(fake.lastRequest!.to, input.to);
  });

  test('GetCandleSeries_returnsApiResponse', () async {
    final fake = _FakeCandleApi();
    final resp =
        CandleSeriesResponse(symbol: 'BTCUSDT', timeframe: '1m', candles: [
      CandleDto(
          timestamp: DateTime.utc(2024, 1, 1),
          open: 1,
          high: 2,
          low: 0.5,
          close: 1.5,
          volume: 10)
    ]);
    fake.toReturn = resp;
    final usecase = GetCandleSeriesImpl(fake);

    final input = GetCandleSeriesInput(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2));
    final out = await usecase.execute(input);

    expect(out, same(resp));
  });

  test('GetCandleSeries_propagatesFailure', () async {
    final fake = _FakeCandleApi();
    fake.toThrow = Exception('boom');
    final usecase = GetCandleSeriesImpl(fake);

    final input = GetCandleSeriesInput(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2));

    expect(() => usecase.execute(input), throwsA(isA<Exception>()));
  });
}
