import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/detail/http_mtf_regimes_api.dart';
import 'package:pano_chart_frontend/features/detail/mtf_regimes_data.dart';
import 'package:pano_chart_frontend/features/detail/mtf_strip_presentation.dart';
import 'package:pano_chart_frontend/features/market_state/regime_colors.dart';

void main() {
  // ---- Data Model Tests ----

  group('MtfRegimesData', () {
    test('fromJson parses all fields', () {
      final json = {
        'symbol': 'BTCUSDT',
        'frames': [
          {
            'timeframe': '15m',
            'structure': {
              'trend': 0.72,
              'sideways': 0.15,
              'compression': 0.08,
              'expansion': 0.05,
            },
            'dominant': 'trend',
            'bias': 'up',
            'score': 0.72,
          },
        ],
        'alignment': 1.0,
        'alignedState': 'trend',
      };

      final data = MtfRegimesData.fromJson(json);
      expect(data.symbol, 'BTCUSDT');
      expect(data.alignment, 1.0);
      expect(data.alignedState, 'trend');
      expect(data.frames.length, 1);
      expect(data.frames[0].timeframe, '15m');
      expect(data.frames[0].dominant, 'trend');
      expect(data.frames[0].bias, 'up');
      expect(data.frames[0].score, 0.72);
    });

    test('fromJson handles missing frames as empty list', () {
      final data = MtfRegimesData.fromJson({'symbol': 'ETHUSDT'});
      expect(data.frames, isEmpty);
      expect(data.alignment, 0.0);
      expect(data.alignedState, 'indecisive');
    });
  });

  // ---- API Tests ----

  group('HttpMtfRegimesApi', () {
    test('sends GET request to /api/symbol/{symbol}/regimes', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'symbol': 'BTCUSDT',
            'frames': <dynamic>[],
            'alignment': 0.0,
            'alignedState': 'indecisive',
          }),
          200,
        );
      });

      final api = HttpMtfRegimesApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final data = await api.fetch(symbol: 'BTCUSDT');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/symbol/BTCUSDT/regimes',
      );
      expect(data.symbol, 'BTCUSDT');
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async => http.Response('error', 500));
      final api = HttpMtfRegimesApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(symbol: 'BTCUSDT'),
        throwsA(isA<HttpMtfRegimesApiException>()),
      );
    });
  });

  // ---- Presentation Tests ----

  group('buildMtfPills', () {
    test('fills all four slots in fixed order, missing frames as null', () {
      const data = MtfRegimesData(
        symbol: 'BTCUSDT',
        frames: [
          MtfFrame(timeframe: '1h', dominant: 'trend', bias: 'up', score: 0.8),
          MtfFrame(
              timeframe: '1d',
              dominant: 'compression',
              bias: 'neutral',
              score: 0.5),
        ],
        alignment: 0.5,
        alignedState: 'indecisive',
      );

      final pills = buildMtfPills(data);
      expect(pills.map((p) => p.timeframe), kMtfStripTimeframes);
      expect(pills[0].dominant, isNull); // 15m missing
      expect(pills[1].dominant, 'trend'); // 1h
      expect(pills[1].bias, 'up');
      expect(pills[2].dominant, isNull); // 4h missing
      expect(pills[3].dominant, 'compression'); // 1d
    });

    test('null data yields four empty pills', () {
      final pills = buildMtfPills(null);
      expect(pills.length, 4);
      for (final p in pills) {
        expect(p.dominant, isNull);
        expect(p.bias, 'neutral');
      }
    });
  });

  group('regimeColor', () {
    test('maps each dominant regime to its Market Pulse color', () {
      expect(regimeColor('trend'), Colors.tealAccent);
      expect(regimeColor('sideways'), Colors.blueGrey);
      expect(regimeColor('compression'), Colors.amber);
      expect(regimeColor('expansion'), Colors.redAccent);
      expect(regimeColor('indecisive'), const Color(0xFFB0C4DE));
    });
  });
}
