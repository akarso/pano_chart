import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/scorecards/http_scorecard_api.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_catalog.dart';
import 'package:pano_chart_frontend/features/scorecards/scorecard_data.dart';

class _GateApi implements ScorecardApi {
  final List<Completer<ScorecardSummary>> waiters = [];

  @override
  Future<ScorecardSummary> summary({
    required String timeframe,
    String since = '30d',
  }) {
    final done = Completer<ScorecardSummary>();
    waiters.add(done);
    return done.future;
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

  void succeed(int index, String label) {
    waiters[index].complete(
      ScorecardSummary(
        timeframe: '',
        since: '',
        sinceRaw: '30d',
        items: [
          ScorecardSummaryItem(
            kind: 'badge',
            label: label,
            hitRate: 0.6,
            baseline: 0.4,
            n: 40,
          ),
        ],
      ),
    );
  }

  void fail(int index) {
    waiters[index].completeError(Exception('down'));
  }
}

void main() {
  test('a failed 4h fetch after a 1h hit paints no chip', () async {
    final api = _GateApi();
    final catalog = ScorecardCatalog();
    final first = catalog.load(api: api, timeframe: '1h', notify: () {});
    api.succeed(0, 'trend_up');
    await first;
    expect(catalog.items, isNotEmpty);

    final second = catalog.load(api: api, timeframe: '4h', notify: () {});
    expect(catalog.items, isEmpty);
    api.fail(1);
    await second;
    expect(catalog.items, isEmpty);
  });

  test('an older 1h response does not replace a later 1h response', () async {
    final api = _GateApi();
    final catalog = ScorecardCatalog();
    final first = catalog.load(api: api, timeframe: '1h', notify: () {});
    final second = catalog.load(api: api, timeframe: '4h', notify: () {});
    final third = catalog.load(api: api, timeframe: '1h', notify: () {});
    api.succeed(2, 'later');
    await third;
    api.succeed(0, 'stale');
    await first;
    api.succeed(1, 'middle');
    await second;
    expect(catalog.items.values.single.label, 'later');
  });

  test('a failed reload of the same timeframe keeps the chip', () async {
    final api = _GateApi();
    final catalog = ScorecardCatalog();
    final first = catalog.load(api: api, timeframe: '1h', notify: () {});
    api.succeed(0, 'trend_up');
    await first;
    final second = catalog.load(api: api, timeframe: '1h', notify: () {});
    api.fail(1);
    await second;
    expect(catalog.items.values.single.label, 'trend_up');
  });
}
