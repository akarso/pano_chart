import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter_svg/flutter_svg.dart';
import '../../core/auto_refresh_timer.dart';
import '../../core/overview_banner.dart';
import '../../core/polling_config.dart';
// import '../../core/sequential_visual_executor.dart'; // PR-034: kept for potential future use
import '../../core/sparkline_flash_dot.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../../infrastructure/preferences_service.dart';
import '../../infrastructure/stablecoin_config.dart';
import '../bubble_map/bubble_map_screen.dart';
import '../bubble_map/bubble_map_view_model.dart';
import '../candles/application/get_candle_series.dart';
import '../events/events_view_model.dart';
import '../events/macro_events_screen.dart';
import '../fear_greed/fear_greed_dialog.dart';
import '../fear_greed/http_fear_greed_api.dart';
import '../market_state/http_composite_index_api.dart';
import '../market_state/http_market_state_api.dart';
import '../market_state/http_regime_api.dart';
import '../market_state/http_regime_history_api.dart';
import '../market_state/http_transition_api.dart';
import '../market_state/market_pulse_screen.dart';
import '../billing/billing_manager.dart';
import '../billing/upgrade_screen.dart';
import '../news/news_list_screen.dart';
import '../news/news_view_model.dart';
import '../social/social_feed_screen.dart';
import '../social/social_feed_view_model.dart';
import 'package:url_launcher/url_launcher.dart';
import '../detail/chart_navigation.dart';
import '../detail/detail_screen.dart';
import '../detail/detail_context.dart';
import '../detail/http_fragility_api.dart';
import '../detail/http_behavior_api.dart';
import '../detail/http_setup_api.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';

/// Overview widget that displays a scrollable grid of market sparklines.
///
/// All data and loading state is owned by [OverviewViewModel].
/// Widget rebuilds via [OverviewViewModel.onChanged] callback.
class OverviewWidget extends StatefulWidget {
  final OverviewViewModel viewModel;
  final GetCandleSeries getCandleSeries;
  final EventsViewModel? eventsViewModel;
  final PreferencesService? prefs;
  final BubbleMapViewModel? bubbleMapViewModel;
  final FearGreedApi? fearGreedApi;
  final MarketStateApi? marketStateApi;
  final CompositeIndexApi? compositeIndexApi;
  final RegimeApi? regimeApi;
  final TransitionApi? transitionApi;
  final RegimeHistoryApi? regimeHistoryApi;
  final StablecoinConfig stablecoins;
  final NewsViewModel? newsViewModel;
  final BillingManager? billingManager;
  final SetupApi? setupApi;
  final FragilityApi? fragilityApi;
  final BehaviorApi? behaviorApi;
  final SocialFeedViewModel? socialFeedViewModel;

  const OverviewWidget({
    Key? key,
    required this.viewModel,
    required this.getCandleSeries,
    this.eventsViewModel,
    this.prefs,
    this.bubbleMapViewModel,
    this.fearGreedApi,
    this.marketStateApi,
    this.compositeIndexApi,
    this.regimeApi,
    this.transitionApi,
    this.regimeHistoryApi,
    this.stablecoins = const StablecoinConfig({}),
    this.newsViewModel,
    this.billingManager,
    this.setupApi,
    this.fragilityApi,
    this.behaviorApi,
    this.socialFeedViewModel,
  }) : super(key: key);

  @override
  OverviewWidgetState createState() => OverviewWidgetState();
}

/// Which overlay is currently visible.
enum _OverlayKind { none, settings, menu }

class OverviewWidgetState extends State<OverviewWidget>
    with TickerProviderStateMixin {
  late final OverviewViewModel vm;
  final ScrollController _scrollController = ScrollController();
  int _columns = 2;
  String _timeframe = '1h';
  bool _normalizeSparklines = true;
  bool _hiResSparklines = true;
  bool _excludeStablecoins = true;
  bool _showFavourites = false;
  Set<String> _favourites = {};

  /// Which overlay panel is open (none by default).
  _OverlayKind _overlay = _OverlayKind.none;

  /// Threshold in pixels from bottom to trigger loading more items.
  static const double _scrollThreshold = 200.0;

  // ---- flash dot state ----
  /// Previous sparkline arrays, keyed by symbol.
  /// Captured before refresh so we can compare after.
  Map<String, List<double>> _previousSparklines = {};
  /// Per-symbol flash dot animation controllers.
  final Map<String, AnimationController> _flashControllers = {};
  /// Per-symbol flash progress (0→1→0 for the flash envelope).
  final Map<String, double> _flashProgress = {};
  /// Per-symbol flash color (green / red / neutral blue).
  final Map<String, Color> _flashColors = {};
  bool _isRefreshing = false;

  /// True while an auto-triggered refresh (not manual pull) is in flight.
  bool _isAutoRefreshing = false;

  // ---- staleness & banner ----
  final StalenessTracker _stalenessTracker = StalenessTracker();

  // ---- auto-refresh (pro only) ----
  AutoRefreshTimer? _autoRefreshTimer;

  PreferencesService? get _prefs => widget.prefs;

  /// Whether auto-refresh is enabled (pro tier).
  bool get _isProUser =>
      widget.billingManager == null || widget.billingManager!.hasFullAccess;

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;

    // ---- staleness tracker ----
    _stalenessTracker
      ..setTimeframe(_timeframe)
      ..onChanged = () {
        if (mounted) setState(() {});
      }
      ..start();

    // Attach prefs to view model for offline cache
    vm.attachPrefs(_prefs);

    // Attach prefs to events view model
    widget.eventsViewModel?.attachPrefs(_prefs);

    // Restore persisted settings.
    final p = _prefs;
    if (p != null) {
      _columns = p.columns;
      _timeframe = p.timeframe;
      _normalizeSparklines = p.normalizeSparklines;
      _hiResSparklines = p.hiResSparklines;
      _excludeStablecoins = p.excludeStablecoins;
      _favourites = p.favourites;

      // Sync sort, sidewaysAlgo, and sortDirection into the view model
      // state so the first loadInitial uses the persisted values.
      if (p.sort != vm.state.sort) {
        vm.changeSortSilent(p.sort);
      }
      if (p.sidewaysAlgo != vm.state.sidewaysAlgo) {
        vm.changeSidewaysAlgoSilent(p.sidewaysAlgo);
      }
      if (p.sortDirection != vm.state.sortDirection) {
        vm.changeSortDirectionSilent(p.sortDirection);
      }

      _stalenessTracker.setTimeframe(_timeframe);
    }

    vm.onChanged = () {
      // Detect whether this is a successful load (items present, not loading).
      final st = vm.state;
      final isOffline = st.error != null &&
          st.error!.contains('Offline') &&
          st.items.isNotEmpty;
      final isSuccessfulLoad =
          !st.isLoading && st.items.isNotEmpty && st.error == null;

      if (isOffline) {
        _stalenessTracker.markOffline();
      } else if (isSuccessfulLoad) {
        _stalenessTracker.markOnline();

        // Flash dots: compare with previous values.
        if (_isRefreshing) {
          final isAuto = _isAutoRefreshing;
          _isRefreshing = false;
          _isAutoRefreshing = false;
          _triggerFlashDots(
            st.items,
            staggerMs: isAuto ? kStaggerDelayMs : 10,
            forceFlash: isAuto,
          );
        }

        // Start auto-refresh once initial data is available (pro only).
        _maybeStartAutoRefresh(st.items.length);
      }

      setState(() {});

      // After new items are rendered, check if we still need more to fill the viewport.
      SchedulerBinding.instance.addPostFrameCallback((_) {
        _checkAndLoadMore();
      });
    };
    _scrollController.addListener(_onScroll);
    vm.loadInitial(_timeframe);
  }

  @override
  void dispose() {
    vm.onChanged = null;
    _autoRefreshTimer?.dispose();
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    _stalenessTracker.stop();
    _stopFpsMonitor();
    for (final ctrl in _flashControllers.values) {
      ctrl.dispose();
    }
    _flashControllers.clear();
    super.dispose();
  }

  // ---- scroll helpers ----

  void _onScroll() {
    _checkAndLoadMore();
  }

  void _checkAndLoadMore() {
    if (!_scrollController.hasClients) return;
    final pos = _scrollController.position;
    if (pos.pixels >= pos.maxScrollExtent - _scrollThreshold) {
      if (!vm.state.isLoading && vm.state.hasMore) {
        vm.loadNext(_timeframe);
      }
    }
  }

  // ---- FPS monitoring (kept for future use) ----

  void _startFpsMonitor() {}

  void _stopFpsMonitor() {}

  // ---- flash dot helpers ----

  /// Captures the full sparkline for each currently loaded item
  /// so we can compare after refresh.
  void _captureSparklineValues() {
    _previousSparklines = {};
    for (final item in vm.state.items) {
      if (item.sparkline.isNotEmpty) {
        _previousSparklines[item.symbol] = List<double>.of(item.sparkline);
      }
    }
  }

  /// After refresh, compares sparklines with their pre-refresh snapshots.
  ///
  /// Change detection: compares the full sparkline array, not just the last
  /// value.  Even a window-slide (new candle added, old dropped) counts.
  ///
  /// For auto-refresh ([forceFlash] = true) items with no data change still
  /// receive a subtle neutral pulse so the overview grid always looks "alive".
  ///
  /// [staggerMs] controls the delay between sequential activations
  /// (10 ms for manual pull-to-refresh, [kStaggerDelayMs] for auto-refresh).
  void _triggerFlashDots(
    List<OverviewItem> items, {
    int staggerMs = 10,
    bool forceFlash = false,
  }) {
    // Dispose old flash controllers.
    for (final ctrl in _flashControllers.values) {
      ctrl.dispose();
    }
    _flashControllers.clear();
    _flashProgress.clear();
    _flashColors.clear();

    for (var order = 0; order < items.length; order++) {
      final item = items[order];
      final prevSparkline = _previousSparklines[item.symbol];
      if (item.sparkline.isEmpty) continue;

      // Determine whether anything changed.
      bool changed = false;
      if (prevSparkline == null) {
        changed = true;
      } else if (prevSparkline.length != item.sparkline.length) {
        changed = true;
      } else {
        for (int i = 0; i < item.sparkline.length; i++) {
          if (item.sparkline[i] != prevSparkline[i]) {
            changed = true;
            break;
          }
        }
      }

      // Pick colour.
      Color color;
      if (changed && prevSparkline != null && prevSparkline.isNotEmpty) {
        // Directional: compare last values.
        final prevLast = prevSparkline.last;
        final currLast = item.sparkline.last;
        color = currLast > prevLast
            ? Colors.green
            : currLast < prevLast
                ? Colors.red
                : const Color(0xFF64B5F6); // neutral blue
      } else if (changed) {
        // Brand-new symbol — use sparkline's own direction.
        final sl = item.sparkline;
        color = sl.last > sl.first
            ? Colors.green
            : sl.last < sl.first
                ? Colors.red
                : const Color(0xFF64B5F6);
      } else if (forceFlash) {
        // No data change but auto-refresh wants a "heartbeat" pulse.
        // Use sparkline's own last-candle direction for colour.
        final sl = item.sparkline;
        if (sl.length >= 2) {
          final prev2 = sl[sl.length - 2];
          color = sl.last > prev2
              ? Colors.green
              : sl.last < prev2
                  ? Colors.red
                  : const Color(0xFF64B5F6);
        } else {
          color = const Color(0xFF64B5F6);
        }
      } else {
        // Manual refresh, nothing changed — skip.
        continue;
      }

      final delay = Duration(milliseconds: staggerMs * order);

      Future.delayed(delay, () {
        if (!mounted) return;
        final ctrl = AnimationController(
          vsync: this,
          duration: const Duration(milliseconds: 4250),
        );
        _flashControllers[item.symbol] = ctrl;
        _flashColors[item.symbol] = color;

        ctrl.addListener(() {
          if (!mounted) return;
          _flashProgress[item.symbol] = ctrl.value;
          setState(() {});
        });
        ctrl.addStatusListener((status) {
          if (status == AnimationStatus.completed) {
            _flashProgress.remove(item.symbol);
            _flashColors.remove(item.symbol);
            ctrl.dispose();
            _flashControllers.remove(item.symbol);
            if (mounted) setState(() {});
          }
        });
        ctrl.forward();
      });
    }
    _previousSparklines = {};
  }

  // ---- auto-refresh helpers (pro only) ----

  /// Initialises the auto-refresh timer the first time we have data.
  /// Subsequent calls are no-ops (the timer is already running).
  void _maybeStartAutoRefresh(int symbolCount) {
    if (!_isProUser || _autoRefreshTimer != null || symbolCount == 0) return;
    _autoRefreshTimer = AutoRefreshTimer(
      interval: overviewAutoRefreshInterval(symbolCount),
      onTick: _autoRefresh,
    );
    _autoRefreshTimer!.start();
  }

  /// Performs an auto-refresh cycle: captures sparkline values, refreshes
  /// data (the flash-dot trigger happens in the [onChanged] listener),
  /// then notifies staleness tracker.
  Future<void> _autoRefresh() async {
    _captureSparklineValues();
    _isRefreshing = true;
    _isAutoRefreshing = true;
    await vm.refresh(_timeframe);
  }

  // ---- overlay helpers ----

  void _toggleOverlay(_OverlayKind kind) {
    setState(() {
      _overlay = _overlay == kind ? _OverlayKind.none : kind;
    });
  }

  // ---- navigation helpers ----

  /// Number of candles the overview sparkline covers.
  static const int _sparklineCandles = kSparklineCandles;

  /// Indicator warmup margin — extra candles fetched for accurate indicators
  /// from the very first visible candle.  Hidden on the chart.
  static const int _indicatorWarmup = kIndicatorWarmup;

  /// Returns `true` if the user has full access (subscription or trial).
  /// When access is denied, navigates to the [UpgradeScreen] and
  /// returns `false`.
  bool _requireAccess() {
    final billing = widget.billingManager;
    // No billing manager → no gating (non-Android / tests).
    if (billing == null || billing.hasFullAccess) return true;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => UpgradeScreen(billingManager: billing),
      ),
    );
    return false;
  }

  Future<void> _onItemTapped(OverviewItem item) async {
    if (!_requireAccess()) return;
    final now = DateTime.now().toUtc();
    final input = buildDetailChartInput(
      symbol: item.symbol,
      timeframe: _timeframe,
      now: now,
    );

    // Compute rank = 1-based position in current list.
    final rankIndex = vm.state.items.indexOf(item);
    final rank = rankIndex >= 0 ? rankIndex + 1 : 0;

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (_) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      Navigator.of(context).pop();
      final result = await Navigator.of(context).push<bool>(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(item.symbol),
            timeframe: Timeframe(_timeframe),
            series: series,
            warmupCount: _indicatorWarmup,
            initialVisibleCount: _sparklineCandles,
            isFavourite: _favourites.contains(item.symbol),
            eventsViewModel: widget.eventsViewModel,
            getCandleSeries: widget.getCandleSeries,
            setupApi: widget.setupApi,
            fragilityApi: widget.fragilityApi,
            behaviorApi: widget.behaviorApi,
            isProUser: _isProUser,
            detailContext: DetailContext(
              rank: rank,
              totalScore: item.totalScore,
              trendScore: item.trendScore,
              sidewaysScore: item.sidewaysScore,
              gainScore: item.gainScore,
              compressionScore: item.compressionScore,
              breakoutUpScore: item.breakoutUpScore,
              breakoutDownScore: item.breakoutDownScore,
              volume: item.volume,
            ),
          ),
        ),
      );
      // Update favourites from detail screen result.
      if (result != null && mounted) {
        setState(() {
          if (result) {
            _favourites.add(item.symbol);
          } else {
            _favourites.remove(item.symbol);
          }
          _prefs?.favourites = _favourites;
        });
      }
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load chart: $e')),
      );
    }
  }

  // ---- pull-to-refresh ----

  Future<void> _onRefresh() async {
    _captureSparklineValues();
    _isRefreshing = true;
    await vm.refresh(_timeframe);
  }

  // ---- sparkline helpers ----

  /// Compute the maximum absolute % change across all visible sparklines.
  /// This provides a shared y-axis scale for non-normalized mode.
  double _globalMaxPctChange(OverviewState state) {
    double maxPct = 0.0;
    for (final item in state.items) {
      if (item.sparkline.length < 2 || item.sparkline.first == 0) continue;
      final first = item.sparkline.first;
      for (final p in item.sparkline) {
        final pct = ((p - first) / first).abs();
        if (pct > maxPct) maxPct = pct;
      }
    }
    // Floor at 0.1% to avoid division by zero on flat data.
    return maxPct < 0.001 ? 0.001 : maxPct;
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final state = vm.state;

    return SafeArea(
      bottom: false,
      child: Column(
        children: [
          // Nav bar + overlay unit — bottom border moves with rollout
          Container(
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: Color(0xFF1A1A2E), width: 1)),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _buildNavBar(),
                _buildOverlayPanel(state),
              ],
            ),
          ),
          Expanded(child: _buildBody(state)),
          _buildTrialBanner(),
        ],
      ),
    );
  }

  // ---- navigation bar ----

  Widget _buildNavBar() {
    const barHeight = 50.0;
    return Container(
      height: barHeight,
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Colors.black, Colors.black],
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 12),
      child: Row(
        children: [
          if (_showFavourites) ...[
            // Back arrow + title (matches Bubble Map AppBar style)
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => setState(() => _showFavourites = false),
              child: const SizedBox(
                width: 36,
                height: 44,
                child: Center(
                  child: Icon(Icons.arrow_back_ios_new,
                      color: Colors.white, size: 18),
                ),
              ),
            ),
            const Text(
              'Favourites',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w700,
                color: Color(0xFF00E6C0),
                letterSpacing: 0.5,
              ),
            ),
          ] else ...[
            // Logo + branding
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () {
                _scrollController.animateTo(0, duration: const Duration(milliseconds: 300), curve: Curves.easeOut);
              },
              child: Padding(
                padding: const EdgeInsets.only(left: 0, right: 8),
                child: Row(
                  children: [
                    Container(
                      margin: const EdgeInsets.only(right: 14),
                      child: ClipRRect(
                        borderRadius: BorderRadius.circular(3),
                        child: Image.asset(
                          'assets/icon.png',
                          width: 26,
                          height: 26,
                        ),
                      ),
                    ),
                    // const Text(
                    //   'your\nmissing\nelement',
                    //   style: TextStyle(
                    //     fontSize: 6,
                    //     fontWeight: FontWeight.w700,
                    //     color: Color(0xFF00E6C0),
                    //     letterSpacing: 0.5,
                    //   ),
                    // ),
                  ],
                ),
              ),
            ),
          ],
          const Spacer(),
          // Favourites toggle
          GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () {
              final willShow = !_showFavourites;
              setState(() => _showFavourites = willShow);
              if (willShow && _favourites.isNotEmpty) {
                vm.loadMissingFavourites(_timeframe, _favourites);
              }
            },
            child: SizedBox(
              width: 44,
              height: 44,
              child: Center(
                child: Icon(
                  _showFavourites ? Icons.star : Icons.star_border,
                  color: _showFavourites ? Colors.amber : Colors.white,
                  size: 22,
                ),
              ),
            ),
          ),
          const SizedBox(width: 8),
          // Settings icon
          _NavBarIcon(
            isActive: _overlay == _OverlayKind.settings,
            svgAsset: 'assets/gear-setting-settings.svg',
            onTap: () => _toggleOverlay(_OverlayKind.settings),
          ),
          const SizedBox(width: 8),
          // Menu icon
          _NavBarIcon(
            isActive: _overlay == _OverlayKind.menu,
            svgAsset: 'assets/menu.svg',
            onTap: () => _toggleOverlay(_OverlayKind.menu),
          ),
          const SizedBox(width: 4),
        ],
      ),
    );
  }

  // ---- overlay panel ----

  Widget _buildOverlayPanel(OverviewState state) {
    return AnimatedSize(
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeInOut,
      alignment: Alignment.topCenter,
      clipBehavior: Clip.hardEdge,
      child: _overlay != _OverlayKind.none
          ? Container(
              key: ValueKey(_overlay),
              width: double.infinity,
              decoration: const BoxDecoration(
                color: Color(0xFF1A1A2E),
              ),
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: _overlay == _OverlayKind.settings
                  ? _buildSettingsOverlay(state)
                  : _buildMenuOverlay(),
            )
          : const SizedBox.shrink(),
    );
  }

  /// Display label for a sort value.
  static String _sortLabel(String sort) {
    switch (sort) {
      case 'sideways': return 'Sideways';
      case 'compression': return 'Compression';
      case 'breakout': return 'Breakout';
      case 'trend': return 'Trend';
      case 'gain': return 'Gainers';
      case 'losers': return 'Losers';
      case 'volume': return 'Volume';
      default: return sort;
    }
  }

  Widget _buildSettingsOverlay(OverviewState state) {
    final screenWidth = MediaQuery.of(context).size.width;
    // Font size proportional to screen width (~3.5vw), floor 11, cap 16.
    final ctrlFontSize = (screenWidth * 0.035).clamp(11.0, 16.0);

    final showDirection = kDirectionalSorts.contains(state.sort);

    return DefaultTextStyle.merge(
      style: TextStyle(fontSize: ctrlFontSize),
      child: Theme(
        data: Theme.of(context).copyWith(
          dropdownMenuTheme: const DropdownMenuThemeData(),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                _controlRow('Columns', DropdownButton<int>(
                  value: _columns,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const [1, 2, 3]
                      .map((c) => DropdownMenuItem(value: c, child: Text('$c')))
                      .toList(),
                  onChanged: (v) {
                    setState(() => _columns = v ?? 2);
                    _prefs?.columns = _columns;
                    SchedulerBinding.instance.addPostFrameCallback((_) {
                      _checkAndLoadMore();
                    });
                  },
                ), ctrlFontSize),
                _controlRow('Timeframe', DropdownButton<String>(
                  value: _timeframe,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const ['1m', '5m', '15m', '1h', '4h', '1d']
                      .map((tf) => DropdownMenuItem(value: tf, child: Text(tf)))
                      .toList(),
                  onChanged: (v) {
                    setState(() => _timeframe = v ?? '1h');
                    _prefs?.timeframe = _timeframe;
                    _stalenessTracker.setTimeframe(_timeframe);
                    // Pause auto-refresh during reload; it resumes via
                    // _maybeStartAutoRefresh once new data arrives.
                    _autoRefreshTimer?.stop();
                    _autoRefreshTimer = null;
                    vm.loadInitial(_timeframe);
                  },
                ), ctrlFontSize),
                _controlRow('Sort', PopupMenuButton<String>(
                  initialValue: state.sort,
                  onSelected: (v) {
                    _prefs?.sort = v;
                    vm.changeSort(v, _timeframe);
                  },
                  itemBuilder: (context) => [
                    PopupMenuItem(value: 'sideways', child: Text('Sideways')),
                    PopupMenuItem(value: 'compression', child: Text('Compression')),
                    PopupMenuItem(value: 'breakout', child: Text('Breakout')),
                    PopupMenuItem(value: 'trend', child: Text('Trend')),
                    const PopupMenuDivider(),
                    PopupMenuItem(value: 'gain', child: Text('Gainers')),
                    PopupMenuItem(value: 'losers', child: Text('Losers')),
                    PopupMenuItem(value: 'volume', child: Text('Volume')),
                  ],
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        _sortLabel(state.sort),
                        style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                      ),
                      Icon(Icons.arrow_drop_down, color: Colors.white, size: ctrlFontSize + 4),
                    ],
                  ),
                ), ctrlFontSize),
              ],
            ),
            const SizedBox(height: 20),
            CustomPaint(
              painter: _DottedLinePainter(color: const Color(0xFF666666)),
              size: const Size(double.infinity, 1),
            ),
            const SizedBox(height: 20),
            // Row 1: Normalize (left) + Direction (right, when visible)
            Row(
              children: [
                SizedBox(
                  height: 24,
                  width: 24,
                  child: Checkbox(
                    value: _normalizeSparklines,
                    onChanged: (v) {
                      setState(() => _normalizeSparklines = v ?? true);
                      _prefs?.normalizeSparklines = _normalizeSparklines;
                    },
                    materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    visualDensity: VisualDensity.compact,
                  ),
                ),
                const SizedBox(width: 6),
                GestureDetector(
                  onTap: () {
                    setState(() => _normalizeSparklines = !_normalizeSparklines);
                    _prefs?.normalizeSparklines = _normalizeSparklines;
                  },
                  child: Text(
                    'Normalize sparklines',
                    style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  ),
                ),
                if (showDirection) ...[
                  const Spacer(),
                  _controlRow('Direction', ToggleButtons(
                    isSelected: [
                      state.sortDirection == 'up',
                      state.sortDirection == 'down',
                    ],
                    onPressed: (index) {
                      final dir = index == 0 ? 'up' : 'down';
                      _prefs?.sortDirection = dir;
                      vm.changeSortDirection(dir, _timeframe);
                    },
                    constraints: const BoxConstraints(minWidth: 36, minHeight: 28),
                    borderRadius: BorderRadius.circular(4),
                    selectedColor: Colors.white,
                    fillColor: Colors.white24,
                    color: Colors.white54,
                    children: const [
                      Icon(Icons.arrow_upward, size: 16),
                      Icon(Icons.arrow_downward, size: 16),
                    ],
                  ), ctrlFontSize),
                ],
              ],
            ),
            const SizedBox(height: 12),
            // Row 2: Exclude stablecoins (left) + Hi res (right)
            Row(
              children: [
                if (widget.stablecoins.count > 0) ...[
                  SizedBox(
                    height: 24,
                    width: 24,
                    child: Checkbox(
                      value: _excludeStablecoins,
                      onChanged: (v) {
                        setState(() => _excludeStablecoins = v ?? true);
                        _prefs?.excludeStablecoins = _excludeStablecoins;
                      },
                      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                      visualDensity: VisualDensity.compact,
                    ),
                  ),
                  const SizedBox(width: 6),
                  GestureDetector(
                    onTap: () {
                      setState(() => _excludeStablecoins = !_excludeStablecoins);
                      _prefs?.excludeStablecoins = _excludeStablecoins;
                    },
                    child: Text(
                      'Exclude stablecoins',
                      style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                    ),
                  ),
                ],
                const Spacer(),
                GestureDetector(
                  onTap: () {
                    setState(() => _hiResSparklines = !_hiResSparklines);
                    _prefs?.hiResSparklines = _hiResSparklines;
                  },
                  child: Text(
                    'Hi res',
                    style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  ),
                ),
                const SizedBox(width: 6),
                SizedBox(
                  height: 24,
                  width: 24,
                  child: Checkbox(
                    value: _hiResSparklines,
                    onChanged: (v) {
                      setState(() => _hiResSparklines = v ?? true);
                      _prefs?.hiResSparklines = _hiResSparklines;
                    },
                    materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    visualDensity: VisualDensity.compact,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _controlRow(String label, Widget dropdown, double fontSize) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label, style: TextStyle(fontSize: fontSize)),
        const SizedBox(width: 4),
        dropdown,
      ],
    );
  }

  Widget _buildMenuOverlay() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (widget.fearGreedApi != null)
          _menuRow(
            icon: Icons.speed,
            label: 'Fear & Greed',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              showFearGreedDialog(context, widget.fearGreedApi!);
            },
          ),
        if (widget.fearGreedApi != null) _menuDivider(),
        if (widget.marketStateApi != null)
          _menuRow(
            icon: Icons.pie_chart,
            label: 'Market Pulse',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => MarketPulseScreen(
                    marketStateApi: widget.marketStateApi!,
                    compositeIndexApi: widget.compositeIndexApi!,
                    regimeApi: widget.regimeApi,
                    transitionApi: widget.transitionApi,
                    regimeHistoryApi: widget.regimeHistoryApi,
                  ),
                ),
              );
            },
          ),
        if (widget.marketStateApi != null) _menuDivider(),
        if (widget.bubbleMapViewModel != null)
          _menuRow(
            icon: Icons.bubble_chart,
            label: 'Bubble Map',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              if (!_requireAccess()) return;
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => BubbleMapScreen(
                    viewModel: widget.bubbleMapViewModel!,
                    getCandleSeries: widget.getCandleSeries,
                    eventsViewModel: widget.eventsViewModel,
                    setupApi: widget.setupApi,
                    fragilityApi: widget.fragilityApi,
                    behaviorApi: widget.behaviorApi,
                    isProUser: _isProUser,
                  ),
                ),
              );
            },
          ),
        if (widget.bubbleMapViewModel != null) _menuDivider(),
        if (widget.eventsViewModel != null)
          _menuRow(
            icon: Icons.public,
            label: 'Macro Events',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              if (!_requireAccess()) return;
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => MacroEventsScreen(
                    viewModel: widget.eventsViewModel!,
                  ),
                ),
              );
            },
          ),
        if (widget.eventsViewModel != null) _menuDivider(),
        if (widget.newsViewModel != null)
          _menuRow(
            icon: Icons.article_outlined,
            label: 'News & Updates',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => NewsListScreen(
                    viewModel: widget.newsViewModel!,
                  ),
                ),
              );
            },
          ),
        if (widget.newsViewModel != null) _menuDivider(),
        if (widget.socialFeedViewModel != null)
          _menuRow(
            icon: Icons.rss_feed,
            label: 'Social Feed',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => SocialFeedScreen(
                    viewModel: widget.socialFeedViewModel!,
                  ),
                ),
              );
            },
          ),
        if (widget.socialFeedViewModel != null) _menuDivider(),
        if (widget.billingManager != null)
          _menuRow(
            icon: Icons.workspace_premium,
            label: 'Upgrade to Pro',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => UpgradeScreen(
                    billingManager: widget.billingManager!,
                  ),
                ),
              );
            },
          ),
        if (widget.billingManager != null) _menuDivider(),
        _menuRow(
          icon: Icons.info_outline,
          label: 'About',
          onTap: () => _showInfoDialog(
            title: 'About',
            content: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text('Simple market screener app showcasing a custom technical analysis algorithm. Crypto swiss army knife.'),
                const SizedBox(height: 12),
                const Text('Built, because I was lacking exactly such a set of tools for my own trading decisions.'),
                const SizedBox(height: 12),
                _linkParagraph(
                  'Read this, if you are new to crypto:',
                  'https://panocharts.com/blog.html#how_not_to_get_scammed',
                ),
                const SizedBox(height: 12),
                const Text('Nothing here is financial advice. Use at your own risk. Always do your own research.'),
              ],
            ),
          ),
        ),
        _menuDivider(),
        _menuRow(
          icon: Icons.help_outline,
          label: 'Help',
          onTap: () => _showInfoDialog(
            title: 'Help',
            content: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text('Pull down to refresh data. Tap on any chart to see detailed view with score breakdown and more info.'),
                const SizedBox(height: 12),
                const Text('Scroll to load more items (max 150 tickers). Use settings to change sort, timeframe, and other options.'),
                const SizedBox(height: 12),
                _linkParagraph(
                  'More detailed help here:',
                  'https://panocharts.com/help.html',
                ),
                const SizedBox(height: 12),
                _linkParagraph(
                  'There is also a Telegram support group:',
                  'https://t.me/panocharts',
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  /// Full-width tappable menu row.
  Widget _menuRow({
    required IconData icon,
    required String label,
    required VoidCallback onTap,
  }) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Row(
          children: [
            Icon(icon, size: 18, color: Colors.white70),
            const SizedBox(width: 12),
            Text(
              label,
              style: const TextStyle(color: Colors.white, fontSize: 14),
            ),
          ],
        ),
      ),
    );
  }

  Widget _menuDivider() {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: CustomPaint(
        size: const Size(double.infinity, 1),
        painter: _DottedLinePainter(color: const Color(0xFF666666)),
      ),
    );
  }

  void _showInfoDialog({required String title, required Widget content}) {
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: SingleChildScrollView(child: content),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

  /// Builds a paragraph with leading text and a tappable URL below it.
  Widget _linkParagraph(String text, String url) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(text),
        const SizedBox(height: 4),
        GestureDetector(
          onTap: () => launchUrl(
            Uri.parse(url),
            mode: LaunchMode.externalApplication,
          ),
          child: Text(
            url,
            style: const TextStyle(
              color: Color(0xFF00E6C0),
              decoration: TextDecoration.underline,
            ),
          ),
        ),
      ],
    );
  }

  // ---- trial banner ----

  /// Shows a bottom banner when the user is on a trial or when it has
  /// expired.  Returns a zero-height [SizedBox] when no banner is needed.
  Widget _buildTrialBanner() {
    final billing = widget.billingManager;
    if (billing == null || billing.status.active) {
      return const SizedBox.shrink();
    }
    final days = billing.trialDaysRemaining;
    final expired = !billing.hasFullAccess;

    final String label;
    final Color bg;
    if (expired) {
      label = 'Free trial expired — tap to upgrade';
      bg = const Color(0xFFB00020);
    } else {
      label = '$days day${days == 1 ? '' : 's'} left in free trial';
      bg = const Color(0xFF1A1A2E);
    }

    return GestureDetector(
      onTap: () {
        Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => UpgradeScreen(billingManager: billing),
          ),
        );
      },
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.symmetric(vertical: 8),
        color: bg,
        child: Text(
          label,
          textAlign: TextAlign.center,
          style: const TextStyle(color: Colors.white, fontSize: 12),
        ),
      ),
    );
  }

  // ---- weak-signal disclaimer ----

  /// Returns the regime-relevant score for a given sort mode.
  static double _sortRelevantScore(OverviewItem item, String sort) {
    switch (sort) {
      case 'sideways':
        return item.sidewaysScore;
      case 'trend':
        return item.trendScore.abs();
      case 'compression':
        return item.compressionScore;
      case 'breakout':
        return (item.breakoutUpScore > item.breakoutDownScore
            ? item.breakoutUpScore
            : item.breakoutDownScore);
      default:
        return item.totalScore;
    }
  }

  /// Median of the sort-relevant score for the first [n] items.
  /// Returns 0 when the list is empty so the banner never fires on an
  /// empty grid.
  static double _medianScore(List<OverviewItem> items, String sort, {int n = 5}) {
    if (items.isEmpty) return 0.0;
    final scores = items
        .take(n)
        .map((i) => _sortRelevantScore(i, sort))
        .toList()
      ..sort();
    final mid = scores.length ~/ 2;
    return scores.length.isOdd
        ? scores[mid]
        : (scores[mid - 1] + scores[mid]) / 2;
  }

  /// Threshold below which the top-ranked items are considered a weak signal.
  static const _weakSignalThreshold = 0.30;

  /// Sorts where the weak-signal disclaimer is irrelevant (not regime-based).
  static const _nonRegimeSorts = {'volume', 'gain', 'losers'};

  Widget _buildWeakSignalBanner(List<OverviewItem> items, String sort) {
    if (items.isEmpty) return const SizedBox.shrink();
    if (_nonRegimeSorts.contains(sort)) return const SizedBox.shrink();
    if (_medianScore(items, sort) >= _weakSignalThreshold) {
      return const SizedBox.shrink();
    }
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      color: const Color(0xFF3A2A00).withAlpha(200),
      child: const Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.warning_amber_rounded, color: Colors.amber, size: 14),
          SizedBox(width: 6),
          Flexible(
            child: Text(
              'Chosen regime weakly represented \u2014 results may be noisy',
              style: TextStyle(
                color: Colors.amber,
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }

  // ---- main body ----

  Widget _buildBody(OverviewState state) {
    if (state.isLoading && state.items.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }

    if (state.error != null && state.items.isEmpty) {
      return Center(child: Text(state.error!));
    }

    final allItems = state.items;
    var visibleItems = _showFavourites
        ? allItems.where((i) => _favourites.contains(i.symbol)).toList()
        : allItems;

    // Hide stablecoins when the setting is active.
    if (_excludeStablecoins && widget.stablecoins.count > 0) {
      visibleItems = visibleItems
          .where((i) => !widget.stablecoins.isStablecoin(i.symbol))
          .toList();
    }

    if (_showFavourites && visibleItems.isEmpty) {
      return const Center(
        child: Text(
          'No favourites yet.\nTap ★ on any detail screen to add.',
          textAlign: TextAlign.center,
          style: TextStyle(color: Colors.white38, fontSize: 14),
        ),
      );
    }

    final spacing = _columns == 3 ? 4.0 : 8.0;
    // Pro users never see the stale banner (auto-refresh handles it).
    // The offline banner is shown for all tiers.
    final bannerKind = _isProUser && _stalenessTracker.kind == OverviewBannerKind.stale
        ? OverviewBannerKind.none
        : _stalenessTracker.kind;
    final banner = OverviewBanner(kind: bannerKind);
    final weakBanner = _buildWeakSignalBanner(visibleItems, state.sort);

    return Column(
      children: [
        banner,
        weakBanner,
        Expanded(
          child: RefreshIndicator(
            onRefresh: _onRefresh,
            child: GridView.builder(
              physics: const AlwaysScrollableScrollPhysics(),
              controller: _scrollController,
              padding: EdgeInsets.only(
                left: 8, right: 8, top: 8,
                bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
              ),
              gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: _columns,
                crossAxisSpacing: spacing,
                mainAxisSpacing: spacing,
                childAspectRatio: 2.5,
              ),
              itemCount: visibleItems.length +
                  (!_showFavourites && state.hasMore ? 1 : 0),
              itemBuilder: (context, index) {
                if (index >= visibleItems.length) {
                  return const Center(child: CircularProgressIndicator());
                }
                final item = visibleItems[index];
                Widget child = GestureDetector(
                  onTap: () => _onItemTapped(item),
                  child: _OverviewGridItem(
                    item: item,
                    columns: _columns,
                    normalize: _normalizeSparklines,
                    hiRes: _hiResSparklines,
                    globalMaxPct: _globalMaxPctChange(state),
                    isFavourite: _favourites.contains(item.symbol),
                    sort: state.sort,
                    flashDotProgress: _flashProgress[item.symbol],
                    flashDotColor: _flashColors[item.symbol],
                  ),
                );
                return child;
              },
            ),
          ),
        ),
      ],
    );
  }
}

// ---- nav bar icon widget ----

class _NavBarIcon extends StatelessWidget {
  final bool isActive;
  final String svgAsset;
  final VoidCallback onTap;

  const _NavBarIcon({
    required this.isActive,
    required this.svgAsset,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: SizedBox(
        width: 44,
        height: 44,
        child: Center(
          child: isActive
              ? const Icon(Icons.close, color: Colors.white, size: 22)
              : SvgPicture.asset(
                  svgAsset,
                  width: 20,
                  height: 20,
                  colorFilter: const ColorFilter.mode(
                    Colors.white,
                    BlendMode.srcIn,
                  ),
                ),
        ),
      ),
    );
  }
}

/// Dominant signal type derived from score breakdown.
enum SignalType { trend, sideways, gain }

/// Returns the dominant signal for an [OverviewItem].
SignalType dominantSignal(OverviewItem item) {
  final entries = {
    SignalType.trend: item.trendScore,
    SignalType.sideways: item.sidewaysScore,
    SignalType.gain: item.gainScore,
  };
  return entries.entries.reduce((a, b) => a.value >= b.value ? a : b).key;
}

/// Parses a backend badge component string into a [SignalType].
SignalType _parseSignalType(String component) {
  switch (component) {
    case 'trend':
      return SignalType.trend;
    case 'sideways':
      return SignalType.sideways;
    case 'gain':
      return SignalType.gain;
    default:
      return SignalType.trend;
  }
}

Color _signalColor(SignalType signal, {double trendScore = 0}) {
  switch (signal) {
    case SignalType.trend:
      return trendScore >= 0 ? Colors.green : Colors.red;
    case SignalType.gain:
      return Colors.green;
    case SignalType.sideways:
      return Colors.orange;
  }
}

String _signalLabel(SignalType signal, {bool abbreviate = false, double trendScore = 0}) {
  switch (signal) {
    case SignalType.trend:
      final arrow = trendScore >= 0 ? '↑' : '↓';
      return abbreviate ? '$arrow T' : '$arrow TREND';
    case SignalType.gain:
      return abbreviate ? 'G' : 'GAIN';
    case SignalType.sideways:
      return abbreviate ? 'S' : 'SIDEWAYS';
  }
}

class _OverviewGridItem extends StatelessWidget {
  final OverviewItem item;
  final int columns;
  final bool normalize;
  final bool hiRes;
  final double globalMaxPct;
  final bool isFavourite;
  final String sort;
  final double? flashDotProgress;
  final Color? flashDotColor;

  const _OverviewGridItem({
    required this.item,
    required this.columns,
    required this.normalize,
    this.hiRes = true,
    required this.globalMaxPct,
    this.isFavourite = false,
    required this.sort,
    this.flashDotProgress,
    this.flashDotColor,
  });

  @override
  Widget build(BuildContext context) {
    final borderRadius = columns == 3 ? 6.0 : 12.0;
    return Card(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(borderRadius),
      ),
      child: AspectRatio(
        aspectRatio: 2.5,
        child: LayoutBuilder(
          builder: (context, constraints) {
            // Scale font proportionally to card width.
            final fontSize = (constraints.maxWidth * 0.08).clamp(9.0, 18.0);
            final pad = (constraints.maxWidth * 0.03).clamp(4.0, 12.0);
            return Stack(
              children: [
                Padding(
                  padding: EdgeInsets.all(pad),
                  child: _buildSparkline(
                    hiRes ? item.sparkline : _downsample(item.sparkline),
                  ),
                ),
                Positioned(
                  left: pad + 4,
                  top: pad,
                  child: Text(
                    item.symbol.replaceAll('USDT', ''),
                    style: TextStyle(
                      fontSize: fontSize,
                      fontWeight: FontWeight.w600,
                      color: Colors.white.withAlpha(
                        ((columns == 1 ? 0.9 : columns == 2 ? 0.8 : 0.7) * 255).round(),
                      ),
                      backgroundColor:
                          Colors.black.withAlpha((0.25 * 255).round()),
                    ),
                  ),
                ),
                if (item.badgeComponent.isNotEmpty)
                  Positioned(
                    right: pad + 4,
                    top: pad,
                    child: _buildBadge(item, fontSize),
                  ),
                if (isFavourite)
                  Positioned(
                    left: pad + 4,
                    bottom: pad,
                    child: Icon(
                      Icons.star,
                      color: Colors.amber.withAlpha((0.8 * 255).round()),
                      size: (fontSize * 0.8).clamp(10.0, 16.0),
                    ),
                  ),
                // Price change label (bottom-left)
                Positioned(
                  left: pad + 4,
                  bottom: pad,
                  child: Builder(
                    builder: (_) {
                      final pct = _sparklinePriceChange(item.sparkline);
                      final rounded = pct.toStringAsFixed(1);
                      // Treat ±0.0 as zero — grey, no sign.
                      final isZero = rounded == '0.0' || rounded == '-0.0';
                      final label = isZero
                          ? '0.0%'
                          : '${pct >= 0 ? '+' : ''}$rounded%';
                      final color = isZero
                          ? Colors.grey
                          : (pct >= 0 ? Colors.green : Colors.red);
                      return Text(
                        label,
                        style: TextStyle(
                          fontSize: (fontSize * 0.55).clamp(7.0, 11.0),
                          color: color,
                          fontWeight: FontWeight.w600,
                          shadows: const [
                            Shadow(color: Colors.black, blurRadius: 3),
                            Shadow(color: Colors.black, blurRadius: 3),
                          ],
                        ),
                      );
                    },
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }

  /// Compute the % price change from the first to last sparkline close.
  double _sparklinePriceChange(List<double> sparkline) {
    if (sparkline.length < 2 || sparkline.first == 0) return 0.0;
    return ((sparkline.last - sparkline.first) / sparkline.first) * 100;
  }

  /// Downsample a sparkline by averaging each pair of adjacent points.
  static List<double> _downsample(List<double> points) {
    if (points.length <= 2) return points;
    final result = <double>[];
    for (var i = 0; i < points.length - 1; i += 2) {
      result.add((points[i] + points[i + 1]) / 2);
    }
    // If odd number of points, keep the last one.
    if (points.length.isOdd) {
      result.add(points.last);
    }
    return result;
  }

  Widget _buildSparkline(List<double> points) {
    if (points.isEmpty) return const Center(child: Text('No data'));
    final sparklinePaint = CustomPaint(
      painter: SparklineRenderer(
        points,
        normalize: normalize,
        globalMaxPct: globalMaxPct,
      ),
      size: Size.infinite,
    );

    // Overlay flash dot if active.
    if (flashDotProgress != null &&
        flashDotColor != null &&
        flashDotProgress! > 0) {
      return Stack(
        children: [
          sparklinePaint,
          Positioned.fill(
            child: CustomPaint(
              painter: SparklineFlashDotPainter(
                color: flashDotColor!,
                progress: flashDotProgress!,
                points: points,
                normalize: normalize,
                globalMaxPct: globalMaxPct,
              ),
            ),
          ),
        ],
      );
    }
    return sparklinePaint;
  }


  Widget _buildBadge(OverviewItem item, double fontSize) {
    final signal = _parseSignalType(item.badgeComponent);
    final scale = columns == 1 ? 1.0 : columns == 2 ? 0.9 : 0.8;
    final badgeFontSize = (fontSize * 0.7 * scale).clamp(7.0, 12.0);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
      decoration: BoxDecoration(
        color: _signalColor(signal, trendScore: item.trendScore).withAlpha((0.8 * 255).round()),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        _signalLabel(signal, abbreviate: columns > 1, trendScore: item.trendScore),
        style: TextStyle(
          fontSize: badgeFontSize,
          fontWeight: FontWeight.bold,
          color: Colors.white,
        ),
      ),
    );
  }
}

/// Draws a simple sparkline (line chart) from a list of values.
///
/// When [normalize] is true (default), the sparkline fills the full height
/// using min-max scaling. When false, values are converted to % change
/// from the first point and scaled relative to [globalMaxPct], so that
/// visually smaller moves appear smaller.
class SparklineRenderer extends CustomPainter {
  final List<double> points;
  final bool normalize;
  final double globalMaxPct;

  SparklineRenderer(
    this.points, {
    this.normalize = true,
    this.globalMaxPct = 0.05,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2) return;

    // Use grey when the change rounds to 0.0% to stay coherent
    // with the percentage label on the card.
    final pct = points.first == 0
        ? 0.0
        : ((points.last - points.first) / points.first) * 100;
    final rounded = pct.toStringAsFixed(1);
    final isZero = rounded == '0.0' || rounded == '-0.0';
    final lineColor = isZero
        ? Colors.grey
        : (points.last >= points.first ? Colors.green : Colors.red);

    final paint = Paint()
      ..color = lineColor
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final path = Path();

    if (normalize) {
      // Original min-max normalization — fills full height.
      final minVal = points.reduce((a, b) => a < b ? a : b);
      final maxVal = points.reduce((a, b) => a > b ? a : b);
      final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);

      for (var i = 0; i < points.length; i++) {
        final x = (i / (points.length - 1)) * size.width;
        final y = size.height - ((points[i] - minVal) / range) * size.height;
        if (i == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
    } else {
      // Percentage mode — height reflects actual % change relative to
      // the global maximum % change across all visible sparklines.
      final first = points.first;
      final maxPct = globalMaxPct;

      for (var i = 0; i < points.length; i++) {
        final x = (i / (points.length - 1)) * size.width;
        final pct = first == 0 ? 0.0 : (points[i] - first) / first;
        // Map [-maxPct, +maxPct] to [height, 0] (top = +maxPct).
        var ratio = (pct + maxPct) / (2 * maxPct);
        // Clamp to [0, 1] for outliers beyond ±globalMaxPct.
        if (ratio < 0) ratio = 0;
        if (ratio > 1) ratio = 1;
        final y = size.height - ratio * size.height;
        if (i == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
    }

    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant SparklineRenderer oldDelegate) {
    return !identical(points, oldDelegate.points) ||
        normalize != oldDelegate.normalize ||
        globalMaxPct != oldDelegate.globalMaxPct;
  }
}

/// Draws a horizontal dotted line for menu dividers.
class _DottedLinePainter extends CustomPainter {
  final Color color;

  _DottedLinePainter({required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;
    const dashWidth = 4.0;
    const dashGap = 3.0;
    double x = 0;
    while (x < size.width) {
      canvas.drawLine(Offset(x, 0), Offset(x + dashWidth, 0), paint);
      x += dashWidth + dashGap;
    }
  }

  @override
  bool shouldRepaint(covariant _DottedLinePainter old) => color != old.color;
}
