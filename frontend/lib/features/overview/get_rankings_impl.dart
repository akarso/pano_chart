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
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
  }) async {
    final dto = await api.fetchRankings(
      timeframe: timeframe,
      sort: sort,
      page: page,
      pageSize: pageSize,
      sidewaysAlgo: sidewaysAlgo,
      symbols: symbols,
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
          sidewaysPercentile: e.sidewaysPercentile,
        ))
      .toList();

    // Log each OverviewItem for debugging
    for (final item in items) {
      print('[OverviewItem] symbol: ${item.symbol}, totalScore: ${item.totalScore}, trend: ${item.trendScore}, sideways: ${item.sidewaysScore}, gain: ${item.gainScore}, volume: ${item.volume}, sidewaysPercentile: ${item.sidewaysPercentile}, badgeComponent: ${item.badgeComponent}');
    }

    return OverviewResult(
      items: items,
      hasMore: dto.page < dto.totalPages,
      snapshot: null,
    );
  }
}
