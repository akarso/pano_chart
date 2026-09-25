/// A single item in the overview grid.
class OverviewItem {
  final String symbol;
  final double totalScore;
  final double trendScore;
  final double sidewaysScore;
  final double gainScore;
  final double compressionScore;
  final double breakoutUpScore;
  final double breakoutDownScore;
  final double volume;
  final List<double> sparkline;
  final String badgeComponent;
  final double sidewaysPercentile;

  /// Log excess return vs composite (PR-096). Null when unscored / excluded.
  final double? rs;

  /// OLS beta on the same aligned window. Null when unscored / excluded.
  final double? beta;

  /// Percentile among scored RS rows only (0–1). Null when unscored.
  final double? rsRank;

  const OverviewItem({
    required this.symbol,
    this.totalScore = 0.0,
    this.trendScore = 0.0,
    this.sidewaysScore = 0.0,
    this.gainScore = 0.0,
    this.compressionScore = 0.0,
    this.breakoutUpScore = 0.0,
    this.breakoutDownScore = 0.0,
    this.volume = 0.0,
    this.sparkline = const [],
    this.badgeComponent = '',
    this.sidewaysPercentile = 0.0,
    this.rs,
    this.beta,
    this.rsRank,
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

  /// User-requested sort (menu selection).
  final String sort;

  /// Backend effective sort from the last successful response.
  final String effectiveSort;

  final String sidewaysAlgo;
  final String sortDirection; // 'up' or 'down'
  final String? snapshot;
  final String? error;

  /// True when the last rankings response scored ≥1 RS row (PR-096).
  final bool rsAvailable;

  const OverviewState({
    required this.isLoading,
    required this.items,
    required this.page,
    required this.hasMore,
    required this.sort,
    this.effectiveSort = '',
    this.sidewaysAlgo = 'v5',
    this.sortDirection = 'up',
    required this.snapshot,
    required this.error,
    this.rsAvailable = false,
  });

  factory OverviewState.initial() => const OverviewState(
        isLoading: false,
        items: [],
        page: 0,
        hasMore: true,
        sort: 'volume',
        effectiveSort: '',
        snapshot: null,
        error: null,
        rsAvailable: false,
      );

  /// True when leaders/laggards were requested and the backend reported a
  /// different effective sort (typically `total`). False before any response
  /// (`effectiveSort` still empty) so the menu does not flash Total.
  bool get rsSortFellBack =>
      (sort == 'leaders' || sort == 'laggards') &&
      effectiveSort.isNotEmpty &&
      effectiveSort != sort;

  OverviewState copyWith({
    bool? isLoading,
    List<OverviewItem>? items,
    int? page,
    bool? hasMore,
    String? sort,
    String? effectiveSort,
    String? sidewaysAlgo,
    String? sortDirection,
    String? snapshot,
    String? error,
    bool? rsAvailable,
  }) {
    return OverviewState(
      isLoading: isLoading ?? this.isLoading,
      items: items ?? this.items,
      page: page ?? this.page,
      hasMore: hasMore ?? this.hasMore,
      sort: sort ?? this.sort,
      effectiveSort: effectiveSort ?? this.effectiveSort,
      sidewaysAlgo: sidewaysAlgo ?? this.sidewaysAlgo,
      sortDirection: sortDirection ?? this.sortDirection,
      snapshot: snapshot ?? this.snapshot,
      error: error,
      rsAvailable: rsAvailable ?? this.rsAvailable,
    );
  }
}

/// Sort menu label for [state]. Shows Total (or other effective sort) only when
/// [OverviewState.rsSortFellBack] is true — never before the first response.
String overviewSortMenuLabel(OverviewState state) {
  final sort = state.rsSortFellBack
      ? (state.effectiveSort.isNotEmpty ? state.effectiveSort : 'total')
      : state.sort;
  switch (sort) {
    case 'sideways':
      return 'Sideways';
    case 'compression':
      return 'Compression';
    case 'breakout':
      return 'Breakout';
    case 'trend':
      return 'Trend';
    case 'leaders':
      return 'Leaders (vs market)';
    case 'laggards':
      return 'Laggards (vs market)';
    case 'gain':
      return 'Gainers';
    case 'losers':
      return 'Losers';
    case 'volume':
      return 'Volume';
    case 'total':
      return 'Total';
    default:
      return sort;
  }
}
