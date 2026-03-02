import 'dart:ui' show VoidCallback;
import 'dart:developer';

import 'get_overview.dart';
import 'overview_state.dart';
import '../../infrastructure/preferences_service.dart';
import 'dart:convert';

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
    List<OverviewItem> _sortItems(List<OverviewItem> items, String sort, {String direction = 'up'}) {
      final sorted = List<OverviewItem>.from(items);
      final desc = direction != 'down'; // 'up' = descending by score
      switch (sort) {
        case 'sideways':
          sorted.sort((a, b) => b.sidewaysPercentile.compareTo(a.sidewaysPercentile));
          break;
        case 'trend':
          sorted.sort((a, b) => desc
              ? b.trendScore.compareTo(a.trendScore)
              : a.trendScore.compareTo(b.trendScore));
          break;
        case 'gain':
          sorted.sort((a, b) => b.gainScore.compareTo(a.gainScore));
          break;
        case 'losers':
          sorted.sort((a, b) => a.gainScore.compareTo(b.gainScore));
          break;
        case 'volume':
          sorted.sort((a, b) => b.volume.compareTo(a.volume));
          break;
        case 'compression':
        case 'breakout':
          // Placeholder — no dedicated score field yet; use totalScore.
          sorted.sort((a, b) => desc
              ? b.totalScore.compareTo(a.totalScore)
              : a.totalScore.compareTo(b.totalScore));
          break;
        default:
          sorted.sort((a, b) => b.totalScore.compareTo(a.totalScore));
      }
      return sorted;
    }
  OverviewState _state = OverviewState.initial();
  OverviewState get state => _state;

  final GetOverview _getOverview;

  VoidCallback? onChanged;

  int _generation = 0;

  OverviewViewModel(this._getOverview);

  PreferencesService? _prefs;

  void attachPrefs(PreferencesService? prefs) {
    _prefs = prefs;
  }

  Future<void> loadInitial(String timeframe) async {
    final currentGen = ++_generation;
    print('[OverviewViewModel] loadInitial: timeframe=$timeframe generation=$currentGen');
    _setState(
        _state.copyWith(isLoading: true, items: [], page: 0, error: null));

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: 1,
        sort: _state.sort,
        sidewaysAlgo: _state.sidewaysAlgo,
      );
        print("[OverviewViewModel] GetOverview result: {"
          " hasMore: "+result.hasMore.toString()+
          ", snapshot: ${result.snapshot}"
          ", items.length: ${result.items.length}");
        for (var i = 0; i < result.items.length; i++) {
        final item = result.items[i];
        print("  [item #$i] "
          "symbol: ${item.symbol}, "
          "totalScore: ${item.totalScore}, "
          "trendScore: ${item.trendScore}, "
          "sidewaysScore: ${item.sidewaysScore}, "
          "sidewaysPercentile: ${item.sidewaysPercentile}, "
          "gainScore: ${item.gainScore}, "
          "volume: ${item.volume}, "
          "badgeComponent: ${item.badgeComponent}, "
          "sparkline: ${item.sparkline}");
        }
        print("[OverviewViewModel] End of GetOverview result");
      if (currentGen != _generation) return;

      // Save to cache if prefs available
      if (_prefs != null) {
        // Store a minimal cache: items, hasMore, snapshot
        final cache = {
          'items': result.items.map((e) => {
            'symbol': e.symbol,
            'totalScore': e.totalScore,
            'trendScore': e.trendScore,
            'sidewaysScore': e.sidewaysScore,
            'gainScore': e.gainScore,
            'volume': e.volume,
            'sparkline': e.sparkline,
            'badgeComponent': e.badgeComponent,
          }).toList(),
          'hasMore': result.hasMore,
          'snapshot': result.snapshot,
          'sort': _state.sort,
          'sidewaysAlgo': _state.sidewaysAlgo,
        };
        _prefs!.setRankingsCache(timeframe, jsonEncode(cache));
      }

      final sortedItems = _sortItems(result.items, _state.sort, direction: _state.sortDirection);
      _setState(
        _state.copyWith(
          isLoading: false,
          items: sortedItems,
          page: 1,
          hasMore: result.hasMore,
          snapshot: result.snapshot,
        ),
      );
    } catch (e) {
      if (currentGen != _generation) return;

      // Try to load from cache if prefs available
      if (_prefs != null) {
        final cacheStr = _prefs!.getRankingsCache(timeframe);
        if (cacheStr != null) {
          try {
            final cache = jsonDecode(cacheStr) as Map<String, dynamic>;
            final items = (cache['items'] as List)
                .map((e) => OverviewItem(
                      symbol: e['symbol'] as String,
                      totalScore: (e['totalScore'] as num).toDouble(),
                      trendScore: (e['trendScore'] as num).toDouble(),
                      sidewaysScore: (e['sidewaysScore'] as num).toDouble(),
                      gainScore: (e['gainScore'] as num).toDouble(),
                      volume: (e['volume'] as num).toDouble(),
                      sparkline: (e['sparkline'] as List).map((v) => (v as num).toDouble()).toList(),
                      badgeComponent: e['badgeComponent'] as String? ?? '',
                    ))
                .toList();
            _setState(_state.copyWith(
              isLoading: false,
              items: items,
              page: 1,
              hasMore: cache['hasMore'] as bool? ?? false,
              snapshot: cache['snapshot'] as String?,
              error: 'Offline — showing cached data',
            ));
            return;
          } catch (_) {
            // Ignore cache parse errors, fall through to error
          }
        }
      }
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  /// Refreshes the current data without clearing existing items.
  /// Used by pull-to-refresh so the grid stays visible during reload.
  Future<void> refresh(String timeframe) async {
    final currentGen = ++_generation;

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: 1,
        sort: _state.sort,
        sidewaysAlgo: _state.sidewaysAlgo,
      );

      if (currentGen != _generation) return;

      _setState(
        _state.copyWith(
          isLoading: false,
          items: result.items,
          page: 1,
          hasMore: result.hasMore,
          snapshot: result.snapshot,
          error: null,
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
        sidewaysAlgo: _state.sidewaysAlgo,
      );

      if (currentGen != _generation) return;

      final merged = [..._state.items, ...result.items];
      final sortedItems = _sortItems(merged, _state.sort, direction: _state.sortDirection);
      _setState(
        _state.copyWith(
          isLoading: false,
          items: sortedItems,
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

    _state = OverviewState.initial().copyWith(
      sort: newSort,
      sidewaysAlgo: _state.sidewaysAlgo,
      sortDirection: _state.sortDirection,
    );
    onChanged?.call();

    loadInitial(timeframe);
  }

  /// Updates sort without triggering a reload — used during init to sync
  /// persisted preferences before the first [loadInitial].
  void changeSortSilent(String newSort) {
    _state = _state.copyWith(sort: newSort);
  }

  /// Changes the sort direction (up/down) and re-sorts locally.
  void changeSortDirection(String direction, String timeframe) {
    if (direction == _state.sortDirection) return;

    final resorted = _sortItems(_state.items, _state.sort, direction: direction);
    _setState(_state.copyWith(sortDirection: direction, items: resorted));
  }

  /// Updates sort direction without triggering a reload.
  void changeSortDirectionSilent(String direction) {
    _state = _state.copyWith(sortDirection: direction);
  }

  void changeSidewaysAlgo(String algo, String timeframe) {
    if (algo == _state.sidewaysAlgo) return;

    _generation++;

    _state = OverviewState.initial().copyWith(
      sort: _state.sort,
      sidewaysAlgo: algo,
    );
    onChanged?.call();

    loadInitial(timeframe);
  }

  /// Updates sideways algo without triggering a reload — used during init
  /// to sync persisted preferences before the first [loadInitial].
  void changeSidewaysAlgoSilent(String algo) {
    _state = _state.copyWith(sidewaysAlgo: algo);
  }

  void _setState(OverviewState newState) {
    _state = newState;
    onChanged?.call();
  }
}
