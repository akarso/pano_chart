import 'get_overview.dart';
import 'overview_api.dart';
import 'overview_state.dart';

/// Concrete [GetOverview] implementation backed by [OverviewApi].
///
/// Currently ignores page/sort/snapshot — MVP uses a single
/// limit-based fetch. Pagination will be added in PR-014.
class GetOverviewImpl implements GetOverview {
  final OverviewApi api;

  GetOverviewImpl(this.api);

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
  }) async {
    final dto = await api.fetchOverview(
      timeframe: timeframe,
      limit: 30,
    );

    return OverviewResult(
      items: dto.results
          .map((e) => OverviewItem(
                symbol: e.symbol,
                totalScore: e.totalScore,
                sparkline: e.sparkline,
              ))
          .toList(),
      hasMore: false,
      snapshot: null,
    );
  }
}
