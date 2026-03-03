import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/fear_greed/http_fear_greed_api.dart';

void main() {
  group('HttpFearGreedApi', () {
    test('fetch parses valid response', () async {
      final mockClient = MockClient((request) async {
        expect(request.url.path, '/api/v1/fear-greed');
        return http.Response(
          jsonEncode({
            'value': 14,
            'valueClassification': 'Extreme Fear',
            'timestampUtc': '2026-03-01T00:00:00Z',
          }),
          200,
        );
      });

      final api = HttpFearGreedApi(
        client: mockClient,
        baseUrl: 'http://localhost:8080',
      );
      final data = await api.fetch();

      expect(data.value, 14);
      expect(data.classification, 'Extreme Fear');
    });

    test('fetch throws on non-200 status', () async {
      final mockClient = MockClient((request) async {
        return http.Response('error', 502);
      });

      final api = HttpFearGreedApi(
        client: mockClient,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpFearGreedApiException>()),
      );
    });
  });
}
