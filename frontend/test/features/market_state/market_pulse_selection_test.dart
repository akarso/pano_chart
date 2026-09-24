import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/market_state/market_pulse_selection.dart';
import 'package:pano_chart_frontend/features/market_state/market_state_data.dart';
import 'package:pano_chart_frontend/features/market_state/participation_counts.dart';
import 'package:pano_chart_frontend/features/market_state/regime_data.dart';

void main() {
  const participation = ParticipationCounts(
    up: 62,
    down: 8,
    ranging: 30,
    total: 100,
  );

  RegimeData regime({
    bool unavailable = false,
    ParticipationCounts p = participation,
  }) =>
      RegimeData(
        timeframe: '4h',
        regime: 'trend',
        prevalence: 0.7,
        bias: 'up',
        scores: const RegimeScores(
          trend: 0.7,
          sideways: 0.1,
          compression: 0.1,
          expansion: 0.1,
        ),
        metrics: const RegimeMetrics(
          trendBreadth: 0.5,
          sidewaysBreadth: 0.2,
          expansionBreadth: 0.1,
          compressionBreadth: 0.2,
          volatilityExpansion: 1.0,
          dispersion: 0.03,
        ),
        dataQuality: unavailable ? 'unavailable' : 'ok',
        regimeSource: 'composite_volume_weighted',
        windowBars: 110,
        participation: p,
      );

  MarketStateData state({
    bool unavailable = false,
    ParticipationCounts p = const ParticipationCounts(
      up: 20,
      down: 15,
      ranging: 65,
      total: 100,
    ),
  }) =>
      MarketStateData(
        timeframe: '4h',
        state: 'sideways',
        confidence: 0.4,
        breadth: const MarketBreadth(
          sideways: 0.4,
          compression: 0.2,
          expansion: 0.2,
          trend: 0.2,
        ),
        symbolCount: 100,
        bias: 'neutral',
        dataQuality: unavailable ? 'unavailable' : 'ok',
        participation: p,
      );

  test('prefers regime participation and bias over state', () {
    final card = ParticipationCardModel.resolve(
      regime: regime(),
      state: state(),
    )!;
    expect(card.counts.up, 62);
    expect(card.reading, 'Broad move — most tokens trend with the tape');
  });

  test('falls back to state when regime is unavailable', () {
    final card = ParticipationCardModel.resolve(
      regime: regime(unavailable: true),
      state: state(),
    )!;
    expect(card.counts.up, 20);
    expect(card.reading, contains('Narrow'));
  });

  test('returns null when neither source has participation', () {
    expect(
      ParticipationCardModel.resolve(
        regime: regime(p: ParticipationCounts.empty),
        state: state(p: ParticipationCounts.empty),
      ),
      isNull,
    );
  });

  test('selectHeadlineRegime uses state.state as fallback', () {
    expect(
      selectHeadlineRegime(regime(unavailable: true), state()),
      'sideways',
    );
  });
}
