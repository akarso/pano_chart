import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/symbol.dart';
import 'package:pano_chart_frontend/domain/timeframe.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
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

Widget _app({MtfRegimesApi? mtfRegimesApi}) {
  return MaterialApp(
    theme: ThemeData.dark(useMaterial3: true),
    home: DetailScreen(
      symbol: const AppSymbol('ETHUSDT'),
      timeframe: const Timeframe('1h'),
      series: _fakeSeries(),
      detailContext: _fakeContext(),
      mtfRegimesApi: mtfRegimesApi,
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

    testWidgets('shows a bias arrow only when the pill is dominant trend',
        (tester) async {
      final api = _FakeMtfRegimesApi(
        data: const MtfRegimesData(
          symbol: 'ETHUSDT',
          frames: [
            MtfFrame(timeframe: '15m', dominant: 'trend', bias: 'down', score: 0.9),
            MtfFrame(
                timeframe: '1h',
                dominant: 'sideways',
                bias: 'neutral',
                score: 0.4),
          ],
          alignment: 0.5,
          alignedState: 'indecisive',
        ),
      );

      await tester.pumpWidget(_app(mtfRegimesApi: api));
      await tester.pumpAndSettle();

      final trendPill = find.byKey(const Key('mtf-pill-15m'));
      final sidewaysPill = find.byKey(const Key('mtf-pill-1h'));

      expect(
        find.descendant(
            of: trendPill, matching: find.byIcon(Icons.trending_down)),
        findsOneWidget,
      );
      expect(
        find.descendant(of: sidewaysPill, matching: find.byIcon(Icons.trending_down)),
        findsNothing,
      );
      expect(
        find.descendant(of: sidewaysPill, matching: find.byIcon(Icons.trending_up)),
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
  });
}
