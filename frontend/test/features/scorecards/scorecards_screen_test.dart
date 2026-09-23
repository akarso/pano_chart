import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/scorecards/http_scorecard_api.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecards_screen.dart';

class _ListApi implements ScorecardApi {
  int summaries = 0;
  Object? error;
  String? openedLabel;
  bool failGet = false;
  ScorecardSummary payload = const ScorecardSummary(
    timeframe: '1h',
    since: '2026-08-22T00:00:00Z',
    sinceRaw: '30d',
    items: [],
  );

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) async {
    summaries++;
    if (error != null) throw error!;
    return payload;
  }

  @override
  Future<ScorecardDetail> get({
    required String kind,
    required String label,
    required String timeframe,
    String since = '30d',
  }) async {
    openedLabel = label;
    if (failGet) throw Exception('down');
    return ScorecardDetail(
      kind: kind,
      label: label,
      timeframe: timeframe,
      since: '',
      sinceRaw: since,
      total: 40,
      hits: 20,
      hitRate: 0.5,
      baseline: 0.4,
      buckets: const [
        ScorecardBucket(
          lo: 0,
          hi: 0.1,
          n: 4,
          hits: 1,
          hitRate: 0.25,
          avgReturn: 0,
        ),
      ],
    );
  }
}

ScorecardSummaryItem _row({required int n, String label = 'trend_up'}) {
  return ScorecardSummaryItem(
    kind: 'badge',
    label: label,
    hitRate: 0.58,
    baseline: 0.49,
    n: n,
  );
}

void main() {
  testWidgets('empty summary says the window has no graded calls', (
    tester,
  ) async {
    final api = _ListApi();
    await tester.pumpWidget(
      MaterialApp(
        home: ScorecardsScreen(api: api, timeframe: '1h'),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('No graded calls in last 30d.'), findsOneWidget);
  });

  testWidgets('error offers retry and a small sample stays listed', (
    tester,
  ) async {
    final api = _ListApi()..error = Exception('down');
    await tester.pumpWidget(
      MaterialApp(
        home: ScorecardsScreen(api: api, timeframe: '1h'),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Retry'), findsOneWidget);

    api.error = null;
    api.payload = ScorecardSummary(
      timeframe: '1h',
      since: '',
      sinceRaw: '7d',
      items: [_row(n: 10)],
    );
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(api.summaries, 2);
    expect(find.textContaining('n=10'), findsOneWidget);
    expect(find.textContaining('last 7d'), findsOneWidget);
    expect(
      find.byKey(const ValueKey('reliability-chip-badge|trend_up')),
      findsNothing,
    );
  });

  testWidgets('tapping the row opens deciles even on the chip', (tester) async {
    final api = _ListApi()
      ..payload = ScorecardSummary(
        timeframe: '1h',
        since: '',
        sinceRaw: '30d',
        items: [_row(n: 412)],
      );
    await tester.pumpWidget(
      MaterialApp(
        home: ScorecardsScreen(api: api, timeframe: '1h'),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('58% · n=412'), warnIfMissed: false);
    await tester.pumpAndSettle();
    expect(api.openedLabel, 'trend_up');
    expect(find.text('0–10'), findsOneWidget);
  });

  testWidgets('decile error offers retry', (tester) async {
    final api = _ListApi()
      ..failGet = true
      ..payload = ScorecardSummary(
        timeframe: '1h',
        since: '',
        sinceRaw: '30d',
        items: [_row(n: 412)],
      );
    await tester.pumpWidget(
      MaterialApp(
        home: ScorecardsScreen(api: api, timeframe: '1h'),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Badge · trend up'));
    await tester.pumpAndSettle();
    expect(find.text('Could not load deciles.'), findsOneWidget);

    api.failGet = false;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(find.text('0–10'), findsOneWidget);
  });
}
