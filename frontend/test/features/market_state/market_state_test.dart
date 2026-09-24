import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';

void main() {
  group('MarketStateData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.54,
        'breadth': {
          'sideways': 0.54,
          'compression': 0.22,
          'trend': 0.16,
          'expansion': 0.08,
        },
        'symbolCount': 150,
      };

      final data = MarketStateData.fromJson(json);

      expect(data.timeframe, '4h');
      expect(data.state, 'sideways');
      expect(data.confidence, 0.54);
      expect(data.symbolCount, 150);
      expect(data.breadth.sideways, 0.54);
      expect(data.breadth.compression, 0.22);
      expect(data.breadth.trend, 0.16);
      expect(data.breadth.expansion, 0.08);
      expect(data.windowBars, 0);
      expect(data.participation.hasData, isFalse);
    });

    test('fromJson parses windowBars, trendScore, participation', () {
      final json = {
        'timeframe': '4h',
        'state': 'trend',
        'confidence': 0.72,
        'regimeSource': 'composite_volume_weighted',
        'windowBars': 48,
        'trendScore': 0.72,
        'participation': {
          'up': 62,
          'down': 8,
          'ranging': 30,
          'total': 100,
        },
        'breadth': {
          'sideways': 0.1,
          'compression': 0.1,
          'trend': 0.7,
          'expansion': 0.1,
        },
        'symbolCount': 150,
      };

      final data = MarketStateData.fromJson(json);
      expect(data.windowBars, 48);
      expect(data.trendScore, 0.72);
      expect(data.participation.up, 62);
      expect(data.confidenceLabel, 'tape confidence');
    });

    test('fromJson handles integer confidence', () {
      final json = {
        'timeframe': '1h',
        'state': 'trend',
        'confidence': 1,
        'breadth': {
          'sideways': 0.0,
          'compression': 0.0,
          'trend': 1.0,
          'expansion': 0.0,
        },
        'symbolCount': 10,
      };

      final data = MarketStateData.fromJson(json);
      expect(data.confidence, 1.0);
    });

    test('fromJson defaults dataQuality to ok when absent for a normal '
        'legacy response', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.5,
        'breadth': {
          'sideways': 0.5,
          'compression': 0.2,
          'trend': 0.2,
          'expansion': 0.1,
        },
        'symbolCount': 120,
      };

      final data = MarketStateData.fromJson(json);
      expect(data.dataQuality, 'ok');
      expect(data.isDataUnavailable, isFalse);
    });

    test('fromJson treats an absent dataQuality with symbolCount 0 as '
        'unavailable (pre-PR-074 legacy outage shape)', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.0,
        'breadth': {
          'sideways': 0.0,
          'compression': 0.0,
          'trend': 0.0,
          'expansion': 0.0,
        },
        'symbolCount': 0,
      };

      final data = MarketStateData.fromJson(json);
      expect(data.dataQuality, 'unavailable');
      expect(data.isDataUnavailable, isTrue);
    });

    test('fromJson preserves an explicit dataQuality even with '
        'symbolCount 0', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.0,
        'breadth': {
          'sideways': 0.0,
          'compression': 0.0,
          'trend': 0.0,
          'expansion': 0.0,
        },
        'symbolCount': 0,
        'dataQuality': 'ok',
      };

      final data = MarketStateData.fromJson(json);
      expect(data.dataQuality, 'ok');
      expect(data.isDataUnavailable, isFalse);
    });

    test('fromJson parses dataQuality unavailable', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.0,
        'breadth': {
          'sideways': 0.0,
          'compression': 0.0,
          'trend': 0.0,
          'expansion': 0.0,
        },
        'symbolCount': 0,
        'dataQuality': 'unavailable',
      };

      final data = MarketStateData.fromJson(json);
      expect(data.dataQuality, 'unavailable');
      expect(data.isDataUnavailable, isTrue);
    });

    test('fromJson parses dataQuality degraded as not unavailable', () {
      final json = {
        'timeframe': '4h',
        'state': 'sideways',
        'confidence': 0.3,
        'breadth': {
          'sideways': 0.3,
          'compression': 0.1,
          'trend': 0.1,
          'expansion': 0.1,
        },
        'symbolCount': 10,
        'dataQuality': 'degraded',
      };

      final data = MarketStateData.fromJson(json);
      expect(data.dataQuality, 'degraded');
      expect(data.isDataUnavailable, isFalse);
    });
  });

  group('MarketBreadth', () {
    test('fromJson parses all regime ratios', () {
      final json = {
        'sideways': 0.5,
        'compression': 0.2,
        'expansion': 0.1,
        'trend': 0.2,
      };
      final breadth = MarketBreadth.fromJson(json);
      expect(breadth.sideways, 0.5);
      expect(breadth.compression, 0.2);
      expect(breadth.expansion, 0.1);
      expect(breadth.trend, 0.2);
    });
  });

  group('HttpMarketStateApi', () {
    test('sends GET request with timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'state': 'sideways',
            'confidence': 0.6,
            'breadth': {
              'sideways': 0.6,
              'compression': 0.2,
              'trend': 0.1,
              'expansion': 0.1,
            },
            'symbolCount': 100,
          }),
          200,
        );
      });

      final api = HttpMarketStateApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final data = await api.fetch(timeframe: '4h');
      expect(capturedUri.toString(),
          'http://localhost:8080/api/market/state?timeframe=4h');
      expect(data.state, 'sideways');
      expect(data.symbolCount, 100);
    });

    test('uses default timeframe 4h', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'state': 'trend',
            'confidence': 0.7,
            'breadth': {
              'sideways': 0.1,
              'compression': 0.1,
              'trend': 0.7,
              'expansion': 0.1,
            },
            'symbolCount': 50,
          }),
          200,
        );
      });

      final api = HttpMarketStateApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(capturedUri.toString(),
          'http://localhost:8080/api/market/state?timeframe=4h');
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpMarketStateApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpMarketStateApiException>()),
      );
    });
  });
}
