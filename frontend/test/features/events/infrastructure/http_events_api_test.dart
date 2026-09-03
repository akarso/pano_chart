import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart' as http_testing;
import 'package:pano_chart_frontend/features/events/infrastructure/http_events_api.dart';

void main() {
  group('HttpEventsApi', () {
    test('fetchEvents returns events on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/v1/events');
        expect(req.url.queryParameters['date_from'], '2025-03-01');
        expect(req.url.queryParameters['date_to'], '2025-03-07');
        return http.Response(
          jsonEncode({
            'events': [
              {
                'id': 'e1',
                'country': 'US',
                'title': 'NFP',
                'impact': 'high',
                'timestamp': '2025-03-07T13:30:00Z',
              },
            ],
          }),
          200,
        );
      });

      final api = HttpEventsApi(client: client, baseUrl: 'http://localhost');
      final result = await api.fetchEvents(
        dateFrom: '2025-03-01',
        dateTo: '2025-03-07',
      );
      expect(result.events.length, 1);
      expect(result.events[0].title, 'NFP');
    });

    test('fetchEvents passes optional impact and country', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.queryParameters['impact'], 'high');
        expect(req.url.queryParameters['country'], 'US');
        return http.Response(jsonEncode({'events': []}), 200);
      });

      final api = HttpEventsApi(client: client, baseUrl: 'http://localhost');
      await api.fetchEvents(
        dateFrom: '2025-03-01',
        dateTo: '2025-03-07',
        impact: 'high',
        country: 'US',
      );
    });

    test('fetchEvents throws on non-200', () async {
      final client = http_testing.MockClient(
          (req) async => http.Response('error', 500));

      final api = HttpEventsApi(client: client, baseUrl: 'http://localhost');
      expect(
        () => api.fetchEvents(dateFrom: '2025-03-01', dateTo: '2025-03-07'),
        throwsA(isA<HttpEventsApiException>()),
      );
    });

    test('fetchEvents does not send empty impact/country params', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.queryParameters.containsKey('impact'), isFalse);
        expect(req.url.queryParameters.containsKey('country'), isFalse);
        return http.Response(jsonEncode({'events': []}), 200);
      });

      final api = HttpEventsApi(client: client, baseUrl: 'http://localhost');
      await api.fetchEvents(dateFrom: '2025-03-01', dateTo: '2025-03-07');
    });
  });
}
