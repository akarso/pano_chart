import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/market_state/composite_chart_painter.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';
import 'package:pano_chart_frontend/features/market_state/http_composite_index_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_market_state_api.dart';
import 'package:pano_chart_frontend/features/market_state/http_regime_api.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_screen.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/participation_counts.dart';
import 'package:pano_chart_frontend/features/market_state/regime_data.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  group('MarketPulseScreen PR-116', () {
    testWidgets('fetches composite with tapeMetricsWindow limit', (tester) async {
      final compositeApi = _RecordingCompositeApi(_baseComposite(n: 110));
      await tester.pumpWidget(MaterialApp(
        home: MarketPulseScreen(
          marketStateApi: _FakeStateApi(_baseState()),
          compositeIndexApi: compositeApi,
          regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 110)),
        ),
      ));
      await tester.pumpAndSettle();

      expect(compositeApi.lastLimit, tapeMetricsWindow);
      expect(compositeApi.lastLimit, 110);
    });

    testWidgets('card order: headline above composite above participation',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 110)),
        regimeApi: _FakeRegimeApi(_trendRegime(
          windowBars: 110,
          participation: const ParticipationCounts(
            up: 62,
            down: 8,
            ranging: 30,
            total: 100,
          ),
        )),
      ));

      expect(find.text('Market participation'), findsOneWidget);
      final headlineY =
          tester.getTopLeft(find.byKey(const Key('mp-headline'))).dy;
      final compositeY =
          tester.getTopLeft(find.byKey(const Key('mp-composite'))).dy;
      final participationY =
          tester.getTopLeft(find.byKey(const Key('mp-participation'))).dy;

      expect(headlineY, lessThan(compositeY));
      expect(compositeY, lessThan(participationY));
    });

    testWidgets('headline is UPTREND; no structure bars on the card',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 110)),
        regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 110)),
      ));

      expect(find.text('UPTREND'), findsOneWidget);
      expect(find.byKey(const Key('mp-structure-bar-Trend')), findsNothing);
      expect(find.byKey(const Key('mp-structure-bar-Compression')), findsNothing);
      expect(find.text('Compression'), findsNothing);
      expect(find.text('Expansion'), findsNothing);
      expect(find.text('Scored on these 110 bars'), findsOneWidget);
      expect(find.text('Avg trend mix'), findsOneWidget);
    });

    testWidgets('caption uses chart length when shorter than windowBars',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 3)),
        regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 48)),
      ));

      expect(find.text('Scored on 48 bars; chart shows 3'), findsOneWidget);
      expect(find.textContaining('last 48 bars shown'), findsNothing);
    });

    testWidgets('VW source without VW series is not treated as the tape',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(
          n: 110,
          withVolumeWeighted: false,
        )),
        regimeApi: _FakeRegimeApi(_trendRegime(
          regimeSource: 'composite_volume_weighted',
          windowBars: 110,
        )),
      ));

      expect(
        find.text('Headline scored on the tape; chart shows another series'),
        findsOneWidget,
      );
      expect(find.textContaining('Scored on these'), findsNothing);
      final paint = tester.widget<CustomPaint>(
        find.byKey(const Key('mp-composite-paint')),
      );
      final painter = paint.painter! as CompositeChartPainter;
      expect(painter.windowBars, 0);
      expect(painter.solidRegression, isFalse);
    });

    testWidgets('Median chip: mismatch caption, no scored-window OLS',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 110)),
        regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 110)),
      ));

      expect(find.text('Scored on these 110 bars'), findsOneWidget);

      await tester.tap(find.text('Median'));
      await tester.pumpAndSettle();

      expect(
        find.text('Headline scored on the tape; chart shows another series'),
        findsOneWidget,
      );
      expect(find.textContaining('Scored on these'), findsNothing);
      expect(
        find.textContaining('Showing a different series than the headline tape'),
        findsOneWidget,
      );
      expect(find.text('series change'), findsOneWidget);

      final paint = tester.widget<CustomPaint>(
        find.byKey(const Key('mp-composite-paint')),
      );
      final painter = paint.painter! as CompositeChartPainter;
      expect(painter.windowBars, 0);
    });

    testWidgets('Median chip survives refresh', (tester) async {
      final compositeApi = _RecordingCompositeApi(_baseComposite(n: 110));
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: compositeApi,
        regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 110)),
      ));

      expect(find.text('Scored on these 110 bars'), findsOneWidget);
      final fetchesAfterLoad = compositeApi.fetchCount;

      await tester.tap(find.text('Median'));
      await tester.pumpAndSettle();

      expect(find.text('series change'), findsOneWidget);
      expect(find.textContaining('Scored on these'), findsNothing);
      expect(
        (tester.widget<CustomPaint>(
          find.byKey(const Key('mp-composite-paint')),
        ).painter! as CompositeChartPainter)
            .windowBars,
        0,
      );

      // Timeframe change reloads (same path as pull-to-refresh / auto-refresh).
      await tester.tap(find.byType(DropdownButton<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('1h').last);
      await tester.pumpAndSettle();

      expect(compositeApi.fetchCount, greaterThan(fetchesAfterLoad));
      expect(find.text('series change'), findsOneWidget);
      expect(find.textContaining('Scored on these'), findsNothing);
      expect(
        find.textContaining('Showing a different series than the headline tape'),
        findsOneWidget,
      );
      expect(
        (tester.widget<CustomPaint>(
          find.byKey(const Key('mp-composite-paint')),
        ).painter! as CompositeChartPainter)
            .windowBars,
        0,
      );
    });

    testWidgets('state-only (no regimeSource) uses window change, not series mismatch',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 20)),
      ));

      expect(find.text('series change'), findsNothing);
      expect(find.text('window change'), findsOneWidget);
      expect(
        find.textContaining('Showing a different series than the headline tape'),
        findsNothing,
      );
    });

    testWidgets('participation fallback subline', (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 20)),
        regimeApi: _FakeRegimeApi(_trendRegime(
          regimeSource: 'participation',
          windowBars: 0,
        )),
      ));

      expect(
        find.text('From token participation (tape unavailable)'),
        findsOneWidget,
      );
    });

    testWidgets('participation reading: broad with tape', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 62, down: 8, ranging: 30, total: 100),
      );
      expect(
        find.text('Broad move — most tokens trend with the tape'),
        findsOneWidget,
      );
      expect(find.textContaining('Up 62%'), findsOneWidget);
    });

    testWidgets('participation reading: against downtape', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 62, down: 8, ranging: 30, total: 100),
        bias: 'down',
      );
      expect(find.text('Broad move against the tape'), findsOneWidget);
    });

    testWidgets('participation reading: mixed against the tape', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 40, down: 20, ranging: 40, total: 100),
        bias: 'down',
      );
      expect(find.text('Mixed — tokens lean against the tape'), findsOneWidget);
    });

    testWidgets('participation reading: narrow against the tape', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 20, down: 15, ranging: 65, total: 100),
        bias: 'down',
      );
      expect(
        find.text('Narrow — a few tokens lean against the tape'),
        findsOneWidget,
      );
    });

    testWidgets('participation reading: narrow', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 20, down: 15, ranging: 65, total: 100),
      );
      expect(
        find.text('Narrow — tape led by a few heavyweights'),
        findsOneWidget,
      );
    });

    testWidgets('participation reading: split', (tester) async {
      await _pumpParticipation(
        tester,
        const ParticipationCounts(up: 35, down: 35, ranging: 30, total: 100),
      );
      expect(find.text('Split market'), findsOneWidget);
    });

    testWidgets('empty participation is not mounted', (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 20)),
        regimeApi: _FakeRegimeApi(_trendRegime(
          participation: ParticipationCounts.empty,
        )),
      ));

      expect(find.byKey(const Key('mp-participation')), findsNothing);
      expect(find.text('Market participation'), findsNothing);
    });

    testWidgets('structure mix lives only in the regime info dialog',
        (tester) async {
      await _pumpTall(tester, home: MarketPulseScreen(
        marketStateApi: _FakeStateApi(_baseState()),
        compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 110)),
        regimeApi: _FakeRegimeApi(_trendRegime(windowBars: 110)),
      ));

      expect(find.text('Structure mix'), findsNothing);
      expect(find.byKey(const Key('mp-structure-bar-Trend')), findsNothing);

      await tester.tap(find.byKey(const Key('mp-regime-help')));
      await tester.pumpAndSettle();

      expect(find.text('Structure mix'), findsOneWidget);
      expect(find.byKey(const Key('mp-structure-bar-Trend')), findsOneWidget);
      expect(find.byKey(const Key('mp-structure-bar-Sideways')), findsOneWidget);
      expect(find.byKey(const Key('mp-structure-bar-Compression')), findsOneWidget);
      expect(find.byKey(const Key('mp-structure-bar-Expansion')), findsOneWidget);
      expect(find.text('Compression'), findsOneWidget);
      expect(find.text('Expansion'), findsOneWidget);
    });
  });
}

Future<void> _pumpTall(WidgetTester tester, {required Widget home}) async {
  tester.view.physicalSize = const Size(800, 2000);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(home: home));
  await tester.pumpAndSettle();
}

Future<void> _pumpParticipation(
  WidgetTester tester,
  ParticipationCounts participation, {
  String bias = 'up',
}) async {
  await _pumpTall(
    tester,
    home: MarketPulseScreen(
      key: UniqueKey(),
      marketStateApi: _FakeStateApi(_baseState()),
      compositeIndexApi: _FakeCompositeApi(_baseComposite(n: 110)),
      regimeApi: _FakeRegimeApi(_trendRegime(
        bias: bias,
        windowBars: 110,
        participation: participation,
      )),
    ),
  );
}

MarketStateData _baseState() => const MarketStateData(
      timeframe: '4h',
      state: 'sideways',
      confidence: 0.4,
      breadth: MarketBreadth(
        sideways: 0.4,
        compression: 0.2,
        expansion: 0.2,
        trend: 0.2,
      ),
      symbolCount: 100,
    );

CompositeIndexData _baseComposite({
  int n = 110,
  bool withVolumeWeighted = true,
}) {
  final points = List.generate(
    n,
    (i) => IndexPoint(timestamp: 1000 + i * 100, value: 100.0 + i * 0.1),
  );
  final vw = withVolumeWeighted
      ? List.generate(
          n,
          (i) => IndexPoint(
            timestamp: 1000 + i * 100,
            value: 100.0 + i * 0.15,
          ),
        )
      : const <IndexPoint>[];
  return CompositeIndexData(
    timeframe: '4h',
    symbolCount: 80,
    points: points,
    volumeWeightedPoints: vw,
  );
}

RegimeData _trendRegime({
  String bias = 'up',
  String regimeSource = 'composite_volume_weighted',
  int windowBars = 110,
  ParticipationCounts participation = const ParticipationCounts(
    up: 50,
    down: 20,
    ranging: 30,
    total: 100,
  ),
}) =>
    RegimeData(
      timeframe: '4h',
      regime: 'trend',
      prevalence: 0.72,
      bias: bias,
      regimeSource: regimeSource,
      windowBars: windowBars,
      trendScore: 0.72,
      scores: const RegimeScores(
        expansion: 0.05,
        compression: 0.08,
        trend: 0.72,
        sideways: 0.15,
      ),
      metrics: const RegimeMetrics(
        trendBreadth: 0.5,
        sidewaysBreadth: 0.2,
        expansionBreadth: 0.1,
        compressionBreadth: 0.2,
        volatilityExpansion: 1.0,
        dispersion: 0.03,
      ),
      participation: participation,
    );

class _FakeStateApi implements MarketStateApi {
  _FakeStateApi(this.data);
  final MarketStateData data;

  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) async => data;
}

class _FakeCompositeApi implements CompositeIndexApi {
  _FakeCompositeApi(this.data);
  final CompositeIndexData data;

  @override
  Future<CompositeIndexData> fetch({
    String timeframe = '4h',
    int limit = tapeMetricsWindow,
  }) async =>
      data;
}

class _RecordingCompositeApi implements CompositeIndexApi {
  _RecordingCompositeApi(this.data);
  final CompositeIndexData data;
  int? lastLimit;
  int fetchCount = 0;

  @override
  Future<CompositeIndexData> fetch({
    String timeframe = '4h',
    int limit = tapeMetricsWindow,
  }) async {
    fetchCount++;
    lastLimit = limit;
    return data;
  }
}

class _FakeRegimeApi implements RegimeApi {
  _FakeRegimeApi(this.data);
  final RegimeData data;

  @override
  Future<RegimeData> fetch({String timeframe = '4h'}) async => data;
}
