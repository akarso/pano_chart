import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:pano_chart_frontend/features/candles/infrastructure/http_candle_api.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_request.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

class _FakeHttpClient extends http.BaseClient {
  http.Request? lastRequest;
  final http.Response Function(http.Request request) handler;

  _FakeHttpClient(this.handler);

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    // convert BaseRequest to Request to access body and method
    final req = http.Request(request.method, request.url);
    lastRequest = req;
    final res = handler(req);
    final stream = Stream.fromIterable([res.bodyBytes]);
    return http.StreamedResponse(stream, res.statusCode,
        headers: res.headers, reasonPhrase: res.reasonPhrase);
  }
}

void main() {
  test('HttpCandleApi_buildsCorrectRequest', () async {
    http.Request? captured;
    final fake = _FakeHttpClient((req) {
      captured = req;
      return http.Response('{}', 200,
          headers: {'content-type': 'application/json'});
    });

    final api = HttpCandleApi(baseUrl: 'https://api.example', client: fake);
    final from = DateTime.utc(2024, 1, 1);
    final to = DateTime.utc(2024, 1, 2);
    final request =
        CandleRequest(symbol: 'BTCUSDT', timeframe: '1m', from: from, to: to);

    await api.fetchCandles(request);

    expect(captured, isNotNull);
    final uri = captured!.url;
    expect(uri.path, '/api/v1/candles');
    expect(uri.queryParameters['symbol'], 'BTCUSDT');
    expect(uri.queryParameters['timeframe'], '1m');
    expect(uri.queryParameters['from'], from.toUtc().toIso8601String());
    expect(uri.queryParameters['to'], to.toUtc().toIso8601String());
  });

  test('HttpCandleApi_parsesSuccessfulResponse', () async {
    final sample = jsonEncode({
      'symbol': 'BTCUSDT',
      'timeframe': '1m',
      'candles': [
        {
          'timestamp': '2024-01-01T00:00:00Z',
          'open': 42000.0,
          'high': 42100.0,
          'low': 41950.0,
          'close': 42050.0,
          'volume': 123.45
        }
      ]
    });

    final fake = _FakeHttpClient((req) => http.Response(sample, 200,
        headers: {'content-type': 'application/json'}));
    final api = HttpCandleApi(baseUrl: 'https://api.example', client: fake);
    final out = await api.fetchCandles(CandleRequest(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2)));

    expect(out.symbol, 'BTCUSDT');
    expect(out.timeframe, '1m');
    expect(out.candles.length, 1);
    expect(out.candles.first.open, 42000.0);
  });

  test('HttpCandleApi_handles400', () async {
    final fake = _FakeHttpClient((req) => http.Response('bad', 400));
    final api = HttpCandleApi(baseUrl: 'https://api.example', client: fake);

    expect(
        () => api.fetchCandles(CandleRequest(
            symbol: 'BTCUSDT',
            timeframe: '1m',
            from: DateTime.utc(2024, 1, 1),
            to: DateTime.utc(2024, 1, 2))),
        throwsA(isA<HttpCandleApiException>()));
  });

  test('HttpCandleApi_handles500', () async {
    final fake = _FakeHttpClient((req) => http.Response('err', 500));
    final api = HttpCandleApi(baseUrl: 'https://api.example', client: fake);

    expect(
        () => api.fetchCandles(CandleRequest(
            symbol: 'BTCUSDT',
            timeframe: '1m',
            from: DateTime.utc(2024, 1, 1),
            to: DateTime.utc(2024, 1, 2))),
        throwsA(isA<HttpCandleApiException>()));
  });
}
