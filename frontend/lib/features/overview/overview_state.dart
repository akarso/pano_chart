/// A single item in the overview grid.
class OverviewItem {
  final String symbol;
  final double totalScore;
  final double trendScore;
  final double sidewaysScore;
  final double gainScore;
  final double volume;
  final List<double> sparkline;
  final String badgeComponent;
  final double sidewaysPercentile;

  const OverviewItem({
    required this.symbol,
    this.totalScore = 0.0,
    this.trendScore = 0.0,
    this.sidewaysScore = 0.0,
    this.gainScore = 0.0,
    this.volume = 0.0,
    this.sparkline = const [],
    this.badgeComponent = '',
    this.sidewaysPercentile = 0.0,
  });
}

/// Sort options that support an up/down direction toggle.
const kDirectionalSorts = {'compression', 'breakout', 'trend'};

/// Immutable state object for the overview screen.
class OverviewState {
  final bool isLoading;
  final List<OverviewItem> items;
  final int page;
  final bool hasMore;
  final String sort;
  final String sidewaysAlgo;
  final String sortDirection; // 'up' or 'down'
  final String? snapshot;
  final String? error;

  const OverviewState({
    required this.isLoading,
    required this.items,
    required this.page,
    required this.hasMore,
    required this.sort,
    this.sidewaysAlgo = 'v1',
    this.sortDirection = 'up',
    required this.snapshot,
    required this.error,
  });

  factory OverviewState.initial() => const OverviewState(
        isLoading: false,
        items: [],
        page: 0,
        hasMore: true,
        sort: 'volume',
        snapshot: null,
        error: null,
      );

  OverviewState copyWith({
    bool? isLoading,
    List<OverviewItem>? items,
    int? page,
    bool? hasMore,
    String? sort,
    String? sidewaysAlgo,
    String? sortDirection,
    String? snapshot,
    String? error,
  }) {
    return OverviewState(
      isLoading: isLoading ?? this.isLoading,
      items: items ?? this.items,
      page: page ?? this.page,
      hasMore: hasMore ?? this.hasMore,
      sort: sort ?? this.sort,
      sidewaysAlgo: sidewaysAlgo ?? this.sidewaysAlgo,
      sortDirection: sortDirection ?? this.sortDirection,
      snapshot: snapshot ?? this.snapshot,
      error: error,
    );
  }
}
