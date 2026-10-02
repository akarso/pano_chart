import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series.dart';
import 'package:pano_chart_frontend/features/candles/application/get_candle_series_input.dart';
import 'package:pano_chart_frontend/features/detail/detail_context.dart';
import 'package:pano_chart_frontend/features/detail/detail_screen.dart';
import 'package:pano_chart_frontend/features/detail/http_mtf_regimes_api.dart';
import 'package:pano_chart_frontend/features/detail/mtf_regimes_data.dart';
import 'package:pano_chart_frontend/features/market_state/regime_colors.dart';

class _FakeMtfRegimesApi implements MtfRegimesApi {
  final MtfRegimesData? data;
  final Object? error;

  _FakeMtfRegimesApi({this.data, this.error});

  @override
  Future<MtfRegimesData> fetch({required String symbol}) async {
    if (error != null) throw error!;
    return data!;
  }
}

/// Fails on its first call, then always succeeds — used to prove a failed
/// MTF fetch can be retried (PR-100 CR).
class _FlakyMtfRegimesApi implements MtfRegimesApi {
  final MtfRegimesData success;
  int calls = 0;

  _FlakyMtfRegimesApi(this.success);

  @override
  Future<MtfRegimesData> fetch({required String symbol}) async {
    calls++;
    if (calls == 1) throw Exception('transient network error');
    return success;
  }
}

/// Always succeeds — used to prove a successful fetch is not redundantly
/// re-fetched (PR-100 CR).
class _CountingMtfRegimesApi implements MtfRegimesApi {
  final MtfRegimesData success;
  int calls = 0;

  _CountingMtfRegimesApi(this.success);

  @override
  Future<MtfRegimesData> fetch({required String symbol}) async {
    calls++;
    return success;
  }
}

/// Hands back a fresh, manually-resolved [Completer] for every call — used
/// to prove that an overlapping, later call failing first does not discard
/// an earlier call's success (PR-100 CR).
class _RacingMtfRegimesApi implements MtfRegimesApi {
  final completers = <Completer<MtfRegimesData>>[];

  @override
  Future<MtfRegimesData> fetch({required String symbol}) {
    final c = Completer<MtfRegimesData>();
    completers.add(c);
    return c.future;
  }
}

class _FakeGetCandleSeries implements GetCandleSeries {
  @override
  Future<CandleSeriesResponse> execute(GetCandleSeriesInput input) async {
    return CandleSeriesResponse(
      symbol: input.symbol,
      timeframe: input.timeframe,
      candles: [
        CandleDto(
          timestamp: DateTime.utc(2025, 1, 1),
          open: 100.0,
          high: 105.0,
          low: 95.0,
          close: 102.0,
          volume: 1000.0,
        ),
      ],
    );
  }
}

DetailContext _fakeContext() => const DetailContext(
      rank: 3,
      totalScore: 0.78,
      trendScore: 0.12,
      sidewaysScore: 0.82,
      gainScore: 0.04,
      volume: 18400000,
    );

CandleSeriesResponse _fakeSeries() {
  return CandleSeriesResponse(
    symbol: 'BTCUSDT',
    timeframe: '1h',
    candles: [
      CandleDto(
        timestamp: DateTime.utc(2025, 1, 1),
        open: 100.0,
        high: 105.0,
        low: 95.0,
        close: 102.0,
        volume: 1000.0,
      ),
    ],
  );
}

Widget _app({MtfRegimesApi? mtfRegimesApi, GetCandleSeries? getCandleSeries}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('ETHUSDT'),
      timeframe: const Timeframe('1h'),
      series: _fakeSeries(),
      detailContext: _fakeContext(),
      mtfRegimesApi: mtfRegimesApi,
      getCandleSeries: getCandleSeries,
    ),
  );
}

void main() {
  group('DetailScreen MTF strip (PR-100)', () {
    testWidgets('renders all four pills, colored by dominant regime',
        (tester) async {
      final api = _FakeMtfRegimesApi(
        data: const MtfRegimesData(
          symbol: 'ETHUSDT',
          frames: [
            MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
            MtfFrame(
                timeframe: '1h',
                dominant: 'compression',
                bias: 'neutral',
                score: 0.5),
          ],
          alignment: 0.5,
          alignedState: 'indecisive',
        ),
      );

      await tester.pumpWidget(_app(mtfRegimesApi: api));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('mtf-pill-15m')), findsOneWidget);
      expect(find.byKey(const Key('mtf-pill-1h')), findsOneWidget);
      expect(find.byKey(const Key('mtf-pill-4h')), findsOneWidget);
      expect(find.byKey(const Key('mtf-pill-1d')), findsOneWidget);

      Container pillContainer(String tf) =>
          tester.widget<Container>(find.byKey(Key('mtf-pill-$tf')));

      final trendDecoration = pillContainer('15m').decoration as BoxDecoration;
      expect(trendDecoration.border!.top.color, regimeColor('trend'));

      final compressionDecoration =
          pillContainer('1h').decoration as BoxDecoration;
      expect(
          compressionDecoration.border!.top.color, regimeColor('compression'));

      // 4h/1d had no fresh frame — neutral placeholder color.
      final missingDecoration = pillContainer('4h').decoration as BoxDecoration;
      expect(missingDecoration.border!.top.color, Colors.white24);
    });

    testWidgets(
        'shows a bias indicator on every real pill, regardless of dominant regime',
        (tester) async {
      final api = _FakeMtfRegimesApi(
        data: const MtfRegimesData(
          symbol: 'ETHUSDT',
          frames: [
            MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'down', score: 0.9),
            // Bias is a per-frame reading independent of the dominant
            // regime — a sideways/compression/expansion frame can still
            // lean up or down, so its pill must show the arrow too.
            MtfFrame(
                timeframe: '1h', dominant: 'sideways', bias: 'down', score: 0.4),
          ],
          alignment: 0.5,
          alignedState: 'indecisive',
        ),
      );

      await tester.pumpWidget(_app(mtfRegimesApi: api));
      await tester.pumpAndSettle();

      final trendPill = find.byKey(const Key('mtf-pill-15m'));
      final sidewaysPill = find.byKey(const Key('mtf-pill-1h'));
      final missingPill = find.byKey(const Key('mtf-pill-4h'));

      expect(
        find.descendant(
            of: trendPill, matching: find.byIcon(Icons.trending_down)),
        findsOneWidget,
      );
      expect(
        find.descendant(
            of: sidewaysPill, matching: find.byIcon(Icons.trending_down)),
        findsOneWidget,
      );
      // A missing/placeholder pill (no fresh frame) has nothing to report.
      expect(
        find.descendant(
            of: missingPill, matching: find.byIcon(Icons.trending_down)),
        findsNothing,
      );
      expect(
        find.descendant(of: missingPill, matching: find.byIcon(Icons.show_chart)),
        findsNothing,
      );
    });

    testWidgets('hidden entirely when no mtfRegimesApi is wired (free tier)',
        (tester) async {
      await tester.pumpWidget(_app(mtfRegimesApi: null));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('mtf-pill-15m')), findsNothing);
    });

    testWidgets('a fetch failure leaves the strip hidden without crashing',
        (tester) async {
      final api = _FakeMtfRegimesApi(error: Exception('network error'));

      await tester.pumpWidget(_app(mtfRegimesApi: api));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('mtf-pill-15m')), findsNothing);
      expect(tester.takeException(), isNull);
    });

    testWidgets(
        'a transient failure recovers when "Reload chart" is tapped (PR-100 CR)',
        (tester) async {
      final api = _FlakyMtfRegimesApi(const MtfRegimesData(
        symbol: 'ETHUSDT',
        frames: [
          MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
        ],
        alignment: 0.25,
        alignedState: 'indecisive',
      ));

      await tester.pumpWidget(
        _app(mtfRegimesApi: api, getCandleSeries: _FakeGetCandleSeries()),
      );
      await tester.pumpAndSettle();

      // First attempt failed — the strip stays hidden, not crashed.
      expect(api.calls, 1);
      expect(find.byKey(const Key('mtf-pill-15m')), findsNothing);

      await tester.tap(find.byTooltip('Reload chart'));
      await tester.pumpAndSettle();

      expect(api.calls, 2);
      expect(find.byKey(const Key('mtf-pill-15m')), findsOneWidget);
    });

    testWidgets(
        'a successful fetch is not re-fetched by "Reload chart" (PR-100 CR)',
        (tester) async {
      final api = _CountingMtfRegimesApi(const MtfRegimesData(
        symbol: 'ETHUSDT',
        frames: [
          MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
        ],
        alignment: 0.25,
        alignedState: 'indecisive',
      ));

      await tester.pumpWidget(
        _app(mtfRegimesApi: api, getCandleSeries: _FakeGetCandleSeries()),
      );
      await tester.pumpAndSettle();

      expect(api.calls, 1);
      expect(find.byKey(const Key('mtf-pill-15m')), findsOneWidget);

      await tester.tap(find.byTooltip('Reload chart'));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Reload chart'));
      await tester.pumpAndSettle();

      expect(api.calls, 1,
          reason: 'MTF is symbol-scoped; a successful reading must not be '
              'redundantly re-fetched just because the chart reloaded');
      expect(find.byKey(const Key('mtf-pill-15m')), findsOneWidget);
    });

    testWidgets(
        "an earlier call's success is not discarded by a later, overlapping "
        'call failing first (PR-100 CR)', (tester) async {
      final api = _RacingMtfRegimesApi();

      await tester.pumpWidget(
        _app(mtfRegimesApi: api, getCandleSeries: _FakeGetCandleSeries()),
      );
      await tester.pump();

      // First fetch (from initState) is now in flight.
      expect(api.completers.length, 1);

      // Reload chart while the first fetch is still pending — starts a
      // second, overlapping fetch (both fetch identical, symbol-scoped
      // data, so which one resolves first should not matter).
      await tester.tap(find.byTooltip('Reload chart'));
      await tester.pump();
      expect(api.completers.length, 2);

      // The newer call fails first.
      api.completers[1].completeError(Exception('transient network error'));
      await tester.pump();
      expect(find.byKey(const Key('mtf-pill-15m')), findsNothing);

      // The older call then succeeds — its data must still be applied,
      // not discarded just because a newer attempt happened to fail.
      api.completers[0].complete(const MtfRegimesData(
        symbol: 'ETHUSDT',
        frames: [
          MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
        ],
        alignment: 0.25,
        alignedState: 'indecisive',
      ));
      await tester.pump();

      expect(find.byKey(const Key('mtf-pill-15m')), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets(
        "a newer call's success is not overwritten by an older call's "
        'success arriving later (PR-100 CR)', (tester) async {
      final api = _RacingMtfRegimesApi();

      await tester.pumpWidget(
        _app(mtfRegimesApi: api, getCandleSeries: _FakeGetCandleSeries()),
      );
      await tester.pump();
      expect(api.completers.length, 1);

      await tester.tap(find.byTooltip('Reload chart'));
      await tester.pump();
      expect(api.completers.length, 2);

      // The newer call (dispatched second) succeeds first, with 2 frames.
      api.completers[1].complete(const MtfRegimesData(
        symbol: 'ETHUSDT',
        frames: [
          MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
          MtfFrame(
              timeframe: '1h', dominant: 'trend', bias: 'up', score: 0.7),
        ],
        alignment: 0.5,
        alignedState: 'trend',
      ));
      await tester.pump();
      BoxDecoration decorationFor(String tf) => tester
          .widget<Container>(find.byKey(Key('mtf-pill-$tf')))
          .decoration as BoxDecoration;
      expect(decorationFor('1h').border!.top.color, regimeColor('trend'));

      // The older call (dispatched first) succeeds afterwards, with only 1
      // frame (no 1h frame at all) — this stale reading must not replace
      // the newer one.
      api.completers[0].complete(const MtfRegimesData(
        symbol: 'ETHUSDT',
        frames: [
          MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'up', score: 0.8),
        ],
        alignment: 0.25,
        alignedState: 'indecisive',
      ));
      await tester.pump();

      expect(decorationFor('1h').border!.top.color, regimeColor('trend'),
          reason: 'the newer, already-applied reading must survive a '
              'slower, older response arriving after it, not fall back to '
              "the older response's missing-frame placeholder");
      expect(tester.takeException(), isNull);
    });
  });
}
