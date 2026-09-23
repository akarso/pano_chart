import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/scorecards/reliability_chip.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';

ScorecardSummaryItem _item({
  double hitRate = 0.58,
  double? baseline = 0.49,
  int n = 412,
}) {
  return ScorecardSummaryItem(
    kind: 'badge',
    label: 'trend_up',
    hitRate: hitRate,
    baseline: baseline,
    n: n,
  );
}

void main() {
  Future<void> pump(WidgetTester tester, ScorecardSummaryItem? item) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: ReliabilityChip(item: item)),
      ),
    );
  }

  Color paintColor(WidgetTester tester) {
    final box = tester.widget<Container>(
      find.descendant(
        of: find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
        matching: find.byType(Container),
      ),
    );
    return (box.decoration as BoxDecoration).color!;
  }

  Color expected(ReliabilityTone tone) =>
      ReliabilityChip.colorFor(tone).withAlpha((0.85 * 255).round());

  testWidgets('green when hit rate beats baseline by 5 points', (tester) async {
    await pump(tester, _item(hitRate: 0.58, baseline: 0.49));
    expect(find.text('58% · n=412'), findsOneWidget);
    expect(paintColor(tester), expected(ReliabilityTone.green));
  });

  testWidgets('green at exactly +5 points', (tester) async {
    await pump(tester, _item(hitRate: 0.55, baseline: 0.50));
    expect(paintColor(tester), expected(ReliabilityTone.green));
  });

  testWidgets('grey within 5 points of baseline', (tester) async {
    await pump(tester, _item(hitRate: 0.50, baseline: 0.49));
    expect(paintColor(tester), expected(ReliabilityTone.grey));
  });

  testWidgets('grey at exactly -5 points', (tester) async {
    await pump(tester, _item(hitRate: 0.45, baseline: 0.50));
    expect(paintColor(tester), expected(ReliabilityTone.grey));
  });

  testWidgets('grey when baseline is null', (tester) async {
    await pump(tester, _item(hitRate: 0.58, baseline: null));
    expect(paintColor(tester), expected(ReliabilityTone.grey));
  });

  testWidgets('red when hit rate trails baseline by more than 5 points', (
    tester,
  ) async {
    await pump(tester, _item(hitRate: 0.40, baseline: 0.49));
    expect(paintColor(tester), expected(ReliabilityTone.red));
  });

  testWidgets('dense chip does not force a 24px height', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: ReliabilityChip(item: _item(), dense: true)),
      ),
    );
    final size = tester.getSize(
      find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
    );
    expect(size.height, lessThan(24));
  });

  testWidgets('hidden when n is below 30', (tester) async {
    await pump(tester, _item(n: 29));
    expect(
      find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
      findsNothing,
    );
    expect(find.text('58% · n=29'), findsNothing);
  });

  testWidgets('hidden when there is no summary row', (tester) async {
    await pump(tester, null);
    expect(
      find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
      findsNothing,
    );
  });

  testWidgets('tap explains the sample and the chance rate', (tester) async {
    await pump(tester, _item());
    await tester.tap(
      find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
    );
    await tester.pumpAndSettle();
    expect(
      find.text(
        'Of the last 412 times the app showed this, 58% worked out. Chance: 49%.',
      ),
      findsOneWidget,
    );
  });
}
