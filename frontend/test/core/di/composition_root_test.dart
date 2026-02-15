import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:pano_chart_frontend/core/di/composition_root.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/overview/overview_view_model.dart';

class _FakeClient extends http.BaseClient {
  http.Request? lastRequest;
  final http.Response Function(http.Request) handler;

  _FakeClient(this.handler);

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final req = http.Request(request.method, request.url);
    lastRequest = req;
    final res = handler(req);
    final stream = Stream.fromIterable([res.bodyBytes]);
    return http.StreamedResponse(stream, res.statusCode, headers: res.headers);
  }
}

void main() {
  test('CompositionRoot_wiresGetCandleSeries', () async {
    http.Request? captured;
    final fake = _FakeClient((req) {
      captured = req;
      return http.Response(
          '{"symbol":"BTCUSDT","timeframe":"1m","candles":[]}', 200,
          headers: {'content-type': 'application/json'});
    });

    final root =
        CompositionRoot(apiBaseUrl: 'https://api.example', httpClient: fake);
    final usecase = root.createGetCandleSeries();

    final input = GetCandleSeriesInput(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2));
    await usecase.execute(input);

    expect(captured, isNotNull);
    expect(captured!.url.path, '/api/v1/candles');
  });

  test('CompositionRoot_usesHttpCandleApi', () async {
    final fake = _FakeClient((req) => http.Response(
        '{"symbol":"BTCUSDT","timeframe":"1m","candles":[]}', 200,
        headers: {'content-type': 'application/json'}));
    final root =
        CompositionRoot(apiBaseUrl: 'https://api.example', httpClient: fake);
    final usecase = root.createGetCandleSeries();

    final input = GetCandleSeriesInput(
        symbol: 'BTCUSDT',
        timeframe: '1m',
        from: DateTime.utc(2024, 1, 1),
        to: DateTime.utc(2024, 1, 2));
    await usecase.execute(input);

    // If HttpCandleApi was used the fake client will have seen a request with the expected path.
    expect(fake.lastRequest, isNotNull);
    expect(fake.lastRequest!.url.path, '/api/v1/candles');
  });

  test('CompositionRoot_wiresOverviewViewModel', () async {
    http.Request? captured;
    final fake = _FakeClient((req) {
      captured = req;
      return http.Response(
        jsonEncode({
          'timeframe': '1h',
          'count': 1,
          'precision': 30,
          'results': [
            {
              'symbol': 'BTCUSDT',
              'totalScore': 2.75,
              'sparkline': [42000.0, 42100.0],
            },
          ],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final root =
        CompositionRoot(apiBaseUrl: 'https://api.example', httpClient: fake);
    final vm = root.createOverviewViewModel();

    expect(vm, isA<OverviewViewModel>());

    // Exercise the full chain: vm → GetOverviewImpl → HttpOverviewApi → fake
    await vm.loadInitial('1h');

    expect(captured, isNotNull);
    expect(captured!.url.path, '/api/overview');
    expect(vm.state.items.length, 1);
    expect(vm.state.items[0].symbol, 'BTCUSDT');
  });
}
