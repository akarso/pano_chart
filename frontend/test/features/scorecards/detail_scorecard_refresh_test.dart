import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:pano_chart_frontend/features/detail/behavior_data.dart';
import 'package:pano_chart_frontend/features/detail/fragility_data.dart';
import 'package:pano_chart_frontend/features/detail/http_behavior_api.dart';
import 'package:pano_chart_frontend/features/detail/http_fragility_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/detail/http_setup_api.dart';
import 'package:pano_chart_frontend/features/detail/setup_data.dart';
import 'package:pano_chart_frontend/features/detail/chart/interactive_chart.dart';
import 'package:pano_chart_frontend/features/scorecards/http_scorecard_api.dart';
import 'package:pano_chart_frontend/features/scorecards/reliability_chip.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';
import 'package:pano_chart_frontend/features/volatility/http_volatility_api.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';

class _Candles implements GetCandleSeries {
  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    return _series();
  }
}

class _Setup implements SetupApi {
  @override
  Future<SetupData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) async {
    return SetupData(
      symbol: symbol,
      timeframe: timeframe,
      bestSetup: 'compression_breakout',
      score: 0.7,
      scores: const {'compression_breakout': 0.7},
      trendHealth: 0.8,
      regime: 'uptrend',
      marketEffective: 0.5,
      confidence: 0.8,
      breakoutUp: 0.6,
      breakoutDown: 0.1,
    );
  }
}

class _Scorecards implements ScorecardApi {
  int summaries = 0;

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) async {
    summaries++;
    return ScorecardSummary(
      timeframe: timeframe,
      since: '',
      sinceRaw: since,
      items: const [],
    );
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) {
    throw UnimplementedError();
  }
}

CandleSeriesResponse _series() {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1m',
    candles: [
      CandleDto(
        timestamp: DateTime.utc(2026, 9, 22, 12),
        open: 1,
        high: 2,
        low: 1,
        close: 2,
        volume: 1,
      ),
    ],
  );
}

void main() {
  testWidgets('chart refresh does not refetch the summary', (tester) async {
    final scorecards = _Scorecards();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1m'),
          series: _series(),
          isProUser: true,
          getCandleSeries: _Candles(),
          setupApi: _Setup(),
          scorecardApi: scorecards,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(scorecards.summaries, 1);

    await tester.pump(const Duration(seconds: 11));
    await tester.pump();
    await tester.pump();
    expect(scorecards.summaries, 1);
  });

  testWidgets('detail without a setup api does not fetch a summary', (
    tester,
  ) async {
    final scorecards = _Scorecards();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          scorecardApi: scorecards,
        ),
      ),
    );
    await tester.pump();
    expect(scorecards.summaries, 0);
  });

  testWidgets('a failed 4h setup hides the 1h card', (tester) async {
    final setup = _GateSetup();
    await _pumpDetail(tester, setup);
    setup.succeed(
      '1h',
      _setup(timeframe: '1h', bestSetup: 'compression_breakout'),
    );
    await tester.pump();
    expect(find.text('breakout_up'), findsOneWidget);

    await _switchTo(tester, '4h');
    expect(find.text('breakout_up'), findsNothing);
    expect(find.text('Setup Quality'), findsNothing);

    setup.fail('4h');
    await tester.pump();
    expect(find.text('breakout_up'), findsNothing);
    expect(find.text('Setup Quality'), findsNothing);
  });

  testWidgets('a late 1h setup does not replace the 4h card', (tester) async {
    final setup = _GateSetup();
    await _pumpDetail(tester, setup);
    await _switchTo(tester, '4h');
    setup.succeed(
      '1h',
      _setup(timeframe: '1h', bestSetup: 'compression_breakout'),
    );
    await tester.pump();
    expect(find.text('breakout_up'), findsNothing);

    setup.succeed(
      '4h',
      _setup(
        timeframe: '4h',
        bestSetup: 'trend_continuation',
        regime: 'downtrend',
      ),
    );
    await tester.pump();
    expect(find.text('trend_down'), findsOneWidget);
    expect(find.text('breakout_up'), findsNothing);
  });

  testWidgets('setup title fits a phone width', (tester) async {
    const item = ScorecardSummaryItem(
      kind: 'setup',
      label: 'breakout_up',
      hitRate: 0.58,
      baseline: 0.4,
      n: 412,
    );
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: SizedBox(
            width: 320,
            child: Padding(
              padding: EdgeInsets.symmetric(horizontal: 16),
              child: FieldsetHeader(
                title: Text(
                  'Setup Quality — 72% · ●',
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
                hint: Icon(Icons.help_outline, size: 13),
                trailing: _SetupTrailing(item: item),
              ),
            ),
          ),
        ),
      ),
    );
    expect(tester.takeException(), isNull);
    expect(
      tester.getSize(find.text('Setup Quality — 72% · ●')).width,
      greaterThan(0),
    );
    final labelFinder = find.text('breakout_up');
    final trailing = find
        .ancestor(of: labelFinder, matching: find.byType(Row))
        .first;
    final row = tester.renderObject<RenderFlex>(trailing);
    final label = tester.getSize(
      find.ancestor(of: labelFinder, matching: find.byType(FittedBox)).first,
    );
    final gap = tester.getSize(
      find.descendant(
        of: trailing,
        matching: find.byWidgetPredicate(
          (widget) => widget is SizedBox && widget.width == 6,
        ),
      ),
    );
    final chip = tester.getSize(
      find.byKey(const ValueKey('reliability-chip-setup|breakout_up')),
    );
    expect(chip.width, greaterThanOrEqualTo(32));
    expect(chip.height, greaterThanOrEqualTo(24));
    expect(
      label.width + gap.width + chip.width,
      lessThanOrEqualTo(row.size.width + 0.01),
    );
    expect(row.size.width, lessThanOrEqualTo(row.constraints.maxWidth + 0.01));
  });

  testWidgets('a short header scales the label and keeps the chip', (
    tester,
  ) async {
    const item = ScorecardSummaryItem(
      kind: 'setup',
      label: 'breakout_up',
      hitRate: 0.58,
      baseline: 0.4,
      n: 412,
    );
    Widget header(double width) {
      return MaterialApp(
        home: Scaffold(
          body: SizedBox(
            width: width,
            child: const FieldsetHeader(
              title: Text('Setup'),
              trailing: _SetupTrailing(item: item),
            ),
          ),
        ),
      );
    }

    await tester.pumpWidget(header(800));
    final labelFinder = find.text('breakout_up');
    final titleFinder = find.text('Setup');
    final trailing = find
        .ancestor(of: labelFinder, matching: find.byType(Row))
        .first;
    final gap = tester.getSize(
      find.descendant(
        of: trailing,
        matching: find.byWidgetPredicate(
          (widget) => widget is SizedBox && widget.width == 6,
        ),
      ),
    );
    final naturalLabel = tester.getSize(
      find.ancestor(of: labelFinder, matching: find.byType(FittedBox)).first,
    );
    final naturalChip = tester.getSize(
      find.byKey(const ValueKey('reliability-chip-setup|breakout_up')),
    );
    final trailingWidth = tester.getSize(trailing).width;
    final titleWidth = tester
        .renderObject<RenderParagraph>(titleFinder)
        .getMaxIntrinsicWidth(double.infinity);
    final separator =
        tester.getRect(trailing).left - tester.getRect(titleFinder).right;
    expect(gap.width, 6);
    expect(separator, 8);
    expect(trailingWidth, naturalLabel.width + gap.width + naturalChip.width);

    await tester.pumpWidget(header(titleWidth + separator + trailingWidth - 1));
    expect(tester.getSize(trailing).width, closeTo(trailingWidth - 1, 0.01));
    expect(tester.getSize(titleFinder).width, greaterThanOrEqualTo(titleWidth));
    expect(tester.takeException(), isNull);
    final scaled = tester.getSize(
      find.ancestor(of: labelFinder, matching: find.byType(FittedBox)).first,
    );
    expect(scaled.width, closeTo(naturalLabel.width - 1, 0.01));
    final chip = tester.getSize(
      find.byKey(const ValueKey('reliability-chip-setup|breakout_up')),
    );
    expect(chip.width, greaterThanOrEqualTo(32));
    expect(chip.height, greaterThanOrEqualTo(24));
    expect(chip.width, naturalChip.width);
    expect(chip.height, naturalChip.height);
  });

  testWidgets('reload retries a failed setup fetch', (tester) async {
    final setup = _GateSetup();
    await _pumpDetail(tester, setup);
    setup.fail('1h');
    await tester.pump();
    expect(find.text('Setup Quality'), findsNothing);

    await tester.tap(find.byTooltip('Reload chart'));
    await tester.pump();
    await tester.pump();
    setup.succeed(
      '1h',
      _setup(timeframe: '1h', bestSetup: 'compression_breakout'),
    );
    await tester.pump();
    expect(find.text('breakout_up'), findsOneWidget);
  });

  testWidgets('a failed refresh of the current timeframe keeps the card', (
    tester,
  ) async {
    final setup = _GateSetup();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1m'),
          series: _series(),
          isProUser: true,
          getCandleSeries: _Candles(),
          setupApi: setup,
          scorecardApi: _Scorecards(),
        ),
      ),
    );
    await tester.pump();
    setup.succeed(
      '1m',
      _setup(timeframe: '1m', bestSetup: 'compression_breakout'),
    );
    await tester.pump();
    expect(find.text('breakout_up'), findsOneWidget);

    await tester.pump(const Duration(seconds: 11));
    await tester.pump();
    setup.fail('1m');
    await tester.pump();
    expect(find.text('breakout_up'), findsOneWidget);
  });

  testWidgets('a late chart refresh does not restore the previous candles', (
    tester,
  ) async {
    final candles = _GateCandles();
    final setup = _GateSetup();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1m'),
          series: _priced(2),
          isProUser: true,
          getCandleSeries: candles,
          setupApi: setup,
          scorecardApi: _Scorecards(),
        ),
      ),
    );
    await tester.pump();
    setup.succeed(
      '1m',
      _setup(timeframe: '1m', bestSetup: 'compression_breakout'),
    );
    await tester.pump();

    await tester.pump(const Duration(seconds: 11));
    await tester.pump();
    expect(candles.pending['1m'], isNotEmpty);

    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    await tester.tap(find.text('4h').last);
    await tester.pump();
    candles.complete('4h', _priced(50, timeframe: '4h'));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('4h candles'), findsOneWidget);
    final setupCalls = setup.calls['4h'];

    candles.complete('1m', _priced(99));
    await tester.pump();
    expect(find.textContaining('4h candles'), findsOneWidget);
    expect(find.textContaining('1m candles'), findsNothing);
    expect(setup.calls['4h'], setupCalls);
  });

  testWidgets('a late panel response does not replace the new timeframe', (
    tester,
  ) async {
    final fragility = _GateFragility();
    final behavior = _GateBehavior();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          getCandleSeries: _Candles(),
          fragilityApi: fragility,
          behaviorApi: behavior,
        ),
      ),
    );
    await tester.pump();
    await _switchTo(tester, '4h');
    fragility.succeed('4h', _fragility('4h', 'low'));
    behavior.succeed('4h', _behavior('4h', 'steady'));
    await tester.pump();
    expect(find.text('Low Risk'), findsOneWidget);
    expect(find.text('steady'), findsOneWidget);

    fragility.succeed('1h', _fragility('1h', 'high'));
    behavior.succeed('1h', _behavior('1h', 'stale'));
    await tester.pump();
    expect(find.text('Low Risk'), findsOneWidget);
    expect(find.text('High Risk'), findsNothing);
    expect(find.text('steady'), findsOneWidget);
    expect(find.text('stale'), findsNothing);
  });

  testWidgets('a timeframe change clears fragility, behavior, and volatility', (
    tester,
  ) async {
    final fragility = _GateFragility();
    final behavior = _GateBehavior();
    final volatility = _GateVolatility();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          getCandleSeries: _Candles(),
          fragilityApi: fragility,
          behaviorApi: behavior,
          volatilityApi: volatility,
        ),
      ),
    );
    await tester.pump();
    fragility.succeed('1h', _fragility('1h', 'high'));
    behavior.succeed('1h', _behavior('1h', 'steady'));
    volatility.succeed('1h', const [
      VolatilityBucket(minute: 720, normalized: 1, spikeProb: 0.8),
    ]);
    await tester.pump();
    expect(find.text('High Risk'), findsOneWidget);
    expect(find.text('steady'), findsOneWidget);
    expect(
      tester
          .widget<InteractiveChart>(find.byType(InteractiveChart))
          .volatilityAligned,
      isNotNull,
    );

    await _switchTo(tester, '4h');
    expect(find.text('High Risk'), findsNothing);
    expect(find.text('steady'), findsNothing);
    expect(
      tester
          .widget<InteractiveChart>(find.byType(InteractiveChart))
          .volatilityAligned,
      isNull,
    );

    fragility.fail('4h');
    behavior.fail('4h');
    volatility.fail('4h');
    await tester.pump();
    expect(find.text('High Risk'), findsNothing);
    expect(find.text('steady'), findsNothing);
    expect(
      tester
          .widget<InteractiveChart>(find.byType(InteractiveChart))
          .volatilityAligned,
      isNull,
    );

    await tester.tap(find.byTooltip('Reload chart'));
    await tester.pump();
    await tester.pump();
    expect(fragility.calls['4h'], 2);
    expect(behavior.calls['4h'], 2);
    expect(volatility.calls['4h'], 2);
    fragility.succeed('4h', _fragility('4h', 'low'));
    behavior.succeed('4h', _behavior('4h', 'calm'));
    volatility.succeed('4h', const [
      VolatilityBucket(minute: 720, normalized: 1, spikeProb: 0.2),
    ]);
    await tester.pump();
    expect(find.text('Low Risk'), findsOneWidget);
    expect(find.text('calm'), findsOneWidget);
    expect(find.text('High Risk'), findsNothing);
    expect(
      tester
          .widget<InteractiveChart>(find.byType(InteractiveChart))
          .volatilityAligned,
      isNotNull,
    );
  });

  testWidgets('a panel body for another timeframe is ignored', (tester) async {
    final fragility = _GateFragility();
    final behavior = _GateBehavior();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          fragilityApi: fragility,
          behaviorApi: behavior,
        ),
      ),
    );
    await tester.pump();
    fragility.succeed('1h', _fragility('4h', 'high'));
    behavior.succeed('1h', _behavior('4h', 'steady'));
    await tester.pump();
    expect(find.text('High Risk'), findsNothing);
    expect(find.text('steady'), findsNothing);
  });

  testWidgets('a wrong timeframe tag keeps the panel already on screen', (
    tester,
  ) async {
    final fragility = _GateFragility();
    final behavior = _GateBehavior();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          getCandleSeries: _Candles(),
          fragilityApi: fragility,
          behaviorApi: behavior,
        ),
      ),
    );
    await tester.pump();
    fragility.succeed('1h', _fragility('1h', 'high'));
    behavior.succeed('1h', _behavior('1h', 'steady'));
    await tester.pump();

    await tester.tap(find.byTooltip('Reload chart'));
    await tester.pump();
    await tester.pump();
    fragility.succeed('1h', _fragility('4h', 'medium'));
    behavior.succeed('1h', _behavior('4h', 'other'));
    await tester.pump();
    expect(find.text('High Risk'), findsOneWidget);
    expect(find.text('steady'), findsOneWidget);
    expect(find.text('Medium Risk'), findsNothing);
    expect(find.text('other'), findsNothing);

    await tester.tap(find.byTooltip('Reload chart'));
    await tester.pump();
    await tester.pump();
    fragility.succeed('1h', _fragility('1h', 'low'));
    behavior.succeed('1h', _behavior('1h', 'calm'));
    await tester.pump();
    expect(find.text('Low Risk'), findsOneWidget);
    expect(find.text('calm'), findsOneWidget);
  });

  testWidgets('reload retries a failed summary', (tester) async {
    final setup = _GateSetup();
    final scorecards = _GateScorecards();
    await tester.pumpWidget(
      MaterialApp(
        home: DetailScreen(
          symbol: const AppSymbol('BTCUSDT'),
          timeframe: const Timeframe('1h'),
          series: _series(),
          getCandleSeries: _Candles(),
          setupApi: setup,
          scorecardApi: scorecards,
        ),
      ),
    );
    await tester.pump();
    setup.succeed(
      '1h',
      _setup(timeframe: '1h', bestSetup: 'compression_breakout'),
    );
    scorecards.fail();
    await tester.pump();
    expect(find.text('breakout_up'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('reliability-chip-setup|breakout_up')),
      findsNothing,
    );

    await tester.tap(find.byTooltip('Reload chart'));
    await tester.pump();
    await tester.pump();
    setup.succeed(
      '1h',
      _setup(timeframe: '1h', bestSetup: 'compression_breakout'),
    );
    scorecards.succeed(
      const ScorecardSummary(
        timeframe: '1h',
        since: '',
        sinceRaw: '30d',
        items: [
          ScorecardSummaryItem(
            kind: 'setup',
            label: 'breakout_up',
            hitRate: 0.58,
            baseline: 0.4,
            n: 412,
          ),
        ],
      ),
    );
    await tester.pump();
    expect(
      find.byKey(const ValueKey('reliability-chip-setup|breakout_up')),
      findsOneWidget,
    );
  });
}

class _SetupTrailing extends StatelessWidget {
  final ScorecardSummaryItem item;

  const _SetupTrailing({required this.item});

  @override
  Widget build(BuildContext context) {
    return setupSignalTrailing(
      label: 'breakout_up',
      chip: ReliabilityChip(item: item),
    );
  }
}

Future<void> _pumpDetail(WidgetTester tester, SetupApi setup) async {
  await tester.pumpWidget(
    MaterialApp(
      home: DetailScreen(
        symbol: const AppSymbol('BTCUSDT'),
        timeframe: const Timeframe('1h'),
        series: _series(),
        getCandleSeries: _Candles(),
        setupApi: setup,
        scorecardApi: _Scorecards(),
      ),
    ),
  );
  await tester.pump();
}

Future<void> _switchTo(WidgetTester tester, String timeframe) async {
  await tester.tap(find.byType(DropdownButton<String>));
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 300));
  await tester.tap(find.text(timeframe).last);
  await tester.pump();
  await tester.pump();
}

class _GateSetup implements SetupApi {
  final Map<String, Completer<SetupData>> _pending = {};
  final Map<String, int> calls = {};

  @override
  Future<SetupData> fetch({required String symbol, String timeframe = '4h'}) {
    calls[timeframe] = (calls[timeframe] ?? 0) + 1;
    final completer = Completer<SetupData>();
    _pending[timeframe] = completer;
    return completer.future;
  }

  void succeed(String timeframe, SetupData data) {
    _pending[timeframe]!.complete(data);
  }

  void fail(String timeframe) {
    _pending[timeframe]!.completeError(Exception('down'));
  }
}

class _SetupChip implements ScorecardApi {
  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) async {
    return ScorecardSummary(
      timeframe: timeframe,
      since: '',
      sinceRaw: since,
      items: const [
        ScorecardSummaryItem(
          kind: 'setup',
          label: 'breakout_up',
          hitRate: 0.58,
          baseline: 0.4,
          n: 412,
        ),
      ],
    );
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) {
    throw UnimplementedError();
  }
}

SetupData _setup({
  required String timeframe,
  required String bestSetup,
  String regime = 'uptrend',
  double breakoutUp = 0.6,
  double breakoutDown = 0.1,
}) {
  return SetupData(
    symbol: 'BTCUSDT',
    timeframe: timeframe,
    bestSetup: bestSetup,
    score: 0.72,
    scores: {bestSetup: 0.72},
    trendHealth: 0.8,
    regime: regime,
    marketEffective: 0.5,
    confidence: 0.8,
    breakoutUp: breakoutUp,
    breakoutDown: breakoutDown,
  );
}

CandleSeriesResponse _priced(double close, {String timeframe = '1m'}) {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: timeframe,
    candles: [
      CandleDto(
        timestamp: DateTime.utc(2026, 9, 22, 12),
        open: close,
        high: close,
        low: close,
        close: close,
        volume: 1,
      ),
    ],
  );
}

class _GateCandles implements GetCandleSeries {
  final Map<String, List<Completer<CandleSeriesResponse>>> pending = {};

  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) {
    final completer = Completer<CandleSeriesResponse>();
    pending.putIfAbsent(input.timeframe, () => []).add(completer);
    return completer.future;
  }

  void complete(String timeframe, CandleSeriesResponse series) {
    pending[timeframe]!.removeAt(0).complete(series);
  }
}

class _GateFragility implements FragilityApi {
  final Map<String, Completer<FragilityData>> _pending = {};
  final Map<String, int> calls = {};

  @override
  Future<FragilityData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) {
    calls[timeframe] = (calls[timeframe] ?? 0) + 1;
    final completer = Completer<FragilityData>();
    _pending[timeframe] = completer;
    return completer.future;
  }

  void succeed(String timeframe, FragilityData data) {
    _pending[timeframe]!.complete(data);
  }

  void fail(String timeframe) {
    _pending[timeframe]!.completeError(Exception('down'));
  }
}

class _GateBehavior implements BehaviorApi {
  final Map<String, Completer<BehaviorData>> _pending = {};
  final Map<String, int> calls = {};

  @override
  Future<BehaviorData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) {
    calls[timeframe] = (calls[timeframe] ?? 0) + 1;
    final completer = Completer<BehaviorData>();
    _pending[timeframe] = completer;
    return completer.future;
  }

  void succeed(String timeframe, BehaviorData data) {
    _pending[timeframe]!.complete(data);
  }

  void fail(String timeframe) {
    _pending[timeframe]!.completeError(Exception('down'));
  }
}

class _GateVolatility implements VolatilityApi {
  final Map<String, Completer<List<VolatilityBucket>>> _pending = {};
  final Map<String, int> calls = {};

  @override
  Future<List<VolatilityBucket>> fetch({String timeframe = '1m'}) {
    calls[timeframe] = (calls[timeframe] ?? 0) + 1;
    final completer = Completer<List<VolatilityBucket>>();
    _pending[timeframe] = completer;
    return completer.future;
  }

  void succeed(String timeframe, List<VolatilityBucket> data) {
    _pending[timeframe]!.complete(data);
  }

  void fail(String timeframe) {
    _pending[timeframe]!.completeError(Exception('down'));
  }
}

class _GateScorecards implements ScorecardApi {
  final List<Completer<ScorecardSummary>> _pending = [];

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) {
    final completer = Completer<ScorecardSummary>();
    _pending.add(completer);
    return completer.future;
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) {
    throw UnimplementedError();
  }

  void fail() {
    _pending.removeAt(0).completeError(Exception('down'));
  }

  void succeed(ScorecardSummary summary) {
    _pending.removeAt(0).complete(summary);
  }
}

FragilityData _fragility(String timeframe, String risk) {
  return FragilityData(
    symbol: 'BTCUSDT',
    timeframe: timeframe,
    fragilityScore: 0.2,
    riskLevel: risk,
    dominantSide: 'neutral',
    squeezeRisk: 'none',
    components: const FragilityComponents(
      fundingExtremeness: 0,
      oiExpansion: 0,
      longShortImbalance: 0,
      liquidationProximity: 0,
    ),
  );
}

BehaviorData _behavior(String timeframe, String summary) {
  return BehaviorData(
    symbol: 'BTCUSDT',
    timeframe: timeframe,
    greed: 0.1,
    fear: 0.1,
    patience: 0.1,
    panic: 0.1,
    summary: summary,
  );
}
