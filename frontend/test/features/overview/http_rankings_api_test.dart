import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart' as http_testing;
import 'package:pano_chart_frontend/features/overview/http_rankings_api.dart';

void main() {
  test('HttpRankingsApi_buildsCorrectRequest', () async {
    Uri? captured;
    final client = http_testing.MockClient((req) async {
      captured = req.url;
      return http.Response(
        jsonEncode({
          'timeframe': '1h',
          'sort': 'total',
          'page': 2,
          'pageSize': 20,
          'totalItems': 50,
          'totalPages': 3,
          'precision': 30,
          'results': [],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final api = HttpRankingsApi(client: client, baseUrl: 'https://api.test');
    await api.fetchRankings(
        timeframe: '1h', sort: 'total', page: 2, pageSize: 20);

    expect(captured, isNotNull);
    expect(captured!.path, '/api/rankings');
    expect(captured!.queryParameters['timeframe'], '1h');
    expect(captured!.queryParameters['sort'], 'total');
    expect(captured!.queryParameters['page'], '2');
    expect(captured!.queryParameters['pageSize'], '20');
  });

  test('HttpRankingsApi_parsesSuccessfulResponse', () async {
    final client = http_testing.MockClient((_) async {
      return http.Response(
        jsonEncode({
          'timeframe': '4h',
          'sort': 'gain',
          'page': 1,
          'pageSize': 30,
          'totalItems': 1,
          'totalPages': 1,
          'precision': 30,
          'results': [
            {
              'symbol': 'BTCUSDT',
              'totalScore': 2.75,
              'scores': {'Trend Predictability': 1.0, 'Sideways Consistency': 0.5, 'Gain/Loss': 1.25},
              'volume': 5000.0,
              'sparkline': [42000.0, 42100.0],
            },
          ],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final api = HttpRankingsApi(client: client, baseUrl: 'https://api.test');
    final dto = await api.fetchRankings(
        timeframe: '4h', sort: 'gain', page: 1, pageSize: 30);

    expect(dto.timeframe, '4h');
    expect(dto.sort, 'gain');
    expect(dto.totalItems, 1);
    expect(dto.precision, 30);
    expect(dto.results.length, 1);
    expect(dto.results[0].symbol, 'BTCUSDT');
    expect(dto.results[0].scores.trend, 1.0);
    expect(dto.results[0].volume, 5000.0);
  });

  test('HttpRankingsApi_throwsOnNon200', () async {
    final client = http_testing.MockClient((_) async {
      return http.Response('{"error":"bad request"}', 400);
    });

    final api = HttpRankingsApi(client: client, baseUrl: 'https://api.test');

    expect(
      () => api.fetchRankings(
          timeframe: '1h', sort: 'total', page: 1, pageSize: 30),
      throwsA(isA<HttpRankingsApiException>()),
    );
  });
}
