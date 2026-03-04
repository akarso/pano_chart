import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/dto/rankings_response_dto.dart';
import 'package:pano_chart_frontend/features/overview/get_rankings_impl.dart';
import 'package:pano_chart_frontend/features/overview/rankings_api.dart';

void main() {
  test('GetRankingsImpl_mapsItemsCorrectly', () async {
    final api = _FakeRankingsApi(const RankingsResponseDto(
      timeframe: '1h',
      sort: 'total',
      page: 1,
      pageSize: 30,
      totalItems: 2,
      totalPages: 1,
      precision: 30,
      results: [
        RankingItemDto(
          symbol: 'BTCUSDT',
          totalScore: 2.75,
          scores: RankingScoresDto(trend: 1.0, sideways: 0.5, gain: 1.25),
          volume: 5000.0,
          sparkline: [42000.0, 42100.0],
          badgeComponent: 'trend',
          sidewaysPercentile: 0.85,
        ),
        RankingItemDto(
          symbol: 'ETHUSDT',
          totalScore: -1.5,
          scores: RankingScoresDto(trend: -0.5, sideways: 0.2, gain: -1.2),
          volume: 2000.0,
          sparkline: [3200.0],
          badgeComponent: '',
          sidewaysPercentile: 0.42,
        ),
      ],
    ));

    final impl = GetRankingsImpl(api);
    final result = await impl.call(timeframe: '1h', page: 1, sort: 'total');

    expect(result.items.length, 2);
    expect(result.items[0].symbol, 'BTCUSDT');
    expect(result.items[0].totalScore, 2.75);
    expect(result.items[0].trendScore, 1.0);
    expect(result.items[0].sidewaysScore, 0.5);
    expect(result.items[0].gainScore, 1.25);
    expect(result.items[0].volume, 5000.0);
    expect(result.items[0].sparkline, [42000.0, 42100.0]);
    expect(result.items[0].badgeComponent, 'trend');

    expect(result.items[1].symbol, 'ETHUSDT');
    expect(result.items[1].trendScore, -0.5);
    expect(result.items[1].badgeComponent, '');
  });

  test('GetRankingsImpl_hasMoreWhenPageLessThanTotalPages', () async {
    final api = _FakeRankingsApi(const RankingsResponseDto(
      timeframe: '1h',
      sort: 'total',
      page: 1,
      pageSize: 30,
      totalItems: 60,
      totalPages: 2,
      precision: 30,
      results: [],
    ));

    final impl = GetRankingsImpl(api);
    final result = await impl.call(timeframe: '1h', page: 1, sort: 'total');

    expect(result.hasMore, true);
  });

  test('GetRankingsImpl_hasMoreFalseOnLastPage', () async {
    final api = _FakeRankingsApi(const RankingsResponseDto(
      timeframe: '1h',
      sort: 'total',
      page: 2,
      pageSize: 30,
      totalItems: 60,
      totalPages: 2,
      precision: 30,
      results: [],
    ));

    final impl = GetRankingsImpl(api);
    final result = await impl.call(timeframe: '1h', page: 2, sort: 'total');

    expect(result.hasMore, false);
  });

  test('GetRankingsImpl_passesPageAndSort', () async {
    final api = _FakeRankingsApi(const RankingsResponseDto(
      timeframe: '4h',
      sort: 'gain',
      page: 3,
      pageSize: 20,
      totalItems: 100,
      totalPages: 5,
      precision: 30,
      results: [],
    ));

    final impl = GetRankingsImpl(api, pageSize: 20);
    await impl.call(timeframe: '4h', page: 3, sort: 'gain');

    expect(api.capturedTimeframe, '4h');
    expect(api.capturedSort, 'gain');
    expect(api.capturedPage, 3);
    expect(api.capturedPageSize, 20);
  });
}

class _FakeRankingsApi implements RankingsApi {
  final RankingsResponseDto response;

  String? capturedTimeframe;
  String? capturedSort;
  int? capturedPage;
  int? capturedPageSize;

  _FakeRankingsApi(this.response);

  @override
  Future<RankingsResponseDto> fetchRankings({
    required String timeframe,
    required String sort,
    required int page,
    required int pageSize,
    String sidewaysAlgo = 'v1',
    List<String> symbols = const [],
  }) async {
    capturedTimeframe = timeframe;
    capturedSort = sort;
    capturedPage = page;
    capturedPageSize = pageSize;
    return response;
  }
}
