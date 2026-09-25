import 'overview_state.dart';

/// Result returned by [GetOverview].
class OverviewResult {
  final List<OverviewItem> items;
  final bool hasMore;
  final String? snapshot;
  final bool rsAvailable;

  /// Effective sort from the backend (`sort` JSON). May differ from the
  /// request when leaders/laggards fall back to `total` (PR-096).
  final String effectiveSort;

  /// Requested sort from the backend (`requestedSort`), when present.
  final String? requestedSort;

  const OverviewResult({
    required this.items,
    required this.hasMore,
    this.snapshot,
    this.rsAvailable = false,
    this.effectiveSort = '',
    this.requestedSort,
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
    String sidewaysAlgo = 'v5',
    List<String> symbols = const [],
  });
}
