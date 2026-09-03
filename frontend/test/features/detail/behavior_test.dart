import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/detail/http_behavior_api.dart';
import 'package:pano_chart_frontend/features/detail/behavior_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('BehaviorData', () {
    test('fromJson parses all fields', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'greed': 0.68,
        'fear': 0.32,
        'patience': 0.51,
        'panic': 0.21,
        'summary': 'Neutral sentiment',
      };

      final data = BehaviorData.fromJson(json);
      expect(data.symbol, 'BTCUSDT');
      expect(data.timeframe, '4h');
      expect(data.greed, 0.68);
      expect(data.fear, 0.32);
      expect(data.patience, 0.51);
      expect(data.panic, 0.21);
      expect(data.summary, 'Neutral sentiment');
    });

    test('fromJson handles integer values', () {
      final json = {
        'symbol': 'ETHUSDT',
        'timeframe': '1h',
        'greed': 1,
        'fear': 0,
        'patience': 1,
        'panic': 0,
        'summary': 'Greed dominant',
      };

      final data = BehaviorData.fromJson(json);
      expect(data.greed, 1.0);
      expect(data.fear, 0.0);
    });

    test('dimensions returns all four dimensions', () {
      final data = BehaviorData(
        symbol: 'BTCUSDT',
        timeframe: '4h',
        greed: 0.5,
        fear: 0.3,
        patience: 0.6,
        panic: 0.1,
        summary: 'Neutral sentiment',
      );

      final dims = data.dimensions;
      expect(dims.length, 4);
      expect(dims['greed'], 0.5);
      expect(dims['fear'], 0.3);
      expect(dims['patience'], 0.6);
      expect(dims['panic'], 0.1);
    });

    test('dimensionLabel returns human-readable labels', () {
      expect(BehaviorData.dimensionLabel('greed'), 'Greed');
      expect(BehaviorData.dimensionLabel('fear'), 'Fear');
      expect(BehaviorData.dimensionLabel('patience'), 'Patience');
      expect(BehaviorData.dimensionLabel('panic'), 'Panic');
      expect(BehaviorData.dimensionLabel('unknown'), 'unknown');
    });
  });

  // ---- API Tests ----

  group('HttpBehaviorApi', () {
    test('sends GET request with symbol and timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'BTCUSDT',
            'timeframe': '1h',
            'greed': 0.68,
            'fear': 0.32,
            'patience': 0.51,
            'panic': 0.21,
            'summary': 'Neutral sentiment',
          }),
          200,
        );
      });

      final api = HttpBehaviorApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(symbol: 'BTCUSDT', timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/BTCUSDT/behavior?timeframe=1h',
      );
      expect(result.symbol, 'BTCUSDT');
      expect(result.timeframe, '1h');
      expect(result.greed, 0.68);
      expect(result.summary, 'Neutral sentiment');
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'ETHUSDT',
            'timeframe': '4h',
            'greed': 0.3,
            'fear': 0.2,
            'patience': 0.5,
            'panic': 0.1,
            'summary': 'Market waiting / coiling',
          }),
          200,
        );
      });

      final api = HttpBehaviorApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch(symbol: 'ETHUSDT');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/ETHUSDT/behavior?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpBehaviorApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(symbol: 'BTCUSDT'),
        throwsA(isA<HttpBehaviorApiException>()),
      );
    });

    test('throws on timeout', () async {
      final client = MockClient((_) async {
        await Future.delayed(const Duration(seconds: 30));
        return http.Response('{}', 200);
      });

      final api = HttpBehaviorApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(symbol: 'BTCUSDT'),
        throwsA(anything),
      );
    });
  });
}
