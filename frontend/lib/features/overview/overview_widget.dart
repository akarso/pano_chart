import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../core/app_lifecycle_manager.dart';
import '../../core/auto_refresh_timer.dart';
import '../../core/overview_banner.dart';
import '../../core/polling_config.dart';
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
import '../market_state/http_sector_rotation_api.dart';
import '../market_state/http_transition_api.dart';
import '../market_state/market_pulse_screen.dart';
import '../billing/billing_manager.dart';
import '../billing/capabilities.dart';
import '../billing/upgrade_screen.dart';
import '../news/news_list_screen.dart';
import '../news/news_view_model.dart';
import '../social/social_feed_screen.dart';
import '../social/social_feed_view_model.dart';
import '../notifications/notification_settings_page.dart';
import '../notifications/api/notification_config_api.dart';

import 'package:url_launcher/url_launcher.dart';

import '../detail/chart_navigation.dart';
import '../detail/detail_screen.dart';
import '../detail/detail_context.dart';
import '../detail/http_fragility_api.dart';
import '../detail/http_behavior_api.dart';
import '../detail/http_mtf_regimes_api.dart';
import '../detail/http_plan_api.dart';
import '../detail/http_setup_api.dart';
import '../scorecards/http_scorecard_api.dart';
import '../scorecards/scorecard_catalog.dart';
import '../scorecards/scorecard_data.dart';
import '../scorecards/scorecards_screen.dart';
import '../scorecards/reliability_chip.dart';
import '../volatility/http_volatility_api.dart';
import '../market_state/regime_colors.dart';
import 'aligned_badge_presentation.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';
import 'relative_strength_chip.dart';
import '../watchlist/watchlist_controller.dart';
import '../replay/replay_asof.dart';
import '../replay/replay_controller.dart';
import '../replay/replay_widgets.dart';

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
  final SectorRotationApi? sectorRotationApi;
  final StablecoinConfig stablecoins;
  final NewsViewModel? newsViewModel;
  final BillingManager? billingManager;
  final SetupApi? setupApi;
  final FragilityApi? fragilityApi;
  final BehaviorApi? behaviorApi;
  final VolatilityApi? volatilityApi;
  final MtfRegimesApi? mtfRegimesApi;
  final PlanApi? planApi;
  final SocialFeedViewModel? socialFeedViewModel;
  final NotificationConfigApi? notificationConfigApi;
  final ScorecardApi? scorecardApi;
  final ReplayController? replayController;

  /// Shared watchlist. When null, this widget owns a local controller
  /// backed by [prefs] so the grid star still paints from the cache.
  final WatchlistController? watchlist;

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
    this.sectorRotationApi,
    this.stablecoins = const StablecoinConfig({}),
    this.newsViewModel,
    this.billingManager,
    this.setupApi,
    this.fragilityApi,
    this.behaviorApi,
    this.volatilityApi,
    this.mtfRegimesApi,
    this.planApi,
    this.socialFeedViewModel,
    this.notificationConfigApi,
    this.scorecardApi,
    this.replayController,
    this.watchlist,
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
  final ScorecardCatalog _scorecards = ScorecardCatalog();

  // True while build() is showing the free-tier upgrade banner (i.e. the
  // list is capped at 15 items) — set at the end of every build so
  // _checkAndLoadMore can skip paginating for data the cap won't show.
  bool _freeTierCapActive = false;
  late final WatchlistController _watchlist;
  bool _ownsWatchlist = false;
  Set<String> _shownFavourites = {};

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

  // ---- lifecycle registration ----
  Pausable? _pausable;
  // Cached from didChangeDependencies — dispose() must not call
  // AppLifecycleScope.of(context) itself: by the time dispose() runs the
  // element may already be deactivated, and looking up an InheritedWidget
  // ancestor on a deactivated element throws ("Looking up a deactivated
  // widget's ancestor is unsafe").
  AppLifecycleManager? _lifecycleManager;

  PreferencesService? get _prefs => widget.prefs;

  Set<String> get _favourites => _watchlist.symbols;

  /// Capabilities derived from current subscription state.
  Capabilities get _capabilities =>
      Capabilities.fromBilling(widget.billingManager);

  /// Whether auto-refresh is enabled (pro tier).
  bool get _isProUser => _capabilities.isPro;

  /// Syncs the view model's entitlement flag before any fetch that might
  /// request the `?mtf=1` overlay (PR-100) — `_isProUser` is a live getter
  /// (re-evaluated from `widget.billingManager` on every access), so this
  /// must run right before each trigger rather than once, since entitlement
  /// can change mid-lifetime (purchase/restore).
  void _syncViewModelEntitlement() {
    vm.isProUser = _isProUser;
    vm.asOfUnix = widget.replayController?.asOfUnixFor(_timeframe);
  }

  /// Immediate UI / timer reaction to scrub (banner, scrubber, pause refresh).
  void _onReplayUiChanged() {
    if (!mounted) return;
    _syncAutoRefreshWithReplay();
    setState(() {});
  }

  /// Debounced (or enter/exit-immediate) fetch reload. Skipped while Pulse
  /// owns the shared controller so notification + menu routes don't double
  /// the replay rate budget.
  ///
  /// Deferred to the next frame so [ReplayController.releasePulseSurface]
  /// (called from Pulse [State.dispose]) cannot kick off Overview work
  /// mid-unmount. Recheck mounted + Pulse-foreground before reloading so
  /// an exit-while-Pulse-open still no-ops here (Pulse owns the scrub).
  void _onReplayReload() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      if (widget.replayController?.isPulseForeground == true) return;
      _syncViewModelEntitlement();
      _loadScorecards();
      vm.loadInitial(_timeframe);
    });
  }

  void _syncAutoRefreshWithReplay() {
    if (widget.replayController?.isActive == true) {
      _autoRefreshTimer?.stop();
      return;
    }
    _ensureAutoRefreshRunning();
  }

  /// Starts or resumes the Pro auto-refresh timer when appropriate.
  /// Does not depend on a successful fetch (exit-from-replay DoD).
  void _ensureAutoRefreshRunning() {
    if (!_isProUser) return;
    if (widget.replayController?.isActive == true) return;
    // Empty fail-closed replay grid still arms the timer so exit recovers.
    final count = vm.state.items.length;
    final n = count == 0 ? 1 : count;
    final interval = overviewAutoRefreshInterval(n);
    if (_autoRefreshTimer == null) {
      _autoRefreshTimer = AutoRefreshTimer(
        interval: interval,
        onTick: _autoRefresh,
      );
    } else {
      _autoRefreshTimer!.updateInterval(interval);
    }
    _autoRefreshTimer!.start();
  }

  void _attachReplay(ReplayController? c) {
    c?.addListener(_onReplayUiChanged);
    c?.addReloadListener(_onReplayReload);
  }

  void _detachReplay(ReplayController? c) {
    c?.removeListener(_onReplayUiChanged);
    c?.removeReloadListener(_onReplayReload);
  }

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;
    _bindWatchlist();
    _attachReplay(widget.replayController);
    vm.asOfUnix = widget.replayController?.asOfUnixFor(_timeframe);

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

      // Free tier: force hi-res off and fall back to a free sort mode.
      if (!_isProUser) {
        _hiResSparklines = false;
        const freeSorts = {'gain', 'losers', 'volume'};
        if (!freeSorts.contains(vm.state.sort)) {
          vm.changeSortSilent('volume');
        }
      }
    }

    vm.onChanged = () {
      // Detect whether this is a successful load (items present, not loading).
      final st = vm.state;
      final isOffline =
          st.error != null &&
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
    _syncViewModelEntitlement();
    vm.loadInitial(_timeframe);
    _loadScorecards();

    // Show the About dialog exactly once on first launch.
    if (_prefs != null && !_prefs!.hasSeenAbout) {
      _prefs!.hasSeenAbout = true;
      SchedulerBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        _showAboutDialog();
      });
    }
  }

  ScorecardSummaryItem? _badgeReliability(OverviewItem item) {
    if (item.badgeComponent.isEmpty) return null;
    final label = badgeScorecardLabel(item.badgeComponent, item.sparkline);
    return _scorecards.items[scorecardKey('badge', label)];
  }

  Future<void> _loadScorecards() {
    return _scorecards.load(
      api: widget.scorecardApi,
      timeframe: _timeframe,
      since: _scorecardSince,
      notify: () {
        if (mounted) setState(() {});
      },
    );
  }

  String get _scorecardSince {
    final asOf = widget.replayController?.asOfFor(_timeframe);
    return asOf != null ? replayScorecardSince(asOf) : '30d';
  }

  @override
  void didUpdateWidget(covariant OverviewWidget oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.replayController, widget.replayController)) {
      _detachReplay(oldWidget.replayController);
      _attachReplay(widget.replayController);
      _syncViewModelEntitlement();
      if (mounted) setState(() {});
    }
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final newManager = AppLifecycleScope.of(context);
    if (identical(newManager, _lifecycleManager)) return;

    // The manager instance changed — e.g. this widget was reparented under
    // a different AppLifecycleScope. Move the registration instead of
    // relying on the old "only ever register once" guard, which left
    // _pausable registered on the OLD manager forever (never removed —
    // dispose() only ever unregisters from whatever _lifecycleManager
    // currently points to) while _lifecycleManager itself had already
    // moved on to the new one.
    if (_pausable != null) {
      _lifecycleManager?.removePausable(_pausable!);
    }
    _lifecycleManager = newManager;
    if (newManager != null) {
      _pausable ??= Pausable(
        onPause: () {
          _autoRefreshTimer?.stop();
          _stalenessTracker.stop();
        },
        onResume: () {
          _syncAutoRefreshWithReplay();
          _stalenessTracker.start();
          // A notification shade is inactive → resumed and never paused.
          // That transition restarts timers and does not spend a watchlist
          // GET from the rate-limit burst.
          if (_lifecycleManager?.resumeFollowsBackground ?? false) {
            _watchlist.reconcile();
          }
        },
      );
      newManager.addPausable(_pausable!);
    }
  }

  @override
  void dispose() {
    _detachReplay(widget.replayController);
    _watchlist.removeListener(_onWatchlistChanged);
    if (_ownsWatchlist) _watchlist.dispose();
    if (_pausable != null) _lifecycleManager?.removePausable(_pausable!);
    vm.onChanged = null;
    _autoRefreshTimer?.dispose();
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    _stalenessTracker.stop();
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
      // Skip while the free-tier cap is showing — the grid's
      // maxScrollExtent is tiny (15 items + banner), so "bottom" is
      // reached almost immediately, and there's no point fetching more
      // data the cap won't display anyway — see PR-077 CR follow-up.
      if (!vm.state.isLoading && vm.state.hasMore && !_freeTierCapActive) {
        _syncViewModelEntitlement();
        vm.loadNext(_timeframe);
      }
    }
  }

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
    if (!_isProUser || symbolCount == 0) return;
    if (widget.replayController?.isActive == true) return;
    final interval = overviewAutoRefreshInterval(symbolCount);
    if (_autoRefreshTimer != null) {
      _autoRefreshTimer!.updateInterval(interval);
      _autoRefreshTimer!.start();
      return;
    }
    _autoRefreshTimer = AutoRefreshTimer(
      interval: interval,
      onTick: _autoRefresh,
    );
    _autoRefreshTimer!.start();
  }

  /// Performs an auto-refresh cycle: captures sparkline values, refreshes
  /// data (the flash-dot trigger happens in the [onChanged] listener),
  /// then notifies staleness tracker.
  Future<void> _autoRefresh() async {
    if (widget.replayController?.isActive == true) return;
    _captureSparklineValues();
    _isRefreshing = true;
    _isAutoRefreshing = true;
    _syncViewModelEntitlement();
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
  /// returns `false`. Fails closed: no billing manager means no access,
  /// not unconditional access — see PR-078. A null billing manager still
  /// can't be pushed through to [UpgradeScreen] (it requires one), so
  /// that case just blocks navigation with nothing to show the user —
  /// same tradeoff already accepted for the free-tier upgrade banner's
  /// tap handler.
  bool _requireAccess() {
    final billing = widget.billingManager;
    if (billing != null && billing.hasFullAccess) return true;
    if (billing == null) return false;
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => UpgradeScreen(billingManager: billing)),
    );
    return false;
  }

  Future<void> _onItemTapped(OverviewItem item) async {
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
      await Navigator.of(context).push<bool>(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(item.symbol),
            timeframe: Timeframe(_timeframe),
            series: series,
            warmupCount: _indicatorWarmup,
            initialVisibleCount: _sparklineCandles,
            isFavourite: _watchlist.contains(item.symbol),
            eventsViewModel: _isProUser ? widget.eventsViewModel : null,
            socialFeedViewModel: _isProUser ? widget.socialFeedViewModel : null,
            getCandleSeries: widget.getCandleSeries,
            setupApi: _isProUser ? widget.setupApi : null,
            fragilityApi: _isProUser ? widget.fragilityApi : null,
            behaviorApi: _isProUser ? widget.behaviorApi : null,
            volatilityApi: _isProUser ? widget.volatilityApi : null,
            mtfRegimesApi: _isProUser ? widget.mtfRegimesApi : null,
            planApi: _isProUser ? widget.planApi : null,
            scorecardApi: widget.scorecardApi,
            isProUser: _isProUser,
            watchlist: _watchlist,
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
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop();
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Failed to load chart: $e')));
    }
  }

  void _bindWatchlist() {
    final shared = widget.watchlist;
    if (shared != null) {
      _watchlist = shared;
    } else {
      _watchlist = WatchlistController(prefs: widget.prefs);
      _ownsWatchlist = true;
    }
    _shownFavourites = Set.of(_watchlist.symbols);
    _watchlist.addListener(_onWatchlistChanged);
    _watchlist.reconcile();
  }

  void _onWatchlistChanged() {
    if (!mounted) return;
    final next = _watchlist.symbols;
    final changed = !setEquals(next, _shownFavourites);
    _shownFavourites = Set.of(next);
    setState(() {});
    if (changed && _showFavourites && next.isNotEmpty) {
      _syncViewModelEntitlement();
      vm.loadMissingFavourites(_timeframe, next);
    }
    if (_watchlist.statusMessage == null) return;
    WidgetsBinding.instance.addPostFrameCallback((_) => _showWatchlistStatus());
  }

  void _showWatchlistStatus() {
    if (!mounted) return;
    final message = _watchlist.takeStatus();
    if (message == null) return;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(message)));
  }

  /// Toggles [symbol] on the shared watchlist. The controller repaints
  /// every listener. A rejection is reported here when this route is
  /// still mounted; otherwise the post-frame status handler shows it.
  void _toggleWatchlist(String symbol) {
    _watchlist.toggle(symbol).then((_) {
      if (!mounted) return;
      _showWatchlistStatus();
    });
  }

  // ---- pull-to-refresh ----

  Future<void> _onRefresh() async {
    _captureSparklineValues();
    _isRefreshing = true;
    _syncViewModelEntitlement();
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
    final replay = widget.replayController;
    final asOf = replay?.asOfFor(_timeframe);

    return SafeArea(
      bottom: false,
      child: Column(
        children: [
          // Nav bar + overlay unit — bottom border moves with rollout
          Container(
            decoration: const BoxDecoration(
              border: Border(
                bottom: BorderSide(color: Color(0xFF1A1A2E), width: 1),
              ),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [_buildNavBar(), _buildOverlayPanel(state)],
            ),
          ),
          if (asOf != null)
            ReplayBanner(
              asOf: asOf,
              onExit: () => replay?.exit(),
              footnote: kReplayBannerFootnote,
            ),
          Expanded(child: _buildBody(state)),
          if (asOf != null && replay != null)
            ReplayScrubber(controller: replay, timeframe: _timeframe),
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
      child: LayoutBuilder(
        builder: (context, constraints) {
          final tight = constraints.maxWidth < 340;
          final iconBox = tight ? 36.0 : 44.0;
          return Row(
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
                      child: Icon(
                        Icons.arrow_back_ios_new,
                        color: Colors.white,
                        size: 18,
                      ),
                    ),
                  ),
                ),
                const Text(
                  'Watchlist',
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
                    _scrollController.animateTo(
                      0,
                      duration: const Duration(milliseconds: 300),
                      curve: Curves.easeOut,
                    );
                  },
                  child: Padding(
                    padding: const EdgeInsets.only(left: 0, right: 8),
                    child: Row(
                      children: [
                        Container(
                          margin: EdgeInsets.only(right: tight ? 4 : 14),
                          child: ClipRRect(
                            borderRadius: BorderRadius.circular(3),
                            child: Image.asset(
                              'assets/icon.png',
                              width: 26,
                              height: 26,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
              const Spacer(),
              // "Watchlist" filter chip (ROADMAP PR-101) — filters the grid to
              // starred symbols, same underlying _showFavourites flag/filtering
              // logic the app already had (the "favourites" star doubles as
              // the backend-synced watchlist, see the toggle methods above).
              Flexible(
                child: Align(
                  alignment: Alignment.centerRight,
                  child: FittedBox(
                    fit: BoxFit.scaleDown,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 3),
                      child: FilterChip(
                        label: const Text('Watchlist'),
                        avatar: Icon(
                          _showFavourites ? Icons.star : Icons.star_border,
                          size: 16,
                          color: _showFavourites
                              ? Colors.black
                              : Colors.white70,
                        ),
                        selected: _showFavourites,
                        showCheckmark: false,
                        visualDensity: VisualDensity.compact,
                        materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                        labelStyle: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: _showFavourites ? Colors.black : Colors.white,
                        ),
                        backgroundColor: Colors.white.withAlpha(
                          (0.08 * 255).round(),
                        ),
                        selectedColor: Colors.amber,
                        onSelected: (selected) {
                          final willShow = selected;
                          setState(() => _showFavourites = willShow);
                          if (willShow && _favourites.isNotEmpty) {
                            _syncViewModelEntitlement();
                            vm.loadMissingFavourites(_timeframe, _favourites);
                          }
                        },
                      ),
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              if (_isProUser && widget.replayController != null)
                IconButton(
                  key: const Key('replay-toggle'),
                  tooltip: widget.replayController!.isActive
                      ? 'Exit replay'
                      : 'Replay',
                  padding: EdgeInsets.zero,
                  constraints: BoxConstraints.tightFor(
                    width: iconBox,
                    height: iconBox,
                  ),
                  icon: Icon(
                    Icons.history,
                    size: tight ? 20 : 22,
                    color: widget.replayController!.isActive
                        ? Colors.lightBlueAccent
                        : Colors.white70,
                  ),
                  onPressed: () {
                    final replay = widget.replayController!;
                    if (replay.isActive) {
                      replay.exit();
                    } else {
                      replay.enter(timeframe: _timeframe);
                    }
                  },
                ),
              if (_isProUser && widget.replayController != null)
                const SizedBox(width: 4),
              // Settings icon
              _NavBarIcon(
                key: const ValueKey('overview-settings-nav-icon'),
                isActive: _overlay == _OverlayKind.settings,
                svgAsset: 'assets/gear-setting-settings.svg',
                onTap: () => _toggleOverlay(_OverlayKind.settings),
                boxSize: iconBox,
              ),
              const SizedBox(width: 8),
              // Menu icon
              _NavBarIcon(
                key: const ValueKey('overview-menu-nav-icon'),
                isActive: _overlay == _OverlayKind.menu,
                svgAsset: 'assets/menu.svg',
                onTap: () => _toggleOverlay(_OverlayKind.menu),
                boxSize: iconBox,
              ),
              const SizedBox(width: 4),
            ],
          );
        },
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
              decoration: const BoxDecoration(color: Color(0xFF1A1A2E)),
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: _overlay == _OverlayKind.settings
                  ? _buildSettingsOverlay(state)
                  : _buildMenuOverlay(),
            )
          : const SizedBox.shrink(),
    );
  }

  /// Display label for the active sort. Delegates to [overviewSortMenuLabel].
  static String _sortLabel(OverviewState state) => overviewSortMenuLabel(state);

  Widget _buildSettingsOverlay(OverviewState state) {
    final screenWidth = MediaQuery.of(context).size.width;
    // Font size proportional to screen width (~3.5vw), floor 11, cap 16.
    final ctrlFontSize = (screenWidth * 0.035).clamp(11.0, 16.0);

    final showDirection = kDirectionalSorts.contains(state.sort);

    return DefaultTextStyle.merge(
      style: TextStyle(fontSize: ctrlFontSize),
      child: Theme(
        data: Theme.of(
          context,
        ).copyWith(dropdownMenuTheme: const DropdownMenuThemeData()),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                _controlRow(
                  'Columns',
                  DropdownButton<int>(
                    value: _columns,
                    isDense: true,
                    style: TextStyle(
                      fontSize: ctrlFontSize,
                      color: Colors.white,
                    ),
                    items: const [1, 2, 3]
                        .map(
                          (c) => DropdownMenuItem(value: c, child: Text('$c')),
                        )
                        .toList(),
                    onChanged: (v) {
                      setState(() => _columns = v ?? 2);
                      _prefs?.columns = _columns;
                      SchedulerBinding.instance.addPostFrameCallback((_) {
                        _checkAndLoadMore();
                      });
                    },
                  ),
                  ctrlFontSize,
                ),
                _controlRow(
                  'Timeframe',
                  DropdownButton<String>(
                    value: _timeframe,
                    isDense: true,
                    style: TextStyle(
                      fontSize: ctrlFontSize,
                      color: Colors.white,
                    ),
                    items: const ['1m', '5m', '15m', '1h', '4h', '1d']
                        .map(
                          (tf) => DropdownMenuItem(value: tf, child: Text(tf)),
                        )
                        .toList(),
                    onChanged: (v) {
                      setState(() => _timeframe = v ?? '1h');
                      _prefs?.timeframe = _timeframe;
                      _stalenessTracker.setTimeframe(_timeframe);
                      // Pause auto-refresh during reload; it resumes via
                      // _maybeStartAutoRefresh once new data arrives (or
                      // immediately on exit-from-replay via
                      // _syncAutoRefreshWithReplay).
                      _autoRefreshTimer?.stop();
                      final replay = widget.replayController;
                      if (replay?.isActive == true) {
                        // Re-align for the new TF. When setAsOf changes the
                        // instant, the reload listener fetches once; when it
                        // no-ops, reload here.
                        final changed = replay!.setAsOf(
                          _timeframe,
                          replay.asOf!,
                          immediateReload: true,
                        );
                        if (!changed) {
                          _syncViewModelEntitlement();
                          _loadScorecards();
                          vm.loadInitial(_timeframe);
                        }
                      } else {
                        _loadScorecards();
                        _syncViewModelEntitlement();
                        vm.loadInitial(_timeframe);
                      }
                    },
                  ),
                  ctrlFontSize,
                ),
                _controlRow(
                  'Sort',
                  PopupMenuButton<String>(
                    initialValue: state.sort,
                    onSelected: (v) {
                      _prefs?.sort = v;
                      _syncViewModelEntitlement();
                      vm.changeSort(v, _timeframe);
                    },
                    itemBuilder: (context) => [
                      if (_isProUser) ...[
                        const PopupMenuItem(
                          value: 'sideways',
                          child: Text('Sideways'),
                        ),
                        const PopupMenuItem(
                          value: 'compression',
                          child: Text('Compression'),
                        ),
                        const PopupMenuItem(
                          value: 'breakout',
                          child: Text('Breakout'),
                        ),
                        const PopupMenuItem(
                          value: 'trend',
                          child: Text('Trend'),
                        ),
                        const PopupMenuItem(
                          value: 'leaders',
                          child: Text('Leaders (vs market)'),
                        ),
                        const PopupMenuItem(
                          value: 'laggards',
                          child: Text('Laggards (vs market)'),
                        ),
                        const PopupMenuItem(
                          value: 'aligned',
                          child: Text('Aligned'),
                        ),
                        const PopupMenuDivider(),
                      ],
                      const PopupMenuItem(
                        value: 'gain',
                        child: Text('Gainers'),
                      ),
                      const PopupMenuItem(
                        value: 'losers',
                        child: Text('Losers'),
                      ),
                      const PopupMenuItem(
                        value: 'volume',
                        child: Text('Volume'),
                      ),
                    ],
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(
                          _sortLabel(state),
                          style: TextStyle(
                            fontSize: ctrlFontSize,
                            color: Colors.white,
                          ),
                        ),
                        Icon(
                          Icons.arrow_drop_down,
                          color: Colors.white,
                          size: ctrlFontSize + 4,
                        ),
                      ],
                    ),
                  ),
                  ctrlFontSize,
                ),
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
                    setState(
                      () => _normalizeSparklines = !_normalizeSparklines,
                    );
                    _prefs?.normalizeSparklines = _normalizeSparklines;
                  },
                  child: Text(
                    'Normalize sparklines',
                    style: TextStyle(
                      fontSize: ctrlFontSize,
                      color: Colors.white,
                    ),
                  ),
                ),
                if (showDirection) ...[
                  const Spacer(),
                  _controlRow(
                    'Direction',
                    ToggleButtons(
                      isSelected: [
                        state.sortDirection == 'up',
                        state.sortDirection == 'down',
                      ],
                      onPressed: (index) {
                        final dir = index == 0 ? 'up' : 'down';
                        _prefs?.sortDirection = dir;
                        vm.changeSortDirection(dir, _timeframe);
                      },
                      constraints: const BoxConstraints(
                        minWidth: 36,
                        minHeight: 28,
                      ),
                      borderRadius: BorderRadius.circular(4),
                      selectedColor: Colors.white,
                      fillColor: Colors.white24,
                      color: Colors.white54,
                      children: const [
                        Icon(Icons.arrow_upward, size: 16),
                        Icon(Icons.arrow_downward, size: 16),
                      ],
                    ),
                    ctrlFontSize,
                  ),
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
                      setState(
                        () => _excludeStablecoins = !_excludeStablecoins,
                      );
                      _prefs?.excludeStablecoins = _excludeStablecoins;
                    },
                    child: Text(
                      'Exclude stablecoins',
                      style: TextStyle(
                        fontSize: ctrlFontSize,
                        color: Colors.white,
                      ),
                    ),
                  ),
                ],
                const Spacer(),
                if (_isProUser) ...[
                  GestureDetector(
                    onTap: () {
                      setState(() => _hiResSparklines = !_hiResSparklines);
                      _prefs?.hiResSparklines = _hiResSparklines;
                    },
                    child: Text(
                      'Hi res',
                      style: TextStyle(
                        fontSize: ctrlFontSize,
                        color: Colors.white,
                      ),
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
              ],
            ),
            if (widget.scorecardApi != null) ...[
              const SizedBox(height: 12),
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton.icon(
                  onPressed: () {
                    setState(() => _overlay = _OverlayKind.none);
                    Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => ScorecardsScreen(
                          api: widget.scorecardApi!,
                          timeframe: _timeframe,
                          since: _scorecardSince,
                        ),
                      ),
                    );
                  },
                  icon: const Icon(Icons.verified_outlined, size: 18),
                  label: Text(
                    'Reliability',
                    style: TextStyle(fontSize: ctrlFontSize),
                  ),
                ),
              ),
            ],
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
    final caps = _capabilities;
    final billing = widget.billingManager;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        // ── Fear & Greed ──
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

        // ── Bubble Map (free: volume only) ──
        if (widget.bubbleMapViewModel != null)
          _menuRow(
            icon: Icons.bubble_chart,
            label: 'Bubble Map',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => BubbleMapScreen(
                    viewModel: widget.bubbleMapViewModel!,
                    getCandleSeries: widget.getCandleSeries,
                    eventsViewModel: widget.eventsViewModel,
                    setupApi: widget.setupApi,
                    fragilityApi: widget.fragilityApi,
                    behaviorApi: widget.behaviorApi,
                    volatilityApi: widget.volatilityApi,
                    // Gated here (unlike the sibling APIs above, which are
                    // pre-existing behavior out of scope for this PR — see
                    // PR-100 CR): a free user can reach Bubble Map and tap
                    // through to a symbol detail screen, so this new
                    // pro-tier field must not ride along ungated.
                    mtfRegimesApi: _isProUser ? widget.mtfRegimesApi : null,
                    planApi: _isProUser ? widget.planApi : null,
                    scorecardApi: widget.scorecardApi,
                    isProUser: _isProUser,
                    watchlist: _watchlist,
                  ),
                ),
              );
            },
          ),
        if (widget.bubbleMapViewModel != null) _menuDivider(),

        // ── Market Pulse (gated) ──
        if (widget.marketStateApi != null)
          _menuRow(
            icon: Icons.pie_chart,
            label: 'Market Pulse',
            onTap: () async {
              setState(() => _overlay = _OverlayKind.none);
              if (!_requireAccess()) return;
              await Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => MarketPulseScreen(
                    marketStateApi: widget.marketStateApi!,
                    compositeIndexApi: widget.compositeIndexApi!,
                    regimeApi: widget.regimeApi,
                    transitionApi: widget.transitionApi,
                    regimeHistoryApi: widget.regimeHistoryApi,
                    sectorRotationApi: widget.sectorRotationApi,
                    scorecardApi: widget.scorecardApi,
                    isProUser: _isProUser,
                    replayController: widget.replayController,
                    initialTimeframe: _timeframe,
                  ),
                ),
              );
              // Pulse releasePulseSurface already triggers one Overview reload.
              if (!mounted) return;
              _syncAutoRefreshWithReplay();
              setState(() {});
            },
          ),
        if (widget.marketStateApi != null) _menuDivider(),

        // ── Macro Events (free: limited events) ──
        if (widget.eventsViewModel != null)
          _menuRow(
            icon: Icons.public,
            label: 'Macro Events',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => MacroEventsScreen(
                    viewModel: widget.eventsViewModel!,
                    isProUser: _isProUser,
                  ),
                ),
              );
            },
          ),
        if (widget.eventsViewModel != null) _menuDivider(),

        // ── Social Feed (gated) ──
        if (widget.socialFeedViewModel != null)
          _menuRow(
            icon: Icons.rss_feed,
            label: 'Social Feed',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              if (!_requireAccess()) return;
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) =>
                      SocialFeedScreen(viewModel: widget.socialFeedViewModel!),
                ),
              );
            },
          ),
        if (widget.socialFeedViewModel != null) _menuDivider(),

        // ── News & Updates (free) ──
        if (widget.newsViewModel != null)
          _menuRow(
            icon: Icons.article_outlined,
            label: 'News & Updates',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) =>
                      NewsListScreen(viewModel: widget.newsViewModel!),
                ),
              );
            },
          ),
        if (widget.newsViewModel != null) _menuDivider(),

        // ── Notifications ──
        if (widget.prefs != null)
          _menuRow(
            icon: Icons.notifications_outlined,
            label: 'Notifications',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => NotificationSettingsPage(
                    prefs: widget.prefs!,
                    configApi: widget.notificationConfigApi,
                    isPro: caps.notificationsFull,
                  ),
                ),
              );
            },
          ),
        if (widget.prefs != null) _menuDivider(),

        // ── About ──
        _menuRow(
          icon: Icons.info_outline,
          label: 'About',
          onTap: () => _showAboutDialog(),
        ),
        _menuDivider(),

        // ── Help ──
        _menuRow(
          icon: Icons.help_outline,
          label: 'Help',
          onTap: () => _showInfoDialog(
            title: 'Help',
            content: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text(
                  'Pull down to refresh data. Tap on any chart to see detailed view with score breakdown and more info.',
                ),
                const SizedBox(height: 12),
                const Text(
                  'Scroll to load more items (max 150 tickers). Use settings to change sort, timeframe, and other options.',
                ),
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

        // ── Upgrade to Pro (conditional CTA) ──
        if (billing != null && !billing.isTrialMode) ...[
          _menuDivider(),
          _menuRow(
            icon: Icons.workspace_premium,
            label: billing.hasFullAccess
                ? 'Manage Subscription'
                : (billing.trialDaysRemaining == 0 ? 'Resume Pro' : 'Get Pro'),
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => UpgradeScreen(billingManager: billing),
                ),
              );
            },
          ),
        ],

        // ── Debug billing toggle (debug builds only) ──
        if (kDebugMode && billing != null) ...[
          _menuDivider(),
          _menuRow(
            icon: Icons.bug_report,
            label:
                'Debug: ${billing.debugOverrideLabel ?? "REAL (${_isProUser ? "pro" : "free"})"}',
            onTap: () {
              setState(() => _overlay = _OverlayKind.none);
              _showDebugBillingPicker(billing);
            },
          ),
        ],
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

  void _showAboutDialog() async {
    String version = 'unknown';
    try {
      final info = await PackageInfo.fromPlatform();
      version = info.version;
    } catch (_) {}
    if (!mounted) return;
    _showInfoDialog(
      title: 'About',
      content: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            'Version $version',
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
          const SizedBox(height: 8),
          const Text(
            'Simple market screener app showcasing a custom technical analysis algorithm. Crypto swiss army knife.',
          ),
          const SizedBox(height: 12),
          const Text(
            'Built, because I was lacking exactly such a set of tools for my own trading decisions.',
          ),
          const SizedBox(height: 12),
          const Text(
            'For a start, explore sparkline charts or one of the menu options.',
          ),
          const SizedBox(height: 12),
          const Text(
            'There is online help and onboarding available in the menu.',
          ),
          const SizedBox(height: 12),
          _linkParagraph(
            'Also read this, if you are new to crypto:',
            'https://panocharts.com/blog.html#how_not_to_get_scammed',
          ),
          const SizedBox(height: 12),
          const Text(
            'Nothing here is financial advice. Use at your own risk. Always do your own research.',
          ),
        ],
      ),
    );
  }

  void _showDebugBillingPicker(BillingManager billing) {
    showDialog(
      context: context,
      builder: (_) => SimpleDialog(
        title: const Text('Debug: Billing State'),
        children: [
          SimpleDialogOption(
            onPressed: () {
              billing.debugSetAccess(fullAccess: null, label: null);
              Navigator.pop(context);
              setState(() {});
            },
            child: const Text('🔄 Real (use actual billing)'),
          ),
          SimpleDialogOption(
            onPressed: () {
              billing.debugSetAccess(fullAccess: true, label: 'PRO');
              Navigator.pop(context);
              setState(() {});
            },
            child: const Text('⭐ Pro (all features)'),
          ),
          SimpleDialogOption(
            onPressed: () {
              billing.debugSetAccess(fullAccess: false, label: 'FREE');
              Navigator.pop(context);
              setState(() {});
            },
            child: const Text('🔒 Free (restricted)'),
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
          onTap: () =>
              launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication),
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
  static double _medianScore(
    List<OverviewItem> items,
    String sort, {
    int n = 5,
  }) {
    if (items.isEmpty) return 0.0;
    final scores =
        items.take(n).map((i) => _sortRelevantScore(i, sort)).toList()..sort();
    final mid = scores.length ~/ 2;
    return scores.length.isOdd
        ? scores[mid]
        : (scores[mid - 1] + scores[mid]) / 2;
  }

  /// Threshold below which the top-ranked items are considered a weak signal.
  static const _weakSignalThreshold = 0.30;

  /// Sorts where the weak-signal disclaimer is irrelevant (not regime-based).
  static const _nonRegimeSorts = {
    'volume',
    'gain',
    'losers',
    'leaders',
    'laggards',
  };

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

    // Free tier: cap visible tokens to 15 (favourites view is unrestricted
    // so users always see their picks). showUpgradeBanner drives a single
    // unobtrusive tile at the cutoff point (not a modal/interstitial —
    // matches the "no aggressive upselling" principle from PR-039) so the
    // cap reads as a paywall, not a bug ("where are the rest of my
    // tokens?").
    var showUpgradeBanner = false;
    int hiddenTokenCount = 0;
    if (!_showFavourites &&
        !_capabilities.fullTokenList &&
        visibleItems.length > 15) {
      hiddenTokenCount = visibleItems.length - 15;
      visibleItems = visibleItems.sublist(0, 15);
      showUpgradeBanner = true;
    }
    // Mirrored into a field so _checkAndLoadMore (outside build) can skip
    // paginating for data the free-tier cap won't show anyway — see PR-077
    // CR follow-up.
    _freeTierCapActive = showUpgradeBanner;

    if (_showFavourites && visibleItems.isEmpty) {
      return const Center(
        child: Text(
          'Your watchlist is empty.\nTap ★ on any tile or detail screen to add.',
          textAlign: TextAlign.center,
          style: TextStyle(color: Colors.white38, fontSize: 14),
        ),
      );
    }

    final spacing = _columns == 3 ? 4.0 : 8.0;
    // Pro users never see the stale banner (auto-refresh handles it).
    // The offline banner is shown for all tiers.
    final bannerKind =
        _isProUser && _stalenessTracker.kind == OverviewBannerKind.stale
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
                left: 8,
                right: 8,
                top: 8,
                bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
              ),
              gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: _columns,
                crossAxisSpacing: spacing,
                mainAxisSpacing: spacing,
                childAspectRatio: 2.5,
              ),
              itemCount:
                  visibleItems.length +
                  (showUpgradeBanner ? 1 : 0) +
                  // Suppress the infinite-scroll loading tile once the free-tier
                  // cap has already kicked in — there's nothing more to page in
                  // for this view, and a spinner right after a hard cutoff would
                  // read as "still loading" rather than "upgrade for more".
                  (!_showFavourites && !showUpgradeBanner && state.hasMore
                      ? 1
                      : 0),
              itemBuilder: (context, index) {
                if (showUpgradeBanner && index == visibleItems.length) {
                  return _UpgradeBannerTile(
                    hiddenCount: hiddenTokenCount,
                    columns: _columns,
                    onTap: () {
                      // billingManager can legitimately be null here as of
                      // PR-078: Capabilities.fromBilling(null) now fails
                      // closed to .free() (was .pro()), so this tile can
                      // show even without a billing manager to launch a
                      // purchase flow through (billing unavailable, e.g. a
                      // future iOS build before billing lands there, or a
                      // test). Nothing to do in that case — there's no
                      // UpgradeScreen to navigate to without one.
                      final billing = widget.billingManager;
                      if (billing == null) return;
                      Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) =>
                              UpgradeScreen(billingManager: billing),
                        ),
                      );
                    },
                  );
                }
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
                    reliability: _badgeReliability(item),
                    rsAvailable: state.rsAvailable,
                    showRsChip: _isProUser,
                    showAlignmentBadge: _isProUser,
                    onToggleWatchlist: () => _toggleWatchlist(item.symbol),
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

// ---- free-tier upgrade banner tile ----

/// A single grid cell shown at the free-tier 15-token cutoff, in place of
/// silently truncating the list — see PR-077. Styled to sit naturally among
/// the surrounding [_OverviewGridItem] cards (same [Card]/[AspectRatio]
/// shape) rather than as a modal or interstitial.
class _UpgradeBannerTile extends StatelessWidget {
  final int hiddenCount;
  final int columns;
  final VoidCallback onTap;

  const _UpgradeBannerTile({
    required this.hiddenCount,
    required this.columns,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final borderRadius = columns == 3 ? 6.0 : 12.0;
    return GestureDetector(
      onTap: onTap,
      child: Card(
        color: const Color(0xFF00E6C0).withAlpha((0.12 * 255).round()),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(borderRadius),
          side: const BorderSide(color: Color(0xFF00E6C0), width: 1),
        ),
        child: AspectRatio(
          aspectRatio: 2.5,
          child: LayoutBuilder(
            builder: (context, constraints) {
              final fontSize = (constraints.maxWidth * 0.08).clamp(9.0, 16.0);
              return Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(
                      Icons.lock_outline,
                      color: const Color(0xFF00E6C0),
                      size: fontSize * 1.4,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '+$hiddenCount more tokens with Pro',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: fontSize,
                        fontWeight: FontWeight.w600,
                        color: const Color(0xFF00E6C0),
                      ),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
      ),
    );
  }
}

// ---- nav bar icon widget ----

class _NavBarIcon extends StatelessWidget {
  final bool isActive;
  final String svgAsset;
  final VoidCallback onTap;

  final double boxSize;

  const _NavBarIcon({
    super.key,
    required this.isActive,
    required this.svgAsset,
    required this.onTap,
    this.boxSize = 44,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: SizedBox(
        width: boxSize,
        height: boxSize,
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

String _signalLabel(
  SignalType signal, {
  bool abbreviate = false,
  double trendScore = 0,
}) {
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

/// Measured slots for one overview grid tile. Built by
/// [_OverviewGridItem._resolveTileLayout], consumed by the overlay builders.
class _OverviewTileLayout {
  final BoxConstraints constraints;
  final double fontSize;
  final double pad;
  final double starExtent;
  final double starIconSize;
  final bool starInBottomRow;
  final double nameLeft;
  final double nameRight;
  final bool reservedMetaStrip;
  final double metaTopBound;
  final bool lockMetaBelowName;
  final double metaBand;
  final double metaLeft;
  final double metaWidth;
  final bool showMetaRow;
  final bool showBottomMeta;
  final double pctFontSize;
  final bool showRsInBand;
  final double badgeColumnMaxHeight;
  final bool showReliabilityPill;
  final ScorecardSummaryItem? reliabilityRow;

  const _OverviewTileLayout({
    required this.constraints,
    required this.fontSize,
    required this.pad,
    required this.starExtent,
    required this.starIconSize,
    required this.starInBottomRow,
    required this.nameLeft,
    required this.nameRight,
    required this.reservedMetaStrip,
    required this.metaTopBound,
    required this.lockMetaBelowName,
    required this.metaBand,
    required this.metaLeft,
    required this.metaWidth,
    required this.showMetaRow,
    required this.showBottomMeta,
    required this.pctFontSize,
    required this.showRsInBand,
    required this.badgeColumnMaxHeight,
    required this.showReliabilityPill,
    required this.reliabilityRow,
  });
}

/// Badge column height/width and the meta strip it reserves.
class _BadgeColumnSlots {
  final bool showReliabilityPill;
  final ScorecardSummaryItem? reliabilityRow;
  final double badgeColumnMaxHeight;
  final double badgeReserve;
  final bool reservedMetaStrip;
  final double metaTopBound;

  const _BadgeColumnSlots({
    required this.showReliabilityPill,
    required this.reliabilityRow,
    required this.badgeColumnMaxHeight,
    required this.badgeReserve,
    required this.reservedMetaStrip,
    required this.metaTopBound,
  });
}

/// Star placement and the horizontal name band beside/above it.
class _StarNameSlots {
  final double starExtent;
  final double starIconSize;
  final bool starInBottomRow;
  final double nameLeft;
  final double nameRight;
  final bool nameAboveStar;
  final bool showBottomMeta;

  const _StarNameSlots({
    required this.starExtent,
    required this.starIconSize,
    required this.starInBottomRow,
    required this.nameLeft,
    required this.nameRight,
    required this.nameAboveStar,
    required this.showBottomMeta,
  });
}

/// Price/RS band under the name and badge column.
class _MetaRowSlots {
  final bool lockMetaBelowName;
  final double metaBand;
  final double metaLeft;
  final double metaWidth;
  final bool showMetaRow;
  final bool showBottomMeta;
  final double pctFontSize;
  final bool showRsInBand;

  const _MetaRowSlots({
    required this.lockMetaBelowName,
    required this.metaBand,
    required this.metaLeft,
    required this.metaWidth,
    required this.showMetaRow,
    required this.showBottomMeta,
    required this.pctFontSize,
    required this.showRsInBand,
  });
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
  final ScorecardSummaryItem? reliability;
  final bool rsAvailable;
  final bool showRsChip;
  final bool showAlignmentBadge;
  final VoidCallback? onToggleWatchlist;

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
    this.reliability,
    this.rsAvailable = false,
    this.showRsChip = false,
    this.showAlignmentBadge = false,
    this.onToggleWatchlist,
  });

  @override
  Widget build(BuildContext context) {
    final borderRadius = columns == 3 ? 6.0 : 12.0;
    final card = Card(
      // Zero margin so the aligned-badge border (drawn on the wrapping
      // Container, see _wrapWithAlignmentBadge) hugs the card's actual
      // edge instead of leaving Card's default margin as a visible gap.
      // Grid spacing is controlled by the GridView's own
      // crossAxisSpacing/mainAxisSpacing, not by this margin.
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(borderRadius),
      ),
      child: AspectRatio(
        aspectRatio: 2.5,
        child: LayoutBuilder(
          builder: (context, constraints) {
            final layout = _resolveTileLayout(
              constraints,
              MediaQuery.textScalerOf(context),
            );
            return _buildTileStack(layout);
          },
        ),
      ),
    );

    return _wrapWithAlignmentBadge(card, borderRadius);
  }

  /// Measures badge/star/name/meta slots for the current tile size.
  _OverviewTileLayout _resolveTileLayout(
    BoxConstraints constraints,
    TextScaler textScaler,
  ) {
    final fontSize = (constraints.maxWidth * 0.08).clamp(9.0, 18.0);
    final pad = (constraints.maxWidth * 0.03).clamp(4.0, 12.0);
    final nameBottom = _nameBottomBound(
      constraints: constraints,
      pad: pad,
      scaledFont: textScaler.scale(fontSize),
    );
    final badge = _resolveBadgeColumnSlots(
      constraints: constraints,
      fontSize: fontSize,
      pad: pad,
      nameBottom: nameBottom,
      textScaler: textScaler,
    );
    final starName = _resolveStarNameSlots(
      constraints: constraints,
      pad: pad,
      nameBottom: nameBottom,
      badgeReserve: badge.badgeReserve,
    );
    final meta = _resolveMetaRowSlots(
      constraints: constraints,
      pad: pad,
      fontSize: fontSize,
      starExtent: starName.starExtent,
      starInBottomRow: starName.starInBottomRow,
      nameAboveStar: starName.nameAboveStar,
      showBottomMeta: starName.showBottomMeta,
      metaTopBound: badge.metaTopBound,
    );
    return _OverviewTileLayout(
      constraints: constraints,
      fontSize: fontSize,
      pad: pad,
      starExtent: starName.starExtent,
      starIconSize: starName.starIconSize,
      starInBottomRow: starName.starInBottomRow,
      nameLeft: starName.nameLeft,
      nameRight: starName.nameRight,
      reservedMetaStrip: badge.reservedMetaStrip,
      metaTopBound: badge.metaTopBound,
      lockMetaBelowName: meta.lockMetaBelowName,
      metaBand: meta.metaBand,
      metaLeft: meta.metaLeft,
      metaWidth: meta.metaWidth,
      showMetaRow: meta.showMetaRow,
      showBottomMeta: meta.showBottomMeta,
      pctFontSize: meta.pctFontSize,
      showRsInBand: meta.showRsInBand,
      badgeColumnMaxHeight: badge.badgeColumnMaxHeight,
      showReliabilityPill: badge.showReliabilityPill,
      reliabilityRow: badge.reliabilityRow,
    );
  }

  double _nameBottomBound({
    required BoxConstraints constraints,
    required double pad,
    required double scaledFont,
  }) {
    final nameBand = scaledFont * 1.5;
    // Keep the name band inside the card so a bottom Positioned never
    // gets top below bottom at large text scales.
    return (pad + nameBand).clamp(
      pad,
      pad > constraints.maxHeight - pad ? pad : constraints.maxHeight - pad,
    );
  }

  /// Reliability pill under the badge: scale into a 14px meta strip when
  /// the full column would starve price/RS, otherwise drop the pill.
  _BadgeColumnSlots _resolveBadgeColumnSlots({
    required BoxConstraints constraints,
    required double fontSize,
    required double pad,
    required double nameBottom,
    required TextScaler textScaler,
  }) {
    final reliabilityRow = reliability;
    var showReliabilityPill =
        item.badgeComponent.isNotEmpty &&
        reliabilityRow != null &&
        reliabilityTone(reliabilityRow) != null;
    final unscaledBadgeColumnHeight = _badgeColumnHeight(
      item,
      fontSize: fontSize,
      columns: columns,
      textScaler: textScaler,
      includePill: showReliabilityPill,
    );
    var badgeColumnHeight = unscaledBadgeColumnHeight;
    var badgeColumnMaxHeight = badgeColumnHeight;
    var badgeColumnBottom = pad + badgeColumnHeight;
    var metaGap = 0.0;
    var reservedMetaStrip = false;
    final hangsPastName = showReliabilityPill && badgeColumnBottom > nameBottom;
    // Gap is applied whenever the column hangs past the name band, so
    // include it when deciding whether the meta strip stays ≥14px.
    const columnMetaGap = 2.0;
    final bandUnderColumn =
        (constraints.maxHeight -
                badgeColumnBottom -
                pad -
                (hangsPastName ? columnMetaGap : 0.0))
            .clamp(0.0, double.infinity);
    if (showReliabilityPill && bandUnderColumn < 14) {
      // Leave a full 14px meta strip; scale the badge column above it.
      // No gap on this path — the strip is reserved exactly.
      final maxCol = (constraints.maxHeight - 2 * pad - 14).clamp(
        0.0,
        double.infinity,
      );
      if (maxCol >= 10) {
        badgeColumnMaxHeight = maxCol;
        badgeColumnHeight = maxCol;
        badgeColumnBottom = pad + maxCol;
        reservedMetaStrip = true;
      } else {
        // Not enough room to keep both — drop the pill and put meta
        // back under the name band.
        showReliabilityPill = false;
        badgeColumnHeight = _badgeColumnHeight(
          item,
          fontSize: fontSize,
          columns: columns,
          textScaler: textScaler,
          includePill: false,
        );
        badgeColumnMaxHeight = badgeColumnHeight;
        badgeColumnBottom = pad + badgeColumnHeight;
      }
    } else if (hangsPastName) {
      metaGap = columnMetaGap;
    }
    // After scaling, keep meta at the reserved strip even when the name
    // band sits lower — the name ellipsizes above that line.
    final metaTopBound = reservedMetaStrip
        ? badgeColumnBottom
        : (badgeColumnBottom > nameBottom ? badgeColumnBottom : nameBottom) +
              metaGap;
    // FittedBox scales the column uniformly; match the name inset to
    // the painted width so a tall scale-2 pill cannot zero the slot.
    var badgeReserve = _badgeReserveWidth(
      item,
      fontSize: fontSize,
      columns: columns,
      textScaler: textScaler,
      reliability: showReliabilityPill ? reliabilityRow : null,
    );
    if (reservedMetaStrip &&
        unscaledBadgeColumnHeight > 0 &&
        badgeColumnMaxHeight < unscaledBadgeColumnHeight) {
      badgeReserve *= badgeColumnMaxHeight / unscaledBadgeColumnHeight;
    }
    return _BadgeColumnSlots(
      showReliabilityPill: showReliabilityPill,
      reliabilityRow: showReliabilityPill ? reliabilityRow : null,
      badgeColumnMaxHeight: badgeColumnMaxHeight,
      badgeReserve: badgeReserve,
      reservedMetaStrip: reservedMetaStrip,
      metaTopBound: metaTopBound,
    );
  }

  /// Places the star beside or below the name, and insets the name past
  /// the badge reserve.
  _StarNameSlots _resolveStarNameSlots({
    required BoxConstraints constraints,
    required double pad,
    required double nameBottom,
    required double badgeReserve,
  }) {
    // 48px when the card has room. Floor at 32px when the card can hold
    // it; never larger than the card. On a short tile the name sits
    // beside the star when both the star and the badge leave room;
    // otherwise the name stays on the top row and the star sits below.
    const minStarExtent = 32.0;
    const minNameWidth = 24.0;
    const minTapExtent = 32.0;
    var starExtent = 48.0;
    final maxStarExtent = constraints.maxHeight - pad;
    if (maxStarExtent < minStarExtent) {
      starExtent = maxStarExtent < 8 ? 8.0 : maxStarExtent;
    } else {
      if (starExtent > maxStarExtent) starExtent = maxStarExtent;
      if (starExtent < minStarExtent) starExtent = minStarExtent;
    }
    final sidePad = pad + 4;
    final starTop = constraints.maxHeight - pad - starExtent;
    final wantBesideStar = starTop < nameBottom;
    var starIndent = 0.0;
    // Name on the top row; bottom row is clipped to the leftover band
    // under the name.
    var nameAboveStar = false;
    // Star stays in the bottom row unless that band is too short for a
    // usable tap target.
    var starInBottomRow = true;
    var showBottomMeta = true;

    void adoptBelowNameBand() {
      nameAboveStar = true;
      final maxBelow = constraints.maxHeight - nameBottom - pad;
      if (maxBelow >= minTapExtent) {
        starExtent = maxBelow < starExtent ? maxBelow : starExtent;
        starInBottomRow = true;
        showBottomMeta = true;
      } else {
        // Band is only a few pixels — move the control to the top-left
        // so it stays tappable and clear of the name.
        starInBottomRow = false;
        starExtent = minTapExtent;
        if (starExtent > constraints.maxHeight - 2 * pad) {
          starExtent = constraints.maxHeight - 2 * pad;
        }
        if (starExtent < 8) starExtent = 8;
        starIndent = starExtent;
        showBottomMeta = maxBelow >= 14;
      }
    }

    if (wantBesideStar) {
      final besideWidth =
          constraints.maxWidth - 2 * sidePad - starExtent - badgeReserve;
      if (besideWidth >= minNameWidth) {
        // Star on the top-left beside the name — not in the bottom row,
        // so percent/RS cannot inherit its height.
        starIndent = starExtent;
        starInBottomRow = false;
      } else {
        adoptBelowNameBand();
      }
    }
    var nameLeft = sidePad + starIndent;
    // Keep the measured badge inset so the name never sits under the
    // badge. If the beside-star band is still too narrow, drop the star
    // indent and use the below-name layout.
    var nameRight = sidePad + badgeReserve;
    if (constraints.maxWidth - nameLeft - nameRight < minNameWidth &&
        starIndent > 0 &&
        !nameAboveStar) {
      starIndent = 0;
      nameLeft = sidePad;
      adoptBelowNameBand();
      nameLeft = sidePad + starIndent;
    }
    if (constraints.maxWidth - nameLeft - nameRight < 8) {
      if (starInBottomRow) {
        starIndent = 0;
        nameLeft = sidePad;
      }
    }
    var starIconSize = starExtent * 0.5;
    if (starIconSize < 14) starIconSize = 14;
    if (starIconSize > 22) starIconSize = 22;
    if (starIconSize > starExtent) starIconSize = starExtent;
    return _StarNameSlots(
      starExtent: starExtent,
      starIconSize: starIconSize,
      starInBottomRow: starInBottomRow,
      nameLeft: nameLeft,
      nameRight: nameRight,
      nameAboveStar: nameAboveStar,
      showBottomMeta: showBottomMeta,
    );
  }

  /// Price/RS strip under the name and badge column.
  _MetaRowSlots _resolveMetaRowSlots({
    required BoxConstraints constraints,
    required double pad,
    required double fontSize,
    required double starExtent,
    required bool starInBottomRow,
    required bool nameAboveStar,
    required bool showBottomMeta,
    required double metaTopBound,
  }) {
    // Leftover band under the name / badge column — used whenever the
    // meta row is height-bounded (beside-star or below-name).
    final lockMetaBelowName = !starInBottomRow || nameAboveStar;
    final metaBand = (constraints.maxHeight - metaTopBound - pad).clamp(
      0.0,
      double.infinity,
    );
    var bottomMeta = showBottomMeta;
    if (lockMetaBelowName) {
      bottomMeta = metaBand >= 14;
    }
    final pctFontSize = lockMetaBelowName && bottomMeta
        ? (metaBand * 0.4).clamp(6.0, (fontSize * 0.55).clamp(7.0, 11.0))
        : (fontSize * 0.55).clamp(7.0, 11.0);
    final showRsInBand =
        showRsChip && bottomMeta && (!lockMetaBelowName || metaBand >= 14);
    // When the star is pinned top-left, keep percent/RS to its right so
    // they never cover the button's lower half.
    final metaLeft = !starInBottomRow ? pad + starExtent : pad + 4;
    final metaWidth = (constraints.maxWidth - metaLeft - (pad + 4)).clamp(
      0.0,
      double.infinity,
    );
    final showMetaRow =
        (starInBottomRow || bottomMeta) &&
        (!lockMetaBelowName || metaBand > 0) &&
        metaWidth > 0;
    return _MetaRowSlots(
      lockMetaBelowName: lockMetaBelowName,
      metaBand: metaBand,
      metaLeft: metaLeft,
      metaWidth: metaWidth,
      showMetaRow: showMetaRow,
      showBottomMeta: bottomMeta,
      pctFontSize: pctFontSize,
      showRsInBand: showRsInBand,
    );
  }

  Widget _buildTileStack(_OverviewTileLayout layout) {
    return Stack(
      clipBehavior: Clip.hardEdge,
      children: [
        Padding(
          padding: EdgeInsets.all(layout.pad),
          child: _buildSparkline(
            hiRes ? item.sparkline : _downsample(item.sparkline),
          ),
        ),
        _buildNameOverlay(layout),
        if (layout.showMetaRow) _buildMetaOverlay(layout),
        if (item.badgeComponent.isNotEmpty) _buildBadgeOverlay(layout),
        // Paint the top-left star after the meta row so hit tests prefer
        // the button over any residual overlap.
        if (!layout.starInBottomRow)
          Positioned(
            left: layout.pad,
            top: layout.pad,
            child: _buildStarButton(layout),
          ),
      ],
    );
  }

  Widget _buildNameOverlay(_OverviewTileLayout layout) {
    return Positioned(
      left: layout.nameLeft,
      top: layout.pad,
      right: layout.nameRight,
      // When a 14px meta strip is reserved under a scaled badge column,
      // keep the name inside the space above that strip.
      bottom: layout.reservedMetaStrip
          ? layout.constraints.maxHeight - layout.metaTopBound
          : null,
      child: ClipRect(
        child: Text(
          key: Key('overview-name-${item.symbol}'),
          item.symbol.replaceAll('USDT', ''),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            fontSize: layout.fontSize,
            fontWeight: FontWeight.w600,
            color: Colors.white.withAlpha(
              ((columns == 1
                          ? 0.9
                          : columns == 2
                          ? 0.8
                          : 0.7) *
                      255)
                  .round(),
            ),
            backgroundColor: Colors.black.withAlpha((0.25 * 255).round()),
          ),
        ),
      ),
    );
  }

  Widget _buildMetaOverlay(_OverviewTileLayout layout) {
    // Bottom row: price (and favourite star) left, RS chip right.
    // Height-bounded under the name / badge column whenever the star is
    // beside or below the name; inset past a top-left star so taps still
    // hit the button.
    return Positioned(
      left: layout.metaLeft,
      right: layout.pad + 4,
      top: layout.lockMetaBelowName ? layout.metaTopBound : null,
      bottom: layout.pad,
      child: Builder(
        builder: (_) {
          final pct = _sparklinePriceChange(item.sparkline);
          final rounded = pct.toStringAsFixed(1);
          // Treat ±0.0 as zero — grey, no sign.
          final isZero = rounded == '0.0' || rounded == '-0.0';
          final label = isZero ? '0.0%' : '${pct >= 0 ? '+' : ''}$rounded%';
          final color = isZero
              ? Colors.grey
              : (pct >= 0 ? Colors.green : Colors.red);
          final narrow = columns == 3 || layout.lockMetaBelowName;
          final row = Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              if (layout.starInBottomRow) _buildStarButton(layout),
              if (layout.showBottomMeta) ...[
                Expanded(
                  child: Text(
                    key: Key('overview-pct-${item.symbol}'),
                    label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: layout.pctFontSize,
                      color: color,
                      fontWeight: FontWeight.w600,
                      shadows: const [
                        Shadow(color: Colors.black, blurRadius: 3),
                        Shadow(color: Colors.black, blurRadius: 3),
                      ],
                    ),
                  ),
                ),
                if (layout.showRsInBand)
                  Flexible(
                    child: RelativeStrengthChip(
                      rsAvailable: rsAvailable,
                      rs: item.rs,
                      beta: item.beta,
                      dense: true,
                      compactLabel: narrow,
                    ),
                  ),
              ],
            ],
          );
          if (!layout.lockMetaBelowName) return row;
          // Scale or clip percent/RS into the leftover band.
          return ClipRect(
            child: Align(
              alignment: Alignment.bottomLeft,
              child: FittedBox(
                fit: BoxFit.scaleDown,
                alignment: Alignment.bottomLeft,
                child: SizedBox(
                  height: layout.metaBand > 0 ? layout.metaBand : null,
                  width: layout.metaWidth,
                  child: row,
                ),
              ),
            ),
          );
        },
      ),
    );
  }

  Widget _buildBadgeOverlay(_OverviewTileLayout layout) {
    // Badge column after the meta row so the reliability pill keeps its
    // taps when heights are tight.
    return Positioned(
      right: layout.pad + 4,
      top: layout.pad,
      height: layout.badgeColumnMaxHeight,
      child: FittedBox(
        fit: BoxFit.scaleDown,
        alignment: Alignment.topRight,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            KeyedSubtree(
              key: Key('overview-badge-${item.symbol}'),
              child: _buildBadge(item, layout.fontSize),
            ),
            if (layout.showReliabilityPill && layout.reliabilityRow != null)
              ReliabilityChip(item: layout.reliabilityRow!, dense: true),
          ],
        ),
      ),
    );
  }

  Widget _buildStarButton(_OverviewTileLayout layout) {
    return IconButton(
      tooltip: isFavourite ? 'Remove from watchlist' : 'Add to watchlist',
      onPressed: onToggleWatchlist,
      style: IconButton.styleFrom(
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
        padding: EdgeInsets.zero,
        minimumSize: Size(layout.starExtent, layout.starExtent),
        maximumSize: Size(layout.starExtent, layout.starExtent),
        fixedSize: Size(layout.starExtent, layout.starExtent),
      ),
      constraints: BoxConstraints.tightFor(
        width: layout.starExtent,
        height: layout.starExtent,
      ),
      icon: Icon(
        isFavourite ? Icons.star : Icons.star_border,
        color: isFavourite
            ? Colors.amber.withAlpha((0.8 * 255).round())
            : Colors.white.withAlpha((0.5 * 255).round()),
        size: layout.starIconSize,
      ),
    );
  }

  /// Wraps [card] with a top-edge colored border + tooltip when this item's
  /// MTF stack is strongly aligned (>= 0.75, PR-100). Otherwise returns
  /// [card] unchanged — no extra widget nesting for the common case.
  Widget _wrapWithAlignmentBadge(Widget card, double borderRadius) {
    final alignment = item.alignment;
    final alignedState = item.alignedState;
    if (!showAlignmentBadge ||
        alignment == null ||
        alignedState == null ||
        alignment < 0.75) {
      return card;
    }
    final color = regimeColor(alignedState);
    return Tooltip(
      message: alignedTooltip(alignedState, alignment),
      child: Container(
        key: Key('aligned-badge-${item.symbol}'),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(borderRadius),
          border: Border(top: BorderSide(color: color, width: 3)),
        ),
        child: card,
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
    final trendDirection = _badgeTrendDirection(item);
    final badgeFontSize = _badgeFontSize(fontSize, columns);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
      decoration: BoxDecoration(
        color: _signalColor(
          signal,
          trendScore: trendDirection,
        ).withAlpha((0.8 * 255).round()),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        _signalLabel(
          signal,
          abbreviate: columns > 1,
          trendScore: trendDirection,
        ),
        style: TextStyle(
          fontSize: badgeFontSize,
          fontWeight: FontWeight.bold,
          color: Colors.white,
        ),
      ),
    );
  }

  /// Vertical space the top-right badge column needs (badge ± reliability).
  double _badgeColumnHeight(
    OverviewItem item, {
    required double fontSize,
    required int columns,
    required TextScaler textScaler,
    required bool includePill,
  }) {
    if (item.badgeComponent.isEmpty) return 0;
    final signal = _parseSignalType(item.badgeComponent);
    final label = _signalLabel(
      signal,
      abbreviate: columns > 1,
      trendScore: _badgeTrendDirection(item),
    );
    final badgePainter = TextPainter(
      text: TextSpan(
        text: label,
        style: TextStyle(
          fontSize: _badgeFontSize(fontSize, columns),
          fontWeight: FontWeight.bold,
        ),
      ),
      textDirection: TextDirection.ltr,
      textScaler: textScaler,
      maxLines: 1,
    )..layout();
    // Container vertical padding is 2 on each side.
    var height = badgePainter.height + 4;
    if (includePill) {
      final chipPainter = TextPainter(
        text: const TextSpan(
          text: '88%',
          style: TextStyle(fontSize: 8, fontWeight: FontWeight.w600),
        ),
        textDirection: TextDirection.ltr,
        textScaler: textScaler,
        maxLines: 1,
      )..layout();
      // Dense ReliabilityChip outer vertical pad is 1px on each side.
      height += chipPainter.height + 2;
    }
    return height;
  }

  /// Horizontal space the top-right badge column needs, including padding.
  /// Uses the wider of the signal badge and the reliability pill.
  double _badgeReserveWidth(
    OverviewItem item, {
    required double fontSize,
    required int columns,
    required TextScaler textScaler,
    ScorecardSummaryItem? reliability,
  }) {
    if (item.badgeComponent.isEmpty) return 0;
    final signal = _parseSignalType(item.badgeComponent);
    final label = _signalLabel(
      signal,
      abbreviate: columns > 1,
      trendScore: _badgeTrendDirection(item),
    );
    final badgePainter = TextPainter(
      text: TextSpan(
        text: label,
        style: TextStyle(
          fontSize: _badgeFontSize(fontSize, columns),
          fontWeight: FontWeight.bold,
        ),
      ),
      textDirection: TextDirection.ltr,
      textScaler: textScaler,
      maxLines: 1,
    )..layout();
    // Container horizontal padding is 4 on each side. A small gap keeps
    // the name clear of the badge once both are laid out.
    var reserve = badgePainter.width + 8 + 2;
    if (reliability != null && reliabilityTone(reliability) != null) {
      final chipPainter = TextPainter(
        text: TextSpan(
          text: reliabilityChipLabel(reliability, dense: true),
          style: const TextStyle(fontSize: 8, fontWeight: FontWeight.w600),
        ),
        textDirection: TextDirection.ltr,
        textScaler: textScaler,
        maxLines: 1,
      )..layout();
      // Dense chip horizontal padding is 3 on each side. Extra gap covers
      // layout rounding so the name stays clear of the painted pill.
      final chipReserve = chipPainter.width + 6 + 4;
      if (chipReserve > reserve) reserve = chipReserve;
    }
    return reserve;
  }

  double _badgeTrendDirection(OverviewItem item) {
    final trendFalling =
        badgeScorecardLabel('trend', item.sparkline) == 'trend_down';
    return trendFalling ? -1.0 : 1.0;
  }

  double _badgeFontSize(double fontSize, int columns) {
    final scale = columns == 1
        ? 1.0
        : columns == 2
        ? 0.9
        : 0.8;
    return (fontSize * 0.7 * scale).clamp(7.0, 12.0);
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
