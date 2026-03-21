import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/http_regime_history_api.dart';
import 'package:pano_chart_frontend/features/market_state/regime_history_data.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_regime_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_transition_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';
import 'package:pano_chart_frontend/features/market_state/regime_data.dart';
import 'package:pano_chart_frontend/features/market_state/transition_data.dart';

void main() {
  // ---- Data Model Tests ----

  group('RegimeHistoryData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'currentAge': 17,
        'history': [
          {
            'regime': 'sideways',
            'start': 1000,
            'end': 2000,
            'durationCandles': 5,
          },
          {
            'regime': 'compression',
            'start': 2000,
            'end': null,
            'durationCandles': 17,
          },
        ],
      };

      final data = RegimeHistoryData.fromJson(json);
      expect(data.timeframe, '4h');
      expect(data.currentAge, 17);
      expect(data.history.length, 2);
      expect(data.history[0].regime, 'sideways');
      expect(data.history[0].start, 1000);
      expect(data.history[0].end, 2000);
      expect(data.history[0].durationCandles, 5);
      expect(data.history[1].regime, 'compression');
      expect(data.history[1].end, isNull);
    });

    test('fromJson handles empty history', () {
      final json = {
        'timeframe': '1h',
        'currentAge': 0,
        'history': <dynamic>[],
      };

      final data = RegimeHistoryData.fromJson(json);
      expect(data.history, isEmpty);
      expect(data.currentAge, 0);
    });

    test('fromJson handles null history', () {
      final json = {
        'timeframe': '4h',
        'currentAge': 0,
      };

      final data = RegimeHistoryData.fromJson(json);
      expect(data.history, isEmpty);
    });
  });

  // ---- API Tests ----

  group('HttpRegimeHistoryApi', () {
    test('sends GET request with timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '1h',
            'currentAge': 5,
            'history': [
              {
                'regime': 'trend',
                'start': 1000,
                'end': 2000,
                'durationCandles': 5,
              },
            ],
          }),
          200,
        );
      });

      final api = HttpRegimeHistoryApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final result = await api.fetch(timeframe: '1h');
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/regime/history?timeframe=1h',
      );
      expect(result.currentAge, 5);
      expect(result.history.length, 1);
    });

    test('uses default timeframe', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'currentAge': 0,
            'history': <dynamic>[],
          }),
          200,
        );
      });

      final api = HttpRegimeHistoryApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/regime/history?timeframe=4h',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpRegimeHistoryApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpRegimeHistoryApiException>()),
      );
    });
  });

  // ---- Widget Tests ----

  group('MarketPulseScreen with regime history', () {
    testWidgets('shows regime history card when regimeHistoryApi is provided',
        (tester) async {
      final stateApi = _FakeStateApi(const MarketStateData(
        timeframe: '4h',
        state: 'compression',
        confidence: 0.7,
        breadth: MarketBreadth(
          sideways: 0.3, compression: 0.4, breakout: 0.1, trend: 0.2,
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
      final historyApi = _FakeRegimeHistoryApi(const RegimeHistoryData(
        timeframe: '4h',
        currentAge: 17,
        history: [
          RegimePeriodData(
            regime: 'sideways',
            start: 1000,
            end: 2000,
            durationCandles: 5,
          ),
          RegimePeriodData(
            regime: 'compression',
            start: 2000,
            durationCandles: 17,
          ),
        ],
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeApi: regimeApi,
          transitionApi: transitionApi,
          regimeHistoryApi: historyApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Scroll down to reveal the history card below the fold.
      await tester.drag(find.byType(ListView), const Offset(0, -500));
      await tester.pumpAndSettle();

      // Regime history card is shown
      expect(find.text('Regime History'), findsOneWidget);
      expect(find.text('Age: 17 candles'), findsOneWidget);

      // Timeline entries should be present
      expect(find.textContaining('COMPRESSION'), findsOneWidget);
      expect(find.textContaining('SIDEWAYS'), findsOneWidget);
    });

    testWidgets('no history card without regimeHistoryApi', (tester) async {
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

      expect(find.text('Regime History'), findsNothing);
    });

    testWidgets('shows empty history message', (tester) async {
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
      final historyApi = _FakeRegimeHistoryApi(const RegimeHistoryData(
        timeframe: '4h',
        currentAge: 0,
        history: [],
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
          regimeHistoryApi: historyApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Scroll down to find the card.
      await tester.drag(find.byType(ListView), const Offset(0, -300));
      await tester.pumpAndSettle();

      expect(find.text('Regime History'), findsOneWidget);
      expect(find.text('No history yet'), findsOneWidget);
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

class _FakeRegimeHistoryApi implements RegimeHistoryApi {
  final RegimeHistoryData data;
  _FakeRegimeHistoryApi(this.data);

  @override
  Future<RegimeHistoryData> fetch({String timeframe = '4h'}) async => data;
}
