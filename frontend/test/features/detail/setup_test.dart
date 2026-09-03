import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/detail/http_setup_api.dart';
import 'package:pano_chart_frontend/features/detail/setup_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('SetupData', () {
    test('fromJson parses all fields', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'bestSetup': 'compression_breakout',
        'score': 0.78,
        'scores': {
          'compression_breakout': 0.78,
          'trend_continuation': 0.34,
          'range_reversion': 0.22,
        },
      };

      final data = SetupData.fromJson(json);
      expect(data.symbol, 'BTCUSDT');
      expect(data.timeframe, '4h');
      expect(data.bestSetup, 'compression_breakout');
      expect(data.score, 0.78);
      expect(data.scores.length, 3);
      expect(data.scores['compression_breakout'], 0.78);
      expect(data.scores['trend_continuation'], 0.34);
      expect(data.scores['range_reversion'], 0.22);
    });

    test('fromJson handles integer score', () {
      final json = {
        'symbol': 'ETHUSDT',
        'timeframe': '1h',
        'bestSetup': 'trend_continuation',
        'score': 1,
        'scores': {
          'trend_continuation': 1,
        },
      };

      final data = SetupData.fromJson(json);
      expect(data.score, 1.0);
      expect(data.scores['trend_continuation'], 1.0);
    });

    test('fromJson handles empty scores', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'bestSetup': '',
        'score': 0.0,
        'scores': <String, dynamic>{},
      };

      final data = SetupData.fromJson(json);
      expect(data.scores, isEmpty);
      expect(data.bestSetup, '');
    });

    test('fromJson handles null scores gracefully', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'bestSetup': '',
        'score': 0.0,
      };

      final data = SetupData.fromJson(json);
      expect(data.scores, isEmpty);
    });

    test('displayName returns human-readable names', () {
      expect(
        SetupData.displayName('compression_breakout'),
        'Compression Breakout',
      );
      expect(
        SetupData.displayName('trend_continuation'),
        'Trend Continuation',
      );
      expect(
        SetupData.displayName('range_reversion'),
        'Range Reversion',
      );
      expect(
        SetupData.displayName('unknown_type'),
        'unknown_type',
      );
    });
  });

  // ---- API Tests ----

  group('HttpSetupApi', () {
    test('sends GET request with symbol and timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'BTCUSDT',
            'timeframe': '1h',
            'bestSetup': 'compression_breakout',
            'score': 0.78,
            'scores': {
              'compression_breakout': 0.78,
              'trend_continuation': 0.34,
              'range_reversion': 0.22,
            },
          }),
          200,
        );
      });

      final api = HttpSetupApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(symbol: 'BTCUSDT', timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/BTCUSDT/setup?timeframe=1h',
      );
      expect(result.symbol, 'BTCUSDT');
      expect(result.timeframe, '1h');
      expect(result.bestSetup, 'compression_breakout');
      expect(result.score, 0.78);
      expect(result.scores.length, 3);
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'ETHUSDT',
            'timeframe': '4h',
            'bestSetup': 'trend_continuation',
            'score': 0.5,
            'scores': {},
          }),
          200,
        );
      });

      final api = HttpSetupApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch(symbol: 'ETHUSDT');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/ETHUSDT/setup?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpSetupApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(symbol: 'BTCUSDT'),
        throwsA(isA<HttpSetupApiException>()),
      );
    });

    test('throws on timeout', () async {
      final client = MockClient((_) async {
        await Future.delayed(const Duration(seconds: 30));
        return http.Response('{}', 200);
      });

      final api = HttpSetupApi(
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
