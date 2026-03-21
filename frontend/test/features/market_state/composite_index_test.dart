import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';

void main() {
  // ---- Data Model Tests ----

  group('CompositeIndexData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'symbolCount': 150,
        'points': [
          {'t': 1710300000, 'v': 100.0},
          {'t': 1710303600, 'v': 101.2},
        ],
      };

      final data = CompositeIndexData.fromJson(json);
      expect(data.timeframe, '4h');
      expect(data.symbolCount, 150);
      expect(data.points.length, 2);
      expect(data.points[0].timestamp, 1710300000);
      expect(data.points[0].value, 100.0);
      expect(data.points[1].timestamp, 1710303600);
      expect(data.points[1].value, 101.2);
    });

    test('fromJson handles empty points', () {
      final json = {
        'timeframe': '1h',
        'symbolCount': 0,
        'points': <dynamic>[],
      };

      final data = CompositeIndexData.fromJson(json);
      expect(data.points, isEmpty);
      expect(data.symbolCount, 0);
    });

    test('fromJson handles null points as empty list', () {
      final json = {
        'timeframe': '4h',
        'symbolCount': 5,
      };

      final data = CompositeIndexData.fromJson(json);
      expect(data.points, isEmpty);
    });

    test('IndexPoint fromJson handles integer values', () {
      final json = {'t': 1710300000, 'v': 100};
      final pt = IndexPoint.fromJson(json);
      expect(pt.timestamp, 1710300000);
      expect(pt.value, 100.0);
    });
  });

  // ---- API Tests ----

  group('HttpCompositeIndexApi', () {
    test('sends GET request with timeframe and limit', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'symbolCount': 50,
            'points': [
              {'t': 1000, 'v': 100.0},
            ],
          }),
          200,
        );
      });

      final api = HttpCompositeIndexApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final data = await api.fetch(timeframe: '4h', limit: 100);
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/composite?timeframe=4h&limit=100',
      );
      expect(data.symbolCount, 50);
      expect(data.points.length, 1);
    });

    test('uses default params', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'symbolCount': 10,
            'points': [],
          }),
          200,
        );
      });

      final api = HttpCompositeIndexApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/composite?timeframe=4h&limit=200',
      );
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('error', 500);
      });

      final api = HttpCompositeIndexApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpCompositeIndexApiException>()),
      );
    });
  });

  // ---- Widget Tests ----

  group('MarketPulseScreen', () {
    testWidgets('shows loading indicator initially', (tester) async {
      final stateApi = _NeverCompleteStateApi();
      final compositeApi = _NeverCompleteCompositeApi();

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
        ),
      ));
      await tester.pump();

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(find.text('Market Pulse'), findsOneWidget);
    });

    testWidgets('shows error state on failure', (tester) async {
      final stateApi = _ErrorStateApi();
      final compositeApi = _ErrorCompositeApi();

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Failed to load market data'), findsOneWidget);
      expect(find.text('Retry'), findsOneWidget);
    });

    testWidgets('shows state and composite data on success', (tester) async {
      final stateApi = _FakeStateApi(MarketStateData(
        timeframe: '4h',
        state: 'compression',
        confidence: 0.65,
        breadth: const MarketBreadth(
          sideways: 0.2,
          compression: 0.5,
          breakout: 0.1,
          trend: 0.2,
        ),
        symbolCount: 100,
      ));
      final compositeApi = _FakeCompositeApi(CompositeIndexData(
        timeframe: '4h',
        symbolCount: 80,
        points: const [
          IndexPoint(timestamp: 1000, value: 100.0),
          IndexPoint(timestamp: 2000, value: 101.5),
        ],
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
        ),
      ));
      await tester.pumpAndSettle();

      // State card
      expect(find.text('COMPRESSION'), findsOneWidget);
      expect(find.textContaining('65.0%'), findsOneWidget);
      // Composite card
      expect(find.text('Market Composite Index'), findsOneWidget);
      expect(find.textContaining('+1.50'), findsOneWidget);
      // Breadth card
      expect(find.text('Market Breadth'), findsOneWidget);
      expect(find.text('Sideways'), findsOneWidget);
      expect(find.text('Compression'), findsOneWidget);
    });

    testWidgets('back button pops navigation', (tester) async {
      bool popped = false;
      final stateApi = _FakeStateApi(MarketStateData(
        timeframe: '4h',
        state: 'trend',
        confidence: 0.8,
        breadth: const MarketBreadth(
          sideways: 0.1,
          compression: 0.05,
          breakout: 0.05,
          trend: 0.8,
        ),
        symbolCount: 50,
      ));
      final compositeApi = _FakeCompositeApi(CompositeIndexData(
        timeframe: '4h',
        symbolCount: 40,
        points: const [],
      ));

      await tester.pumpWidget(MaterialApp(
        home: Builder(
          builder: (ctx) => TextButton(
            onPressed: () {
              Navigator.of(ctx).push(MaterialPageRoute(
                builder: (_) => MarketPulseScreen(
                  marketStateApi: stateApi,
                  compositeIndexApi: compositeApi,
                ),
              ));
            },
            child: const Text('GO'),
          ),
        ),
      ));

      await tester.tap(find.text('GO'));
      await tester.pumpAndSettle();

      expect(find.text('Market Pulse'), findsOneWidget);

      // Tap back
      await tester.tap(find.byIcon(Icons.arrow_back_ios_new));
      await tester.pumpAndSettle();

      expect(find.text('GO'), findsOneWidget);
    });

    testWidgets('timeframe dropdown includes 1m', (tester) async {
      final stateApi = _FakeStateApi(MarketStateData(
        timeframe: '4h',
        state: 'trend',
        confidence: 0.8,
        breadth: const MarketBreadth(
          sideways: 0.1,
          compression: 0.05,
          breakout: 0.05,
          trend: 0.8,
        ),
        symbolCount: 50,
      ));
      final compositeApi = _FakeCompositeApi(CompositeIndexData(
        timeframe: '4h',
        symbolCount: 40,
        points: const [],
      ));

      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: stateApi,
          compositeIndexApi: compositeApi,
        ),
      ));
      await tester.pumpAndSettle();

      // Open the timeframe dropdown
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();

      // Verify 1m is available
      expect(find.text('1m').last, findsOneWidget);
    });
  });
}

// ---- Fakes ----

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

class _ErrorStateApi implements MarketStateApi {
  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) async {
    throw Exception('network error');
  }
}

class _ErrorCompositeApi implements CompositeIndexApi {
  @override
  Future<CompositeIndexData> fetch({String timeframe = '4h', int limit = 200}) async {
    throw Exception('network error');
  }
}

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
