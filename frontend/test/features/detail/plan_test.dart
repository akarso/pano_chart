import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart/interactive_chart.dart';
import 'package:pano_chart_frontend/features/detail/chart/chart_config.dart';
import 'package:pano_chart_frontend/features/detail/chart/plan_levels_painter.dart';
import 'package:pano_chart_frontend/features/detail/detail_context.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/detail/http_plan_api.dart';
import 'package:pano_chart_frontend/features/detail/plan_data.dart';
import 'package:pano_chart_frontend/features/detail/plan_panel.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('PlanData', () {
    test('fromJson parses COMMON contract fields', () {
      final data = PlanData.fromJson({
        'symbol': 'BTCUSDT',
        'timeframe': '1h',
        'low': 100,
        'high': 110,
        'mid': 105,
        'atr': 1,
        'price': 105,
        'longEntry': 100.25,
        'longStop': 99,
        'longTarget': 105,
        'longTargetFull': 109.75,
        'shortEntry': 109.75,
        'shortStop': 111,
        'shortTarget': 105,
        'shortTargetFull': 100.25,
        'riskReward': 3.8,
        'rangeQuality': 0.72,
        'position': 0.5,
        'valid': true,
        'size': 80,
        'shortSize': 80,
      });
      expect(data.valid, isTrue);
      expect(data.longEntry, 100.25);
      expect(data.riskReward, 3.8);
    });

    test('sizeFor matches backend Size(100, 100.25, 99)=80', () {
      expect(PlanData.sizeFor(100, 100.25, 99), closeTo(80, 1e-9));
      expect(PlanData.sizeFor(0, 100.25, 99), 0);
    });

    test('expandPriceRange includes channel and trade levels when valid', () {
      const levels = PlanChartLevels(
        low: 90,
        mid: 100,
        high: 110,
        valid: true,
        entry: 90.25,
        stop: 89,
        target: 100,
      );
      var lo = 98.0;
      var hi = 102.0;
      levels.expandPriceRange((v) {
        if (v < lo) lo = v;
        if (v > hi) hi = v;
      });
      expect(lo, 89);
      expect(hi, 110);
    });
  });

  group('HttpPlanApi', () {
    test('GET /api/symbol/{symbol}/plan with timeframe and risk', () async {
      Uri? captured;
      final client = MockClient((req) async {
        captured = req.url;
        return http.Response(jsonEncode(_validJson()), 200);
      });
      final api = HttpPlanApi(client: client, baseUrl: 'http://localhost:8080');
      await api.fetch(symbol: 'BTCUSDT', timeframe: '1h', risk: 100);
      expect(captured.toString(), contains('/api/symbol/BTCUSDT/plan'));
      expect(captured!.queryParameters['timeframe'], '1h');
      expect(captured!.queryParameters['risk'], '100');
    });
  });

  group('PlanLevelsPainter / chart overlay', () {
    test('channel Y positions land inside pane when range includes levels', () {
      const levels = PlanChartLevels(
        low: 100,
        mid: 105,
        high: 110,
        valid: true,
        entry: 100.25,
        stop: 99,
        target: 105,
      );
      // Simulate InteractiveChart expand + paint geometry.
      var lo = 104.0;
      var hi = 106.0;
      levels.expandPriceRange((v) {
        if (v < lo) lo = v;
        if (v > hi) hi = v;
      });
      expect(lo, lessThanOrEqualTo(99));
      expect(hi, greaterThanOrEqualTo(110));

      const size = Size(300, 200);
      const padFrac = 0.06;
      final pad = size.height * padFrac;
      final chartH = size.height - 2 * pad;
      final range = hi - lo;
      double toY(double p) => pad + chartH * (1 - (p - lo) / range);

      for (final p in [levels.low, levels.mid, levels.high]) {
        final y = toY(p);
        expect(y, inInclusiveRange(0, size.height),
            reason: 'channel line at $p must be on-screen (y=$y)');
      }
    });

    testWidgets('InteractiveChart paints PlanLevelsPainter when valid',
        (tester) async {
      final series = _tightSeries();
      const levels = PlanChartLevels(
        low: 90,
        mid: 100,
        high: 110,
        valid: true,
        entry: 90.25,
        stop: 89,
        target: 100,
      );
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SizedBox(
              width: 400,
              height: 400,
              child: InteractiveChart(
                series: series,
                config: const ChartIndicatorConfig(),
                planLevels: levels,
                initialVisibleCount: 10,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('plan-levels-overlay')), findsOneWidget);
      final paint = tester.widget<CustomPaint>(
        find.byKey(const Key('plan-levels-overlay')),
      );
      expect(paint.painter, isA<PlanLevelsPainter>());
      final painter = paint.painter! as PlanLevelsPainter;
      expect(painter.levels.low, 90);
      expect(painter.levels.mid, 100);
      expect(painter.levels.high, 110);
      // Y-scale must have expanded to include the channel.
      expect(painter.priceLo, lessThanOrEqualTo(89));
      expect(painter.priceHi, greaterThanOrEqualTo(110));
    });

    testWidgets('InteractiveChart omits overlay when planLevels is null',
        (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SizedBox(
              width: 400,
              height: 400,
              child: InteractiveChart(
                series: _tightSeries(),
                config: const ChartIndicatorConfig(),
                initialVisibleCount: 10,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('plan-levels-overlay')), findsNothing);
    });
  });

  group('PlanPanel', () {
    testWidgets('valid: levels + size recomputes on risk', (tester) async {
      SharedPreferences.setMockInitialValues({});
      final riskCtrl = TextEditingController(text: '100');
      var risk = 100.0;
      var isLong = true;

      Widget build() => MaterialApp(
            home: Scaffold(
              body: PlanPanel(
                data: _validPlan(),
                isLong: isLong,
                risk: risk,
                riskController: riskCtrl,
                onLongChanged: (v) => isLong = v,
                onRiskChanged: (v) => risk = v,
              ),
            ),
          );

      await tester.pumpWidget(build());
      expect(find.byKey(const Key('plan-panel')), findsOneWidget);
      expect(find.text('Entry'), findsOneWidget);
      expect(find.textContaining('Size 80'), findsOneWidget);

      await tester.enterText(find.byKey(const Key('plan-risk-input')), '50');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pumpWidget(build());
      await tester.pump();
      expect(risk, 50);
      expect(find.textContaining('Size 40'), findsOneWidget);
    });

    testWidgets('invalid: reason, muted quality, no levels', (tester) async {
      final riskCtrl = TextEditingController(text: '100');
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: PlanPanel(
              data: _invalidPlan(quality: 0.8, reason: 'channel width'),
              isLong: true,
              risk: 100,
              riskController: riskCtrl,
              onLongChanged: (_) {},
              onRiskChanged: (_) {},
            ),
          ),
        ),
      );
      expect(find.byKey(const Key('plan-reason')), findsOneWidget);
      expect(find.text('channel width'), findsOneWidget);
      expect(find.text('Entry'), findsNothing);
      expect(find.text('Q —'), findsOneWidget);
      final dot = tester.widget<Container>(
        find.byKey(const Key('plan-quality-dot')),
      );
      final deco = dot.decoration! as BoxDecoration;
      expect(deco.color, Colors.white38);
    });

    testWidgets('Long/Short toggle fires callback', (tester) async {
      final riskCtrl = TextEditingController(text: '100');
      var isLong = true;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: StatefulBuilder(
              builder: (context, setState) => PlanPanel(
                data: _validPlan(),
                isLong: isLong,
                risk: 100,
                riskController: riskCtrl,
                onLongChanged: (v) => setState(() => isLong = v),
                onRiskChanged: (_) {},
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.byKey(const Key('plan-short')));
      await tester.pump();
      expect(isLong, isFalse);
    });

    testWidgets('rejects risk 0', (tester) async {
      final riskCtrl = TextEditingController(text: '100');
      var risk = 100.0;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: PlanPanel(
              data: _validPlan(),
              isLong: true,
              risk: risk,
              riskController: riskCtrl,
              onLongChanged: (_) {},
              onRiskChanged: (v) => risk = v,
            ),
          ),
        ),
      );
      await tester.enterText(find.byKey(const Key('plan-risk-input')), '0');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(risk, 100);
      expect(riskCtrl.text, '100');
    });
  });

  group('DetailScreen Plan integration', () {
    late CandleSeriesResponse series;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      series = _tightSeries();
    });

    Widget app({required PlanApi planApi}) {
      return MaterialApp(
        home: DetailScreen(
          symbol: AppSymbol('BTCUSDT'),
          timeframe: Timeframe('1h'),
          series: series,
          planApi: planApi,
          isProUser: true,
          detailContext: const DetailContext(
            rank: 1,
            totalScore: 0.8,
            trendScore: 0.1,
            sidewaysScore: 0.7,
            gainScore: 0.0,
            volume: 1e6,
          ),
        ),
      );
    }

    testWidgets('valid plan shows panel + chart overlay', (tester) async {
      await tester.pumpWidget(app(planApi: _FakePlanApi(valid: true)));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('plan-panel')), findsOneWidget);
      expect(find.byKey(const Key('plan-levels-overlay')), findsOneWidget);
      expect(find.byKey(const Key('plan-disclaimer')), findsOneWidget);
    });

    testWidgets('invalid plan: reason, no chart overlay', (tester) async {
      await tester.pumpWidget(app(planApi: _FakePlanApi(valid: false)));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('plan-panel')), findsOneWidget);
      expect(find.byKey(const Key('plan-reason')), findsOneWidget);
      expect(find.byKey(const Key('plan-levels-overlay')), findsNothing);
    });

    testWidgets('fetch error shows retry, not silent absence', (tester) async {
      await tester.pumpWidget(app(planApi: _FailingPlanApi()));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('plan-error')), findsOneWidget);
      expect(find.byKey(const Key('plan-retry')), findsOneWidget);
      expect(find.byKey(const Key('plan-panel')), findsNothing);
    });

    testWidgets('null planApi shows neither panel nor error', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: DetailScreen(
            symbol: AppSymbol('BTCUSDT'),
            timeframe: Timeframe('1h'),
            series: series,
            isProUser: false,
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('plan-panel')), findsNothing);
      expect(find.byKey(const Key('plan-error')), findsNothing);
    });
  });
}

Map<String, dynamic> _validJson() => {
      'symbol': 'BTCUSDT',
      'timeframe': '1h',
      'low': 100,
      'high': 110,
      'mid': 105,
      'atr': 1,
      'price': 105,
      'longEntry': 100.25,
      'longStop': 99,
      'longTarget': 105,
      'longTargetFull': 109.75,
      'shortEntry': 109.75,
      'shortStop': 111,
      'shortTarget': 105,
      'shortTargetFull': 100.25,
      'riskReward': 3.8,
      'rangeQuality': 0.8,
      'position': 0.5,
      'valid': true,
      'size': 80,
      'shortSize': 80,
    };

PlanData _validPlan() => PlanData.fromJson(_validJson());

PlanData _invalidPlan({required double quality, required String reason}) {
  return PlanData(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    low: 100,
    high: 110,
    mid: 105,
    atr: 1,
    price: 105,
    longEntry: 0,
    longStop: 0,
    longTarget: 0,
    longTargetFull: 0,
    shortEntry: 0,
    shortStop: 0,
    shortTarget: 0,
    shortTargetFull: 0,
    riskReward: 1.0,
    rangeQuality: quality,
    position: 0.5,
    valid: false,
    reason: reason,
  );
}

CandleSeriesResponse _tightSeries() {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: [
      for (var i = 0; i < 30; i++)
        CandleDto(
          timestamp: DateTime.utc(2025, 1, 1).add(Duration(hours: i)),
          open: 104.0,
          high: 106.0,
          low: 103.0,
          close: 105.0,
          volume: 1000.0,
        ),
    ],
  );
}

class _FakePlanApi implements PlanApi {
  final bool valid;
  _FakePlanApi({required this.valid});

  @override
  Future<PlanData> fetch({
    required String symbol,
    required String timeframe,
    double risk = 100,
  }) async {
    if (!valid) {
      return _invalidPlan(quality: 0.8, reason: 'range quality');
    }
    final p = _validPlan();
    return PlanData(
      symbol: p.symbol,
      timeframe: timeframe,
      low: p.low,
      high: p.high,
      mid: p.mid,
      atr: p.atr,
      price: p.price,
      longEntry: p.longEntry,
      longStop: p.longStop,
      longTarget: p.longTarget,
      longTargetFull: p.longTargetFull,
      shortEntry: p.shortEntry,
      shortStop: p.shortStop,
      shortTarget: p.shortTarget,
      shortTargetFull: p.shortTargetFull,
      riskReward: p.riskReward,
      rangeQuality: p.rangeQuality,
      position: p.position,
      valid: true,
      size: PlanData.sizeFor(risk, p.longEntry, p.longStop),
      shortSize: PlanData.sizeFor(risk, p.shortEntry, p.shortStop),
    );
  }
}

class _FailingPlanApi implements PlanApi {
  @override
  Future<PlanData> fetch({
    required String symbol,
    required String timeframe,
    double risk = 100,
  }) async {
    throw HttpPlanApiException('Plan API error: 500');
  }
}
