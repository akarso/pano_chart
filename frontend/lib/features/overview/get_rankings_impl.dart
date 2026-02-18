import 'get_overview.dart';
import 'overview_state.dart';
import 'rankings_api.dart';

/// Concrete [GetOverview] implementation backed by [RankingsApi].
///
/// Maps the paginated `/api/rankings` response into [OverviewResult].
class GetRankingsImpl implements GetOverview {
  final RankingsApi api;
  final int pageSize;

  GetRankingsImpl(this.api, {this.pageSize = 30});

  @override
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
    String sidewaysAlgo = 'v2',
  }) async {
    final dto = await api.fetchRankings(
      timeframe: timeframe,
      sort: sort,
      page: page,
      pageSize: pageSize,
      sidewaysAlgo: sidewaysAlgo,
    );

    final items = dto.results
        .map((e) => OverviewItem(
              symbol: e.symbol,
              totalScore: e.totalScore,
              trendScore: e.scores.trend,
              sidewaysScore: e.scores.sideways,
              gainScore: e.scores.gain,
              volume: e.volume,
              sparkline: e.sparkline,
              badgeComponent: e.badgeComponent,
            ))
        .toList();

    return OverviewResult(
      items: items,
      hasMore: dto.page < dto.totalPages,
      snapshot: null,
    );
  }
}
