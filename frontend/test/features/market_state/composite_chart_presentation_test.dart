import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/market_state/composite_chart_presentation.dart';
import 'package:pano_chart_frontend/features/market_state/composite_index_data.dart';

void main() {
  group('mergeContextWithTape', () {
    test('returns tape when context is not longer', () {
      final tape = [
        const IndexPoint(timestamp: 1, value: 100),
        const IndexPoint(timestamp: 2, value: 110),
      ];
      final context = [
        const IndexPoint(timestamp: 1, value: 100),
        const IndexPoint(timestamp: 2, value: 105),
      ];
      final merged = mergeContextWithTape(context, tape);
      expect(merged, tape);
    });

    test('scales tape onto context join without rewriting the prefix', () {
      final context = [
        for (var i = 0; i < 5; i++)
          IndexPoint(timestamp: i, value: 100.0 + i),
      ];
      // Tape = last 2 closes rebased to 100 at join candle.
      final tape = [
        const IndexPoint(timestamp: 3, value: 100),
        const IndexPoint(timestamp: 4, value: 125), // +25% from join
      ];
      final merged = mergeContextWithTape(context, tape);
      expect(merged.length, 5);
      expect(merged[0].value, 100);
      expect(merged[1].value, 101);
      expect(merged[2].value, 102);
      // Join matches context[3]=103; tape +25% → 103 * 1.25
      expect(merged[3].value, 103);
      expect(merged[4].value, closeTo(103 * 1.25, 1e-9));
    });
  });

  group('CompositeChartPresentation.resolve', () {
    CompositeIndexData series(int n, {double step = 0.1}) => CompositeIndexData(
          timeframe: '4h',
          symbolCount: 10,
          points: [
            for (var i = 0; i < n;
                i++) IndexPoint(timestamp: i, value: 100 + i * step),
          ],
          volumeWeightedPoints: [
            for (var i = 0; i < n;
                i++) IndexPoint(timestamp: i, value: 100 + i * step * 1.5),
          ],
        );

    test('splices tape into 200-bar context and enables scored OLS', () {
      final context = series(200);
      final tape = series(110, step: 0.2); // different path = tape rebase
      final view = CompositeChartPresentation.resolve(
        context: context,
        tape: tape,
        regimeSource: 'composite_volume_weighted',
        windowBars: 110,
        useVolumeWeighted: true,
      );
      expect(view.chartPoints.length, 200);
      expect(view.scoredWin, 110);
      expect(view.showRegression, isTrue);
      expect(view.hasScoredEvidence, isTrue);
      expect(view.changeScope, 'scored-window change');
      // Suffix follows scaled tape, not context's last 110.
      expect(view.chartPoints[90].value, context.volumeWeightedPoints[90].value);
      expect(
        view.chartPoints.last.value,
        isNot(context.volumeWeightedPoints.last.value),
      );
    });

    test('without tape series, long context stays unscored', () {
      final view = CompositeChartPresentation.resolve(
        context: series(200),
        tape: null,
        regimeSource: 'composite_volume_weighted',
        windowBars: 110,
        useVolumeWeighted: true,
      );
      expect(view.scoredWin, 0);
      expect(view.showRegression, isFalse);
      expect(view.changeScope, 'context change');
    });

    test('series mismatch hides regression', () {
      final view = CompositeChartPresentation.resolve(
        context: series(110),
        tape: series(110),
        regimeSource: 'composite_volume_weighted',
        windowBars: 110,
        useVolumeWeighted: false,
      );
      expect(view.matchesTape, isFalse);
      expect(view.scoredWin, 0);
      expect(view.showRegression, isFalse);
      expect(view.changeScope, 'series change');
    });
  });

  group('headlineSubline', () {
    test('uses last-N wording when scored evidence is on a longer chart', () {
      expect(
        headlineSubline(
          regimeSource: 'composite_volume_weighted',
          windowBars: 110,
          chartLen: 200,
          timeframe: '4h',
          matchesTape: true,
          hasScoredEvidence: true,
        ),
        'Scored on the last 110 bars shown • 4h',
      );
    });
  });
}
