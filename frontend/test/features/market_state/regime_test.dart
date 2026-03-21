import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/http_regime_api.dart';
import 'package:pano_chart_frontend/features/market_state/regime_data.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('RegimeData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'regime': 'compression',
        'prevalence': 0.71,
        'scores': {
          'expansion': 0.05,
          'compression': 0.71,
          'trend': 0.14,
          'sideways': 0.10,
        },
        'metrics': {
          'trendBreadth': 0.18,
          'sidewaysBreadth': 0.10,
          'breakoutBreadth': 0.05,
          'compressionBreadth': 0.34,
          'volatilityExpansion': 0.82,
          'dispersion': 0.21,
        },
      };

      final data = RegimeData.fromJson(json);
      expect(data.timeframe, '4h');
      expect(data.regime, 'compression');
      expect(data.prevalence, 0.71);
      expect(data.scores.compression, 0.71);
      expect(data.scores.expansion, 0.05);
      expect(data.scores.trend, 0.14);
      expect(data.scores.sideways, 0.10);
      expect(data.metrics.trendBreadth, 0.18);
      expect(data.metrics.sidewaysBreadth, 0.10);
      expect(data.metrics.breakoutBreadth, 0.05);
      expect(data.metrics.compressionBreadth, 0.34);
      expect(data.metrics.volatilityExpansion, 0.82);
      expect(data.metrics.dispersion, 0.21);
    });

    test('fromJson handles integer values', () {
      final json = {
        'timeframe': '1h',
        'regime': 'sideways',
        'prevalence': 1,
        'scores': {
          'expansion': 0,
          'compression': 0,
          'trend': 0,
          'sideways': 1,
        },
        'metrics': {
          'trendBreadth': 0,
          'compressionBreadth': 0,
          'volatilityExpansion': 1,
          'dispersion': 0,
        },
      };

      final data = RegimeData.fromJson(json);
      expect(data.prevalence, 1.0);
      expect(data.scores.sideways, 1.0);
      expect(data.metrics.volatilityExpansion, 1.0);
    });
  });

  // ---- API Tests ----

  group('HttpRegimeApi', () {
    test('sends GET request with timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'regime': 'compression',
            'prevalence': 0.71,
            'scores': {
              'expansion': 0.05,
              'compression': 0.71,
              'trend': 0.14,
              'sideways': 0.10,
            },
            'metrics': {
              'trendBreadth': 0.18,
              'compressionBreadth': 0.34,
              'volatilityExpansion': 0.82,
              'dispersion': 0.21,
            },
          }),
          200,
        );
      });

      final api = HttpRegimeApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/regime?timeframe=1h',
      );
      expect(result.regime, 'compression');
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'regime': 'sideways',
            'prevalence': 0.5,
            'scores': {
              'expansion': 0.1,
              'compression': 0.1,
              'trend': 0.3,
              'sideways': 0.5,
            },
            'metrics': {
              'trendBreadth': 0.1,
              'compressionBreadth': 0.1,
              'volatilityExpansion': 1.0,
              'dispersion': 0.02,
            },
          }),
          200,
        );
      });

      final api = HttpRegimeApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/regime?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpRegimeApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpRegimeApiException>()),
      );
    });
  });

  // ---- Widget Tests ----

  group('MarketPulseScreen with regime', () {
    testWidgets('shows regime card when regimeApi is provided', (tester) async {
      // Increase surface so metrics card is visible below regime card
      tester.view.physicalSize = const Size(800, 2000);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final stateApi = _FakeStateApi(const MarketStateData(
        timeframe: '4h',
        state: 'sideways',
        confidence: 0.6,
        breadth: MarketBreadth(
          sideways: 0.6, compression: 0.2, breakout: 0.1, trend: 0.1,
        ),
        symbolCount: 50,
      ));
      final compositeApi = _FakeCompositeApi(const CompositeIndexData(
        timeframe: '4h',
        symbolCount: 40,
        points: [],
      ));
      final regimeApi = _FakeRegimeApi(const RegimeData(
        timeframe: '4h',
        regime: 'compression',
        prevalence: 0.71,
        scores: RegimeScores(
          expansion: 0.05, compression: 0.71, trend: 0.14, sideways: 0.10,
        ),
        metrics: RegimeMetrics(
          trendBreadth: 0.18,
          sidewaysBreadth: 0.10,
          breakoutBreadth: 0.05,
          compressionBreadth: 0.34,
          volatilityExpansion: 0.82,
          dispersion: 0.21,
        ),
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Regime card shown instead of state card
      expect(find.text('COMPRESSION'), findsOneWidget);
      expect(find.text('71% prevalence  •  4h'), findsOneWidget);

      // Metrics card shown
      expect(find.text('Market Metrics'), findsOneWidget);
      expect(find.text('Volatility'), findsOneWidget);
      expect(find.text('normal'), findsOneWidget); // 0.82 is between 0.8 and 1.3
      expect(find.text('Dispersion'), findsOneWidget);
    });

    testWidgets('shows state card as fallback without regimeApi',
        (tester) async {
      final stateApi = _FakeStateApi(const MarketStateData(
        timeframe: '4h',
        state: 'trend',
        confidence: 0.8,
        breadth: MarketBreadth(
          sideways: 0.1, compression: 0.1, breakout: 0.0, trend: 0.8,
        ),
        symbolCount: 50,
      ));
      final compositeApi = _FakeCompositeApi(const CompositeIndexData(
        timeframe: '4h',
        symbolCount: 40,
        points: [],
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          // no regimeApi
        ),
      ));
      await tester.pumpAndSettle();

      // State card shown (fallback)
      expect(find.text('TREND'), findsOneWidget);
      // No metrics card
      expect(find.text('Market Metrics'), findsNothing);
    });

    testWidgets('shows volatility label "high" for expansion',
        (tester) async {
      // Increase surface so metrics card is visible below regime card
      tester.view.physicalSize = const Size(800, 2000);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final stateApi = _FakeStateApi(const MarketStateData(
        timeframe: '4h',
        state: 'sideways',
        confidence: 0.5,
        breadth: MarketBreadth(
          sideways: 1.0, compression: 0.0, breakout: 0.0, trend: 0.0,
        ),
        symbolCount: 10,
      ));
      final compositeApi = _FakeCompositeApi(const CompositeIndexData(
        timeframe: '4h',
        symbolCount: 10,
        points: [],
      ));
      final regimeApi = _FakeRegimeApi(const RegimeData(
        timeframe: '4h',
        regime: 'expansion',
        prevalence: 0.90,
        scores: RegimeScores(
          expansion: 0.90,
          compression: 0.02,
          trend: 0.05,
          sideways: 0.03,
        ),
        metrics: RegimeMetrics(
          trendBreadth: 0.0,
          sidewaysBreadth: 0.0,
          breakoutBreadth: 0.0,
          compressionBreadth: 0.0,
          volatilityExpansion: 1.5,
          dispersion: 0.08,
        ),
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('EXPANSION'), findsOneWidget);
      expect(find.text('high'), findsAtLeastNWidgets(2)); // volatility + dispersion
    });

    testWidgets('loading state shown when regime API pending', (tester) async {
      final stateApi = _NeverCompleteStateApi();
      final compositeApi = _NeverCompleteCompositeApi();
      final regimeApi = _NeverCompleteRegimeApi();

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
        ),
      ));
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });
  });
}

// ---- Fakes ----

class _FakeStateApi implements MarketStateApi {
  final MarketStateData data;
  _FakeStateApi(this.data);

  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) async => data;
}

class _FakeCompositeApi implements CompositeIndexApi {
  final CompositeIndexData data;
  _FakeCompositeApi(this.data);

  @override
  Future<CompositeIndexData> fetch({String timeframe = '4h', int limit = 200}) async => data;
}

class _FakeRegimeApi implements RegimeApi {
  final RegimeData data;
  _FakeRegimeApi(this.data);

  @override
  Future<RegimeData> fetch({String timeframe = '4h'}) async => data;
}

class _NeverCompleteStateApi implements MarketStateApi {
  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) {
    return Completer<MarketStateData>().future;
  }
}

class _NeverCompleteCompositeApi implements CompositeIndexApi {
  @override
  Future<CompositeIndexData> fetch({String timeframe = '4h', int limit = 200}) {
    return Completer<CompositeIndexData>().future;
  }
}

class _NeverCompleteRegimeApi implements RegimeApi {
  @override
  Future<RegimeData> fetch({String timeframe = '4h'}) {
    return Completer<RegimeData>().future;
  }
}
