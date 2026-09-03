import 'dto/overview_response_dto.dart';

/// Port for fetching overview data from the backend.
abstract class OverviewApi {
  Future<OverviewResponseDto> fetchOverview({
    required String timeframe,
    required int limit,
  });
}
