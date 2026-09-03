import 'dart:ui' show VoidCallback;

import '../overview/get_overview.dart';
import '../overview/overview_state.dart';
import 'bubble_map_state.dart';
import 'bubble_packer.dart';
import 'bubble_token.dart';

/// ViewModel for the bubble map screen.
///
/// Fetches data via the existing [GetOverview] use case (backed by
/// `/api/rankings`) and repacks bubbles whenever the viewport size,
/// timeframe, page, or sort changes.
class BubbleMapViewModel {
  BubbleMapState _state = BubbleMapState.initial();
  BubbleMapState get state => _state;

  final GetOverview _getOverview;
  final BubblePacker _packer;

  VoidCallback? onChanged;

  int _generation = 0;

  /// Cached overview items before packing (so re-layout on resize doesn't
  /// require a new network call).
  List<BubbleToken> _tokens = [];

  /// Viewport dimensions used for the last pack.
  double _lastWidth = 0;
  double _lastHeight = 0;

  static const int pageSize = 50;

  BubbleMapViewModel(this._getOverview, {BubblePacker? packer})
      : _packer = packer ?? BubblePacker();

  /// Loads data and packs bubbles.
  ///
  /// Call this from the screen's [initState] or whenever timeframe / page
  /// changes.
  Future<void> load({
    required String timeframe,
    required int pageIndex,
    required double width,
    required double height,
  }) async {
    final gen = ++_generation;
    _setState(_state.copyWith(
      isLoading: true,
      timeframe: timeframe,
      pageIndex: pageIndex,
      error: null,
    ));

    try {
      final result = await _getOverview(
        timeframe: timeframe,
        page: pageIndex + 1, // API is 1-based
        sort: 'volume',
        sidewaysAlgo: 'v5',
      );

      if (gen != _generation) return; // stale response

      _tokens = _mapTokens(result.items);
      _lastWidth = width;
      _lastHeight = height;

      final packed = _packer.pack(
        _tokens,
        width: width,
        height: height,
        sizeBy: _state.sizeBy,
      );

      _setState(_state.copyWith(isLoading: false, bubbles: packed));
    } catch (e) {
      if (gen != _generation) return;
      _setState(_state.copyWith(isLoading: false, error: e.toString()));
    }
  }

  /// Re-packs existing tokens for a new viewport size without fetching.
  void relayout(double width, double height) {
    if (_tokens.isEmpty) return;
    if (width == _lastWidth && height == _lastHeight) return;

    _lastWidth = width;
    _lastHeight = height;

    final packed = _packer.pack(
      _tokens,
      width: width,
      height: height,
      sizeBy: _state.sizeBy,
    );

    _setState(_state.copyWith(bubbles: packed));
  }

  /// Switches the sizing metric and repacks.
  void changeSizeBy(String sizeBy) {
    if (sizeBy == _state.sizeBy) return;
    _setState(_state.copyWith(sizeBy: sizeBy));

    if (_tokens.isNotEmpty && _lastWidth > 0 && _lastHeight > 0) {
      final packed = _packer.pack(
        _tokens,
        width: _lastWidth,
        height: _lastHeight,
        sizeBy: sizeBy,
      );
      _setState(_state.copyWith(bubbles: packed));
    }
  }

  // ---- internal ----

  List<BubbleToken> _mapTokens(List<OverviewItem> items) {
    return items
        .map((i) => BubbleToken(
              symbol: i.symbol,
              volume: i.volume,
              priceChange: BubbleToken.priceChangeFromSparkline(i.sparkline),
              totalScore: i.totalScore,
              trendScore: i.trendScore,
              sidewaysScore: i.sidewaysScore,
              gainScore: i.gainScore,
              compressionScore: i.compressionScore,
              breakoutUpScore: i.breakoutUpScore,
              breakoutDownScore: i.breakoutDownScore,
              badgeComponent: i.badgeComponent,
            ))
        .toList();
  }

  void _setState(BubbleMapState next) {
    _state = next;
    onChanged?.call();
  }
}
