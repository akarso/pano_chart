import 'dart:ui' show VoidCallback;

import 'get_overview.dart';
import 'overview_state.dart';

/// OverviewViewModel owns asynchronous state and pagination-ready logic.
///
/// Uses a simple [VoidCallback] to notify the widget of state changes.
/// Widget rebuilds via `setState` when [onChanged] fires.
///
/// A generation counter (`_generation`) guards against stale async responses.
/// Any state-resetting action (sort change, timeframe change, refresh)
/// increments the counter; in-flight responses from a previous generation
/// are silently discarded.
class OverviewViewModel {
  OverviewState _state = OverviewState.initial();
  OverviewState get state => _state;

  final GetOverview _getOverview;

  VoidCallback? onChanged;

  int _generation = 0;

  OverviewViewModel(this._getOverview);

  Future<void> loadInitial(String timeframe) async {
    final currentGen = ++_generation;

    _setState(
        _state.copyWith(isLoading: true, items: [], page: 0, error: null));

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: 1,
        sort: _state.sort,
      );

      if (currentGen != _generation) return;

      _setState(
        _state.copyWith(
          isLoading: false,
          items: result.items,
          page: 1,
          hasMore: result.hasMore,
          snapshot: result.snapshot,
        ),
      );
    } catch (e) {
      if (currentGen != _generation) return;
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  Future<void> loadNext(String timeframe) async {
    if (_state.isLoading || !_state.hasMore) return;

    final currentGen = _generation;

    _setState(_state.copyWith(isLoading: true));

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: _state.page + 1,
        sort: _state.sort,
        snapshot: _state.snapshot,
      );

      if (currentGen != _generation) return;

      _setState(
        _state.copyWith(
          isLoading: false,
          items: [..._state.items, ...result.items],
          page: _state.page + 1,
          hasMore: result.hasMore,
        ),
      );
    } catch (e) {
      if (currentGen != _generation) return;
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  void changeSort(String newSort, String timeframe) {
    if (newSort == _state.sort) return;

    _generation++;

    _state = OverviewState.initial().copyWith(sort: newSort);
    onChanged?.call();

    loadInitial(timeframe);
  }

  void _setState(OverviewState newState) {
    _state = newState;
    onChanged?.call();
  }
}
