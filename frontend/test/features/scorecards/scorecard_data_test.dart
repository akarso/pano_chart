import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';

void main() {
  test('summary parses nullable baseline and numeric zero', () {
    final summary = ScorecardSummary.fromJson({
      'timeframe': '1h',
      'since': '2026-08-22T00:00:00Z',
      'sinceRaw': '30d',
      'items': [
        {
          'kind': 'badge',
          'label': 'trend_up',
          'hitRate': 0.58,
          'baseline': 0.49,
          'n': 412,
        },
        {
          'kind': 'badge',
          'label': 'sideways',
          'hitRate': 0,
          'baseline': null,
          'n': 10,
        },
        {
          'kind': 'setup',
          'label': 'range',
          'hitRate': 0.2,
          'baseline': 0,
          'n': 40,
        },
      ],
    });

    expect(summary.sinceRaw, '30d');
    expect(summary.items[0].baseline, 0.49);
    expect(summary.items[0].n, 412);
    expect(summary.items[1].baseline, isNull);
    expect(summary.items[2].baseline, 0);
  });

  test('a non-object summary entry is skipped', () {
    final summary = ScorecardSummary.fromJson({
      'timeframe': '1h',
      'since': '',
      'sinceRaw': '30d',
      'items': [
        {
          'kind': 'badge',
          'label': 'trend_up',
          'hitRate': 0.58,
          'baseline': 0.49,
          'n': 412,
        },
        'not-an-object',
        3,
        {
          'kind': 'setup',
          'label': 'breakout_up',
          'hitRate': 0.4,
          'baseline': 0.3,
          'n': 40,
        },
      ],
    });

    expect(summary.items.map((item) => item.label), [
      'trend_up',
      'breakout_up',
    ]);
  });

  test('detail parses decile buckets', () {
    final card = ScorecardDetail.fromJson({
      'kind': 'badge',
      'label': 'trend_up',
      'timeframe': '1h',
      'since': '2026-08-22T00:00:00Z',
      'sinceRaw': '30d',
      'total': 20,
      'hits': 8,
      'hitRate': 0.4,
      'baseline': null,
      'buckets': [
        {
          'lo': 0.0,
          'hi': 0.1,
          'n': 12,
          'hits': 3,
          'hitRate': 0.25,
          'avgReturn': -0.01,
        },
      ],
    });

    expect(card.baseline, isNull);
    expect(card.buckets, hasLength(1));
    expect(card.buckets.single.n, 12);
    expect(card.buckets.single.hitRate, 0.25);
    expect(card.buckets.single.avgReturn, -0.01);
  });

  test('a non-object decile bucket is skipped', () {
    final card = ScorecardDetail.fromJson({
      'kind': 'badge',
      'label': 'trend_up',
      'timeframe': '1h',
      'since': '',
      'sinceRaw': '30d',
      'total': 20,
      'hits': 8,
      'hitRate': 0.4,
      'baseline': null,
      'buckets': [
        {
          'lo': 0.0,
          'hi': 0.1,
          'n': 12,
          'hits': 3,
          'hitRate': 0.25,
          'avgReturn': -0.01,
        },
        'not-an-object',
        {
          'lo': 0.1,
          'hi': 0.2,
          'n': 4,
          'hits': 1,
          'hitRate': 0.25,
          'avgReturn': 0,
        },
      ],
    });

    expect(card.buckets.map((bucket) => bucket.lo), [0.0, 0.1]);
  });

  test('badge and setup labels match the logged vocabulary', () {
    expect(badgeScorecardLabel('trend', [1, 2, 3]), 'trend_up');
    expect(badgeScorecardLabel('trend', [3, 2, 1]), 'trend_down');
    expect(badgeScorecardLabel('sideways', const []), 'sideways');
    expect(badgeScorecardLabel('gain', const [1]), 'gain');

    expect(
      setupScorecardLabel(
        bestSetup: 'compression_breakout',
        regime: '',
        breakoutUp: 0.6,
        breakoutDown: 0.2,
      ),
      'breakout_up',
    );
    expect(
      setupScorecardLabel(
        bestSetup: 'range_reversion',
        regime: '',
        breakoutUp: 0,
        breakoutDown: 0,
      ),
      'range',
    );
    expect(
      setupScorecardLabel(
        bestSetup: 'trend_continuation',
        regime: 'downtrend',
        breakoutUp: 0.9,
        breakoutDown: 0.1,
      ),
      'trend_down',
    );
    expect(regimeScorecardLabel('trend'), 'regime:trend');
  });

  test('tone hides small samples and treats a missing baseline as grey', () {
    expect(
      reliabilityTone(
        const ScorecardSummaryItem(
          kind: 'badge',
          label: 'trend_up',
          hitRate: 0.9,
          baseline: 0.1,
          n: 29,
        ),
      ),
      isNull,
    );
    expect(
      reliabilityTone(
        const ScorecardSummaryItem(
          kind: 'badge',
          label: 'trend_up',
          hitRate: 0.9,
          baseline: null,
          n: 30,
        ),
      ),
      ReliabilityTone.grey,
    );
    // 0.3 - 0.25 is not exactly 0.05 in binary. Basis points still count it as +5.
    expect(
      reliabilityTone(
        const ScorecardSummaryItem(
          kind: 'badge',
          label: 'trend_up',
          hitRate: 0.3,
          baseline: 0.25,
          n: 30,
        ),
      ),
      ReliabilityTone.green,
    );
  });

  test('titles do not prefix a label that already names the kind', () {
    expect(scorecardRowTitle('regime', 'regime:trend'), 'Regime · trend');
    expect(
      scorecardRowTitle('transition', 'transition:sideways'),
      'Transition · sideways',
    );
    expect(scorecardRowTitle('badge', 'trend_up'), 'Badge · trend up');
  });

  test('regime copy counts changes, not times the headline was on screen', () {
    const item = ScorecardSummaryItem(
      kind: 'regime',
      label: 'regime:trend',
      hitRate: 0.58,
      baseline: 0.49,
      n: 40,
    );
    expect(
      reliabilityExplanation(item),
      'Of the last 40 regime changes, 58% worked out. Chance: 49%.',
    );
  });

  test('setup chip is omitted below the logged confidence', () {
    const row = ScorecardSummaryItem(
      kind: 'setup',
      label: 'breakout_up',
      hitRate: 0.6,
      baseline: 0.4,
      n: 40,
    );
    expect(
      setupReliabilityItem(
        confidence: 0.49,
        label: 'breakout_up',
        items: const {'setup|breakout_up': row},
      ),
      isNull,
    );
    expect(
      setupReliabilityItem(
        confidence: 0.5,
        label: 'breakout_up',
        items: const {'setup|breakout_up': row},
      ),
      row,
    );
  });
}
