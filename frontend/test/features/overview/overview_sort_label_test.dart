import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/overview_state.dart';

void main() {
  test('overviewSortMenuLabel stays Leaders before any response', () {
    final state = OverviewState.initial().copyWith(sort: 'leaders');
    expect(state.rsSortFellBack, false);
    expect(overviewSortMenuLabel(state), 'Leaders (vs market)');
  });

  test('overviewSortMenuLabel shows Total only after real fallback', () {
    final state = OverviewState.initial().copyWith(
      sort: 'leaders',
      effectiveSort: 'total',
      rsAvailable: false,
    );
    expect(state.rsSortFellBack, true);
    expect(overviewSortMenuLabel(state), 'Total');
  });

  test('overviewSortMenuLabel keeps Leaders when RS available', () {
    final state = OverviewState.initial().copyWith(
      sort: 'leaders',
      effectiveSort: 'leaders',
      rsAvailable: true,
    );
    expect(state.rsSortFellBack, false);
    expect(overviewSortMenuLabel(state), 'Leaders (vs market)');
  });
}
