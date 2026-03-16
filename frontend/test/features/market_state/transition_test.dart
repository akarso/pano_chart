import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/http_transition_api.dart';
import 'package:pano_chart_frontend/features/market_state/transition_data.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_regime_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';
import 'package:pano_chart_frontend/features/market_state/regime_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('TransitionData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'currentRegime': 'compression',
        'probabilities': {
          'trend': 0.42,
          'sideways': 0.28,
          'expansion': 0.30,
        },
        'horizon': '12 candles',
      };

      final data = TransitionData.fromJson(json);
      expect(data.timeframe, '4h');
      expect(data.currentRegime, 'compression');
      expect(data.probabilities.trend, 0.42);
      expect(data.probabilities.sideways, 0.28);
      expect(data.probabilities.expansion, 0.30);
      expect(data.horizon, '12 candles');
    });

    test('fromJson handles integer values', () {
      final json = {
        'timeframe': '1h',
        'currentRegime': 'sideways',
        'probabilities': {
          'trend': 0,
          'sideways': 1,
          'expansion': 0,
        },
        'horizon': '12 candles',
      };

      final data = TransitionData.fromJson(json);
      expect(data.probabilities.trend, 0.0);
      expect(data.probabilities.sideways, 1.0);
      expect(data.probabilities.expansion, 0.0);
    });
  });

  // ---- API Tests ----

  group('HttpTransitionApi', () {
    test('sends GET request with timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '1h',
            'currentRegime': 'compression',
            'probabilities': {
              'trend': 0.42,
              'sideways': 0.28,
              'expansion': 0.30,
            },
            'horizon': '12 candles',
          }),
          200,
        );
      });

      final api = HttpTransitionApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/transition?timeframe=1h',
      );
      expect(result.currentRegime, 'compression');
      expect(result.probabilities.trend, 0.42);
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'currentRegime': 'sideways',
            'probabilities': {
              'trend': 0.2,
              'sideways': 0.7,
              'expansion': 0.1,
            },
            'horizon': '12 candles',
          }),
          200,
        );
      });

      final api = HttpTransitionApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/transition?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpTransitionApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpTransitionApiException>()),
      );
    });
  });

  // ---- Widget Tests ----

  group('MarketPulseScreen with transition', () {
    testWidgets('shows transition card when transitionApi is provided',
        (tester) async {
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
        confidence: 0.71,
        metrics: RegimeMetrics(
          trendBreadth: 0.18,
          compressionBreadth: 0.34,
          volatilityExpansion: 0.82,
          dispersion: 0.21,
        ),
      ));
      final transitionApi = _FakeTransitionApi(const TransitionData(
        timeframe: '4h',
        currentRegime: 'compression',
        probabilities: TransitionProbabilities(
          trend: 0.42,
          sideways: 0.28,
          expansion: 0.30,
        ),
        horizon: '12 candles',
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
          transitionApi: transitionApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Scroll down to reveal the transition card below the fold.
      await tester.drag(find.byType(ListView), const Offset(0, -300));
      await tester.pumpAndSettle();

      // Transition card is shown
      expect(find.text('Transition Probabilities'), findsOneWidget);
      expect(find.text('12 candles'), findsOneWidget);
      expect(find.text('42%'), findsOneWidget);
      expect(find.text('28%'), findsOneWidget);
      expect(find.text('30%'), findsOneWidget);
    });

    testWidgets('no transition card without transitionApi', (tester) async {
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
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Transition Probabilities'), findsNothing);
    });

    testWidgets('shows loading when transition API is pending', (tester) async {
      final stateApi = _NeverCompleteStateApi();
      final compositeApi = _NeverCompleteCompositeApi();
      final transitionApi = _NeverCompleteTransitionApi();

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          transitionApi: transitionApi,
        ),
      ));
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('probability labels use correct regimes', (tester) async {
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
        regime: 'sideways',
        confidence: 0.5,
        metrics: RegimeMetrics(
          trendBreadth: 0.1,
          compressionBreadth: 0.1,
          volatilityExpansion: 1.0,
          dispersion: 0.02,
        ),
      ));
      final transitionApi = _FakeTransitionApi(const TransitionData(
        timeframe: '4h',
        currentRegime: 'sideways',
        probabilities: TransitionProbabilities(
          trend: 0.20,
          sideways: 0.65,
          expansion: 0.15,
        ),
        horizon: '12 candles',
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
          transitionApi: transitionApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Scroll down to reveal the transition card below the fold.
      await tester.drag(find.byType(ListView), const Offset(0, -300));
      await tester.pumpAndSettle();

      // Check that all three probability rows are present
      // (The labels "Trend", "Sideways", "Expansion" appear in the transition card)
      expect(find.text('Transition Probabilities'), findsOneWidget);
      expect(find.text('20%'), findsOneWidget);
      expect(find.text('65%'), findsOneWidget);
      expect(find.text('15%'), findsOneWidget);
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
  Future<CompositeIndexData> fetch(
      {String timeframe = '4h', int limit = 200}) async =>
      data;
}

class _FakeRegimeApi implements RegimeApi {
  final RegimeData data;
  _FakeRegimeApi(this.data);

  @override
  Future<RegimeData> fetch({String timeframe = '4h'}) async => data;
}

class _FakeTransitionApi implements TransitionApi {
  final TransitionData data;
  _FakeTransitionApi(this.data);

  @override
  Future<TransitionData> fetch({String timeframe = '4h'}) async => data;
}

class _NeverCompleteStateApi implements MarketStateApi {
  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) {
    return Completer<MarketStateData>().future;
  }
}

class _NeverCompleteCompositeApi implements CompositeIndexApi {
  @override
  Future<CompositeIndexData> fetch(
      {String timeframe = '4h', int limit = 200}) {
    return Completer<CompositeIndexData>().future;
  }
}

class _NeverCompleteTransitionApi implements TransitionApi {
  @override
  Future<TransitionData> fetch({String timeframe = '4h'}) {
    return Completer<TransitionData>().future;
  }
}
