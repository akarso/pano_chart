import 'overview_state.dart';

/// Result returned by [GetOverview].
class OverviewResult {
  final List<OverviewItem> items;
  final bool hasMore;
  final String? snapshot;

  const OverviewResult({
    required this.items,
    required this.hasMore,
    this.snapshot,
  });
}

/// Abstract use case for fetching overview data.
///
/// Implementations may call the backend `/api/overview` endpoint,
/// delegate to [GetCandleSeries], or return fake data for tests.
abstract class GetOverview {
  Future<OverviewResult> call({
    required String timeframe,
    required int page,
    required String sort,
    String? snapshot,
  });
}
