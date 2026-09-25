import 'dart:async';
import 'dart:convert';
import 'dart:ui' show VoidCallback;

import 'package:flutter/foundation.dart';

import '../../infrastructure/preferences_service.dart';
import 'dto/rankings_response_dto.dart' show parseJsonBool;
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

  PreferencesService? _prefs;

  /// When set, [loadInitial]/[refresh] cache writes await this before SET.
  /// Tests use it to flip sort/generation mid-write. Must be null in production.
  @visibleForTesting
  Completer<void>? cacheWriteGate;

  void attachPrefs(PreferencesService? prefs) {
    _prefs = prefs;
  }

  /// Applies local sort. Leaders/laggards only reorder when [rsAvailable];
  /// otherwise the backend (fallback) order is kept.
  List<OverviewItem> _sortItems(
    List<OverviewItem> items,
    String sort, {
    String direction = 'up',
    bool rsAvailable = false,
  }) {
    if ((sort == 'leaders' || sort == 'laggards') && !rsAvailable) {
      return List<OverviewItem>.from(items);
    }

    final sorted = List<OverviewItem>.from(items);
    final desc = direction != 'down'; // 'up' = descending by score
    switch (sort) {
      case 'sideways':
        sorted.sort(
            (a, b) => b.sidewaysPercentile.compareTo(a.sidewaysPercentile));
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
      case 'leaders':
        sorted.sort((a, b) => compareRsNullsLast(a, b, descending: true));
        break;
      case 'laggards':
        sorted.sort((a, b) => compareRsNullsLast(a, b, descending: false));
        break;
      case 'compression':
        double signedComp(OverviewItem item) {
          final s = item.sparkline;
          if (s.length < 2 || s.first == 0) return item.compressionScore;
          return s.last >= s.first
              ? item.compressionScore
              : -item.compressionScore;
        }
        sorted.sort((a, b) => desc
            ? signedComp(b).compareTo(signedComp(a))
            : signedComp(a).compareTo(signedComp(b)));
        break;
      case 'breakout':
        if (desc) {
          sorted.sort(
              (a, b) => b.breakoutUpScore.compareTo(a.breakoutUpScore));
        } else {
          sorted.sort(
              (a, b) => b.breakoutDownScore.compareTo(a.breakoutDownScore));
        }
        break;
      default:
        sorted.sort((a, b) => b.totalScore.compareTo(a.totalScore));
    }
    return sorted;
  }

  /// RS desc/asc with unscored rows last (matches backend leaders/laggards).
  /// Visible for tests.
  static int compareRsNullsLast(
    OverviewItem a,
    OverviewItem b, {
    required bool descending,
  }) {
    final ar = a.rs;
    final br = b.rs;
    if (ar == null && br == null) return a.symbol.compareTo(b.symbol);
    if (ar == null) return 1;
    if (br == null) return -1;
    final c = descending ? br.compareTo(ar) : ar.compareTo(br);
    return c != 0 ? c : a.symbol.compareTo(b.symbol);
  }

  List<OverviewItem> _sortedFromResult(OverviewResult result) {
    return _sortItems(
      result.items,
      _state.sort,
      direction: _state.sortDirection,
      rsAvailable: result.rsAvailable,
    );
  }

  Map<String, dynamic> _buildRankingsCacheMap(
    OverviewResult result, {
    required String sort,
    required String sidewaysAlgo,
  }) {
    return {
      'items': result.items
          .map((e) => {
                'symbol': e.symbol,
                'totalScore': e.totalScore,
                'trendScore': e.trendScore,
                'sidewaysScore': e.sidewaysScore,
                'gainScore': e.gainScore,
                'compressionScore': e.compressionScore,
                'breakoutUpScore': e.breakoutUpScore,
                'breakoutDownScore': e.breakoutDownScore,
                'volume': e.volume,
                'sparkline': e.sparkline,
                'badgeComponent': e.badgeComponent,
                'sidewaysPercentile': e.sidewaysPercentile,
                'rs': e.rs,
                'beta': e.beta,
                'rsRank': e.rsRank,
              })
          .toList(),
      'hasMore': result.hasMore,
      'snapshot': result.snapshot,
      'sort': sort,
      'sidewaysAlgo': sidewaysAlgo,
      'rsAvailable': result.rsAvailable,
      'effectiveSort': result.effectiveSort,
    };
  }

  /// Writes [cache] only if [gen] is still the active generation.
  /// A slower SET that lands after a newer write may clobber disk; the payload
  /// is still sort-tagged, so offline restore ignores a sort mismatch.
  Future<void> _writeRankingsCacheIfCurrent(
    String timeframe,
    int gen,
    Map<String, dynamic> cache,
  ) async {
    if (_prefs == null) return;
    if (gen != _generation) return;
    final payload = jsonEncode(cache);
    final gate = cacheWriteGate;
    if (gate != null) await gate.future;
    if (gen != _generation) return;
    await _prefs!.setRankingsCache(timeframe, payload);
  }

  OverviewItem _itemFromCacheMap(Map<String, dynamic> e) {
    return OverviewItem(
      symbol: e['symbol'] as String,
      totalScore: (e['totalScore'] as num).toDouble(),
      trendScore: (e['trendScore'] as num).toDouble(),
      sidewaysScore: (e['sidewaysScore'] as num).toDouble(),
      gainScore: (e['gainScore'] as num).toDouble(),
      compressionScore: (e['compressionScore'] as num?)?.toDouble() ?? 0.0,
      breakoutUpScore: (e['breakoutUpScore'] as num?)?.toDouble() ?? 0.0,
      breakoutDownScore: (e['breakoutDownScore'] as num?)?.toDouble() ?? 0.0,
      volume: (e['volume'] as num).toDouble(),
      sparkline: (e['sparkline'] as List)
          .map((v) => (v as num).toDouble())
          .toList(),
      badgeComponent: e['badgeComponent'] as String? ?? '',
      sidewaysPercentile:
          (e['sidewaysPercentile'] as num?)?.toDouble() ?? 0.0,
      rs: (e['rs'] as num?)?.toDouble(),
      beta: (e['beta'] as num?)?.toDouble(),
      rsRank: (e['rsRank'] as num?)?.toDouble(),
    );
  }

  Future<void> loadInitial(String timeframe) async {
    final currentGen = ++_generation;
    _setState(
        _state.copyWith(isLoading: true, items: [], page: 0, error: null));

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: 1,
        sort: _state.sort,
        sidewaysAlgo: _state.sidewaysAlgo,
      );
      if (currentGen != _generation) return;

      // Snapshot before notify — user can flip sort while prefs awaits.
      final sortSnapshot = _state.sort;
      final algoSnapshot = _state.sidewaysAlgo;
      final cache = _buildRankingsCacheMap(
        result,
        sort: sortSnapshot,
        sidewaysAlgo: algoSnapshot,
      );

      final sortedItems = _sortedFromResult(result);
      _setState(
        _state.copyWith(
          isLoading: false,
          items: sortedItems,
          page: 1,
          hasMore: result.hasMore,
          snapshot: result.snapshot,
          rsAvailable: result.rsAvailable,
          effectiveSort: result.effectiveSort.isNotEmpty
              ? result.effectiveSort
              : _state.effectiveSort,
        ),
      );
      try {
        await _writeRankingsCacheIfCurrent(timeframe, currentGen, cache);
      } catch (_) {
        // Disk failure must not hide a good network result.
      }
    } catch (e) {
      if (currentGen != _generation) return;

      if (_prefs != null) {
        final cacheStr = _prefs!.getRankingsCache(timeframe);
        if (cacheStr != null) {
          try {
            final cache = jsonDecode(cacheStr) as Map<String, dynamic>;
            final cachedSort = cache['sort'] as String?;
            if (cachedSort != null && cachedSort != _state.sort) {
              // Wrong sort mode — do not show volume order under Leaders.
              throw StateError('cache sort mismatch');
            }
            final rsAvailable = parseJsonBool(cache['rsAvailable']);
            final items = (cache['items'] as List)
                .map((e) => _itemFromCacheMap(e as Map<String, dynamic>))
                .toList();
            final sorted = _sortItems(
              items,
              _state.sort,
              direction: _state.sortDirection,
              rsAvailable: rsAvailable,
            );
            final effective = cache['effectiveSort'] as String? ??
                (rsAvailable ? _state.sort : 'total');
            _setState(_state.copyWith(
              isLoading: false,
              items: sorted,
              page: 1,
              hasMore: cache['hasMore'] as bool? ?? false,
              snapshot: cache['snapshot'] as String?,
              error: 'Offline — showing cached data',
              rsAvailable: rsAvailable,
              effectiveSort: effective,
            ));
            return;
          } catch (_) {
            // Ignore cache parse / mismatch errors, fall through to error
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

      final sortSnapshot = _state.sort;
      final algoSnapshot = _state.sidewaysAlgo;
      final cache = _buildRankingsCacheMap(
        result,
        sort: sortSnapshot,
        sidewaysAlgo: algoSnapshot,
      );

      final sortedItems = _sortedFromResult(result);
      _setState(
        _state.copyWith(
          isLoading: false,
          items: sortedItems,
          page: 1,
          hasMore: result.hasMore,
          snapshot: result.snapshot,
          error: null,
          rsAvailable: result.rsAvailable,
          effectiveSort: result.effectiveSort.isNotEmpty
              ? result.effectiveSort
              : _state.effectiveSort,
        ),
      );
      try {
        await _writeRankingsCacheIfCurrent(timeframe, currentGen, cache);
      } catch (_) {
        // Disk failure must not hide a good network result.
      }
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
      // Same as favourites: do not promote RS from a later page.
      final rsAvailable = _state.rsAvailable;
      final sortedItems = _sortItems(
        merged,
        _state.sort,
        direction: _state.sortDirection,
        rsAvailable: rsAvailable,
      );
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

    final resorted = _sortItems(
      _state.items,
      _state.sort,
      direction: direction,
      rsAvailable: _state.rsAvailable,
    );
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

  /// Fetches any favourited symbols that are **not** yet present in the
  /// loaded items list. Uses the `symbols` query filter so the backend
  /// returns only the requested symbols regardless of their ranking page.
  ///
  /// If all favourites are already loaded this is a no-op.
  Future<void> loadMissingFavourites(
    String timeframe,
    Set<String> favourites,
  ) async {
    if (favourites.isEmpty) return;

    final loaded = _state.items.map((i) => i.symbol).toSet();
    final missing = favourites.difference(loaded);
    if (missing.isEmpty) return;

    final currentGen = _generation;

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: 1,
        sort: _state.sort,
        sidewaysAlgo: _state.sidewaysAlgo,
        symbols: missing.toList(),
      );

      if (currentGen != _generation) return;

      if (result.items.isEmpty) return;

      final currentSymbols = _state.items.map((i) => i.symbol).toSet();
      final newOnly =
          result.items.where((i) => !currentSymbols.contains(i.symbol));
      final merged = [..._state.items, ...newOnly];
      // Symbols-filter must not promote a fallback board to RS / re-sort A–Z.
      final rsAvailable = _state.rsAvailable;
      final sorted = _sortItems(
        merged,
        _state.sort,
        direction: _state.sortDirection,
        rsAvailable: rsAvailable,
      );

      _setState(_state.copyWith(items: sorted));
    } catch (_) {
      // Silently ignore — user still sees favourites already loaded.
    }
  }

  void _setState(OverviewState newState) {
    _state = newState;
    onChanged?.call();
  }
}
