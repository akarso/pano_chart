import 'dto/rankings_response_dto.dart';

/// Port for fetching rankings data from the backend.
abstract class RankingsApi {
  Future<RankingsResponseDto> fetchRankings({
    required String timeframe,
    required String sort,
    required int page,
    required int pageSize,
  });
}
