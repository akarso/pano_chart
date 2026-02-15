import '../candles/api/candle_response.dart';

/// A single item in the overview grid.
class OverviewItem {
  final String symbol;
  final List<CandleDto> candles;

  const OverviewItem({
    required this.symbol,
    required this.candles,
  });
}

/// Immutable state object for the overview screen.
class OverviewState {
  final bool isLoading;
  final List<OverviewItem> items;
  final int page;
  final bool hasMore;
  final String sort;
  final String? snapshot;
  final String? error;

  const OverviewState({
    required this.isLoading,
    required this.items,
    required this.page,
    required this.hasMore,
    required this.sort,
    required this.snapshot,
    required this.error,
  });

  factory OverviewState.initial() => const OverviewState(
        isLoading: false,
        items: [],
        page: 0,
        hasMore: true,
        sort: 'total',
        snapshot: null,
        error: null,
      );

  OverviewState copyWith({
    bool? isLoading,
    List<OverviewItem>? items,
    int? page,
    bool? hasMore,
    String? sort,
    String? snapshot,
    String? error,
  }) {
    return OverviewState(
      isLoading: isLoading ?? this.isLoading,
      items: items ?? this.items,
      page: page ?? this.page,
      hasMore: hasMore ?? this.hasMore,
      sort: sort ?? this.sort,
      snapshot: snapshot ?? this.snapshot,
      error: error,
    );
  }
}
