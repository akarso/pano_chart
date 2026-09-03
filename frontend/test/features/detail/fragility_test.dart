import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/detail/http_fragility_api.dart';
import 'package:pano_chart_frontend/features/detail/fragility_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('FragilityData', () {
    test('fromJson parses all fields', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'fragilityScore': 0.575,
        'riskLevel': 'medium',
        'dominantSide': 'long',
        'squeezeRisk': 'long_squeeze',
        'components': {
          'fundingExtremeness': 0.5,
          'oiExpansion': 0.8,
          'longShortImbalance': 0.3,
          'liquidationProximity': 0.6,
        },
      };

      final data = FragilityData.fromJson(json);
      expect(data.symbol, 'BTCUSDT');
      expect(data.timeframe, '4h');
      expect(data.fragilityScore, 0.575);
      expect(data.riskLevel, 'medium');
      expect(data.dominantSide, 'long');
      expect(data.squeezeRisk, 'long_squeeze');
      expect(data.components.fundingExtremeness, 0.5);
      expect(data.components.oiExpansion, 0.8);
      expect(data.components.longShortImbalance, 0.3);
      expect(data.components.liquidationProximity, 0.6);
    });

    test('fromJson handles integer score', () {
      final json = {
        'symbol': 'ETHUSDT',
        'timeframe': '1h',
        'fragilityScore': 1,
        'riskLevel': 'high',
        'components': {
          'fundingExtremeness': 1,
          'oiExpansion': 1,
          'longShortImbalance': 1,
          'liquidationProximity': 1,
        },
      };

      final data = FragilityData.fromJson(json);
      expect(data.fragilityScore, 1.0);
      expect(data.components.fundingExtremeness, 1.0);
      expect(data.components.oiExpansion, 1.0);
    });

    test('fromJson handles missing components', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'fragilityScore': 0.0,
        'riskLevel': 'low',
      };

      final data = FragilityData.fromJson(json);
      expect(data.dominantSide, 'neutral');
      expect(data.squeezeRisk, 'none');
      expect(data.components.fundingExtremeness, 0);
      expect(data.components.oiExpansion, 0);
      expect(data.components.longShortImbalance, 0);
      expect(data.components.liquidationProximity, 0);
    });

    test('fromJson handles partial components', () {
      final json = {
        'symbol': 'BTCUSDT',
        'timeframe': '4h',
        'fragilityScore': 0.3,
        'riskLevel': 'low',
        'components': {
          'fundingExtremeness': 0.5,
        },
      };

      final data = FragilityData.fromJson(json);
      expect(data.components.fundingExtremeness, 0.5);
      expect(data.components.oiExpansion, 0);
    });

    test('riskLabel returns human-readable labels', () {
      expect(FragilityData.riskLabel('high'), 'High Risk');
      expect(FragilityData.riskLabel('medium'), 'Medium Risk');
      expect(FragilityData.riskLabel('low'), 'Low Risk');
      expect(FragilityData.riskLabel('unknown'), 'unknown');
    });

    test('squeezeLabel returns human-readable labels', () {
      expect(FragilityData.squeezeLabel('long_squeeze'), 'Long Squeeze');
      expect(FragilityData.squeezeLabel('short_squeeze'), 'Short Squeeze');
      expect(FragilityData.squeezeLabel('none'), 'None');
      expect(FragilityData.squeezeLabel('unknown'), 'None');
    });

    test('sideLabel returns human-readable labels', () {
      expect(FragilityData.sideLabel('long'), 'Crowded Long');
      expect(FragilityData.sideLabel('short'), 'Crowded Short');
      expect(FragilityData.sideLabel('neutral'), 'Neutral');
      expect(FragilityData.sideLabel('other'), 'Neutral');
    });

    test('fromJson parses short dominant side', () {
      final json = {
        'symbol': 'ETHUSDT',
        'timeframe': '1h',
        'fragilityScore': 0.7,
        'riskLevel': 'high',
        'dominantSide': 'short',
        'squeezeRisk': 'short_squeeze',
        'components': <String, dynamic>{},
      };

      final data = FragilityData.fromJson(json);
      expect(data.dominantSide, 'short');
      expect(data.squeezeRisk, 'short_squeeze');
    });
  });

  group('FragilityComponents', () {
    test('displayName returns friendly labels', () {
      expect(
        FragilityComponents.displayName('fundingExtremeness'),
        'Funding',
      );
      expect(
        FragilityComponents.displayName('oiExpansion'),
        'OI Expansion',
      );
      expect(
        FragilityComponents.displayName('longShortImbalance'),
        'L/S Imbalance',
      );
      expect(
        FragilityComponents.displayName('liquidationProximity'),
        'Liq. Proximity',
      );
      expect(
        FragilityComponents.displayName('unknownKey'),
        'unknownKey',
      );
    });
  });

  // ---- API Tests ----

  group('HttpFragilityApi', () {
    test('sends GET request with symbol and timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'BTCUSDT',
            'timeframe': '1h',
            'fragilityScore': 0.575,
            'riskLevel': 'medium',
            'components': {
              'fundingExtremeness': 0.5,
              'oiExpansion': 0.8,
              'longShortImbalance': 0.3,
              'liquidationProximity': 0.6,
            },
          }),
          200,
        );
      });

      final api = HttpFragilityApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(symbol: 'BTCUSDT', timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/BTCUSDT/fragility?timeframe=1h',
      );
      expect(result.symbol, 'BTCUSDT');
      expect(result.timeframe, '1h');
      expect(result.fragilityScore, 0.575);
      expect(result.riskLevel, 'medium');
      expect(result.components.fundingExtremeness, 0.5);
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'ETHUSDT',
            'timeframe': '4h',
            'fragilityScore': 0.3,
            'riskLevel': 'low',
            'components': {},
          }),
          200,
        );
      });

      final api = HttpFragilityApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch(symbol: 'ETHUSDT');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/token/ETHUSDT/fragility?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpFragilityApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(symbol: 'BTCUSDT'),
        throwsA(isA<HttpFragilityApiException>()),
      );
    });

    test('throws on timeout', () async {
      final client = MockClient((_) async {
        await Future.delayed(const Duration(seconds: 30));
        return http.Response('{}', 200);
      });

      final api = HttpFragilityApi(
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
