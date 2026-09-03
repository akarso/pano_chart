import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:pano_chart_frontend/features/overview/http_overview_api.dart';

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
  test('HttpOverviewApi_buildsCorrectRequest', () async {
    http.Request? captured;
    final fakeClient = _FakeClient((req) {
      captured = req;
      return http.Response(
        jsonEncode({
          'timeframe': '1h',
          'count': 0,
          'precision': 30,
          'results': [],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final api = HttpOverviewApi(
      baseUrl: 'https://api.example',
      client: fakeClient,
    );

    await api.fetchOverview(timeframe: '1h', limit: 30);

    expect(captured, isNotNull);
    expect(captured!.url.toString(),
        'https://api.example/api/overview?timeframe=1h&limit=30');
    expect(captured!.method, 'GET');
  });

  test('HttpOverviewApi_parsesSuccessfulResponse', () async {
    final fakeClient = _FakeClient((_) {
      return http.Response(
        jsonEncode({
          'timeframe': '4h',
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

    final api = HttpOverviewApi(
      baseUrl: 'https://api.example',
      client: fakeClient,
    );

    final dto = await api.fetchOverview(timeframe: '4h', limit: 30);

    expect(dto.timeframe, '4h');
    expect(dto.count, 1);
    expect(dto.results.length, 1);
    expect(dto.results[0].symbol, 'BTCUSDT');
    expect(dto.results[0].totalScore, 2.75);
  });

  test('HttpOverviewApi_throwsOnNon200', () async {
    final fakeClient = _FakeClient((_) {
      return http.Response('Internal Server Error', 500);
    });

    final api = HttpOverviewApi(
      baseUrl: 'https://api.example',
      client: fakeClient,
    );

    expect(
      () => api.fetchOverview(timeframe: '1h', limit: 30),
      throwsA(isA<HttpOverviewApiException>()),
    );
  });

  test('HttpOverviewApi_throwsOn404', () async {
    final fakeClient = _FakeClient((_) {
      return http.Response('Not Found', 404);
    });

    final api = HttpOverviewApi(
      baseUrl: 'https://api.example',
      client: fakeClient,
    );

    try {
      await api.fetchOverview(timeframe: '1h', limit: 30);
      fail('Expected HttpOverviewApiException');
    } on HttpOverviewApiException catch (e) {
      expect(e.statusCode, 404);
      expect(e.message, contains('404'));
    }
  });
}
