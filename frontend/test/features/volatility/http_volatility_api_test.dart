import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/volatility/http_volatility_api.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';

void main() {
  group('HttpVolatilityApi', () {
    test('fetch parses buckets from API response', () async {
      final client = MockClient((req) async {
        expect(req.url.path, '/api/volatility');
        expect(req.url.queryParameters['timeframe'], '1m');
        return http.Response(
          jsonEncode({
            'timeframe': '1m',
            'buckets': [
              {'minute': 0, 'avg_move': 0.01, 'spike_prob': 0.1, 'normalized': 0.8},
              {'minute': 60, 'avg_move': 0.02, 'spike_prob': 0.5, 'normalized': 1.3},
            ],
          }),
          200,
        );
      });

      final api = HttpVolatilityApi(client: client, baseUrl: 'http://test');
      final result = await api.fetch();

      expect(result.length, 2);
      expect(result[0].minute, 0);
      expect(result[0].normalized, 0.8);
      expect(result[0].spikeProb, 0.1);
      expect(result[1].minute, 60);
      expect(result[1].normalized, 1.3);
    });

    test('fetch passes timeframe parameter', () async {
      String? capturedTf;
      final client = MockClient((req) async {
        capturedTf = req.url.queryParameters['timeframe'];
        return http.Response(
          jsonEncode({'timeframe': '5m', 'buckets': []}),
          200,
        );
      });

      final api = HttpVolatilityApi(client: client, baseUrl: 'http://test');
      await api.fetch(timeframe: '5m');

      expect(capturedTf, '5m');
    });

    test('fetch throws on non-200 response', () async {
      final client = MockClient((_) async => http.Response('error', 503));
      final api = HttpVolatilityApi(client: client, baseUrl: 'http://test');

      expect(
        () => api.fetch(),
        throwsA(isA<HttpVolatilityApiException>()),
      );
    });

    test('fetch returns empty list when no buckets', () async {
      final client = MockClient((_) async {
        return http.Response(
          jsonEncode({'timeframe': '1m', 'buckets': []}),
          200,
        );
      });

      final api = HttpVolatilityApi(client: client, baseUrl: 'http://test');
      final result = await api.fetch();
      expect(result, isEmpty);
    });
  });
}
