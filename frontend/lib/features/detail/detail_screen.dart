import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/app_lifecycle_manager.dart';
import '../../core/auto_refresh_timer.dart';
import '../../core/format_price.dart';
import '../../core/polling_config.dart';
import '../candles/api/candle_response.dart';
import '../candles/application/get_candle_series.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../../infrastructure/preferences_service.dart';
import '../events/event_filter.dart';
import '../events/events_view_model.dart';
import '../events/macro_events_screen.dart';
import '../social/social_feed_screen.dart';
import '../social/social_feed_view_model.dart';
import 'chart/chart_config.dart';
import 'chart/indicator_panel.dart';
import 'chart/interactive_chart.dart';
import 'chart_navigation.dart';
import 'detail_context.dart';
import 'http_setup_api.dart';
import 'http_fragility_api.dart';
import 'http_behavior_api.dart';
import 'fragility_data.dart';
import 'behavior_data.dart';
import 'setup_data.dart';
import '../scorecards/http_scorecard_api.dart';
import '../scorecards/reliability_chip.dart';
import '../scorecards/scorecard_catalog.dart';
import '../scorecards/scorecard_data.dart';
import 'trade/exchange_config.dart';
import 'trade/trade_action_buttons.dart';
import '../volatility/volatility_alignment.dart';
import '../volatility/volatility_model.dart';
import '../volatility/http_volatility_api.dart';

/// DetailScreen displays a single symbol in detail with candle chart,
/// header block, time context, score breakdown, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;
  final DetailContext? detailContext;
  final bool isFavourite;
  final EventsViewModel? eventsViewModel;
  final SocialFeedViewModel? socialFeedViewModel;

  /// API for fetching setup quality scores.
  final SetupApi? setupApi;

  /// API for fetching fragility / position crowding scores.
  final FragilityApi? fragilityApi;

  /// API for fetching retail behavior scores.
  final BehaviorApi? behaviorApi;

  /// Service used to fetch candles when the user switches timeframe.
  final GetCandleSeries? getCandleSeries;

  /// Leading candles used only for indicator warmup (not scrollable).
  final int warmupCount;

  /// Number of candles to fill the viewport width initially.
  final int initialVisibleCount;

  /// Whether the user has pro access (enables auto-refresh).
  final bool isProUser;

  /// API for fetching intraday volatility profiles.
  final VolatilityApi? volatilityApi;

  /// Reliability summary for the setup chip. Null hides the chip.
  final ScorecardApi? scorecardApi;

  const DetailScreen({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.series,
    this.detailContext,
    this.isFavourite = false,
    this.eventsViewModel,
    this.socialFeedViewModel,
    this.setupApi,
    this.fragilityApi,
    this.behaviorApi,
    this.getCandleSeries,
    this.warmupCount = 0,
    this.initialVisibleCount = 30,
    this.isProUser = false,
    this.volatilityApi,
    this.scorecardApi,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
  ChartIndicatorConfig _chartConfig = const ChartIndicatorConfig();

  /// Config with pro-only features disabled when not pro.
  /// Preserves the saved config so settings are restored on upgrade.
  ChartIndicatorConfig get _effectiveConfig {
    if (widget.isProUser) return _chartConfig;
    return _chartConfig.copyWith(
      showBehaviorPanel: false,
      showVolatility: false,
    );
  }

  late bool isFavourite;
  String _preferredExchangeId = 'binance';
  List<ExchangeConfig> _exchanges = kDefaultExchanges;
  CustomExchange? _customExchange;

  // ---- mutable series / timeframe state ----
  late String _timeframe;
  late CandleSeriesResponse _series;
  late int _warmupCount;
  bool _isLoadingTf = false;

  // ---- setup quality state ----
  SetupData? _setupData;
  bool _isLoadingSetup = false;
  bool _setupFetched = false;
  int _setupGeneration = 0;
  final ScorecardCatalog _scorecards = ScorecardCatalog();

  // ---- fragility state ----
  FragilityData? _fragilityData;
  bool _isLoadingFragility = false;
  bool _fragilityFetched = false;
  int _fragilityGeneration = 0;

  // ---- behavior state ----
  BehaviorData? _behaviorData;
  bool _isLoadingBehavior = false;
  bool _behaviorFetched = false;
  int _behaviorGeneration = 0;

  // ---- volatility state ----
  List<VolatilityBucket>? _volatilityData;
  String? _volatilityTimeframe;
  bool _volatilityFetched = false;
  int _volatilityGeneration = 0;

  // ---- auto-refresh (pro only) ----
  AutoRefreshTimer? _autoRefreshTimer;

  // ---- macro events 15m refresh timer ----
  AutoRefreshTimer? _eventsRefreshTimer;

  // ---- lifecycle registration ----
  Pausable? _pausable;
  AppLifecycleManager? _lifecycle;

  @override
  void initState() {
    super.initState();
    isFavourite = widget.isFavourite;
    _timeframe = widget.timeframe.value;
    _series = widget.series;
    _warmupCount = widget.warmupCount;
    _loadChartConfig();
    _loadExchangePreference();
    _loadExchangeConfigs();
    _loadEvents();
    _wireSocialFeedCallback();
    _loadSetupData();
    _loadScorecards();
    _loadFragilityData();
    _loadBehaviorData();
    _loadVolatilityData();
    _startAutoRefresh();
    _startEventsRefreshTimer();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_pausable == null) {
      final mgr = AppLifecycleScope.of(context);
      _lifecycle = mgr;
      if (mgr != null) {
        _pausable = Pausable(
          onPause: () {
            _autoRefreshTimer?.stop();
            _eventsRefreshTimer?.stop();
          },
          onResume: () {
            _autoRefreshTimer?.start();
            _eventsRefreshTimer?.start();
          },
        );
        mgr.addPausable(_pausable!);
      }
    }
  }

  @override
  void dispose() {
    if (_pausable != null) _lifecycle?.removePausable(_pausable!);
    _autoRefreshTimer?.dispose();
    _eventsRefreshTimer?.dispose();
    widget.socialFeedViewModel?.onChanged = null;
    super.dispose();
  }

  /// Fetch candles for [tf] and swap the active series.
  Future<void> _switchTimeframe(String tf) async {
    final svc = widget.getCandleSeries;
    if (svc == null || tf == _timeframe) return;
    setState(() => _isLoadingTf = true);
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol.value,
        timeframe: tf,
      );
      final result = await svc.execute(input);
      if (!mounted) return;
      setState(() {
        _timeframe = tf;
        _series = result;
        _warmupCount = kIndicatorWarmup;
        _isLoadingTf = false;
        _setupData = null;
        _setupFetched = false;
        _fragilityData = null;
        _fragilityFetched = false;
        _behaviorData = null;
        _behaviorFetched = false;
        _volatilityData = null;
        _volatilityTimeframe = null;
        _volatilityFetched = false;
      });
      _loadEvents(); // reload events for new date range
      _loadSetupData(); // reload setup for new timeframe
      _loadScorecards();
      _loadFragilityData(); // reload fragility for new timeframe
      _loadBehaviorData(); // reload behavior for new timeframe
      _loadVolatilityData(); // reload volatility for new timeframe
      _startAutoRefresh(); // restart with new timeframe interval
    } catch (_) {
      if (mounted) setState(() => _isLoadingTf = false);
    }
  }

  // ---- auto-refresh (pro only) ----

  /// Starts (or restarts) the chart auto-refresh timer for the current
  /// timeframe.  No-op when the user is not on the pro tier.
  void _startAutoRefresh() {
    _autoRefreshTimer?.dispose();
    _autoRefreshTimer = null;
    if (!widget.isProUser) return;
    final interval = kChartRefreshIntervals[_timeframe];
    if (interval == null) return;
    _autoRefreshTimer = AutoRefreshTimer(
      interval: interval,
      onTick: _autoRefreshChart,
    );
    _autoRefreshTimer!.start();
  }

  /// Re-fetches candles + all dependent panels silently.
  /// A result for a timeframe that is no longer on screen is dropped,
  /// including its setup reload.
  Future<void> _autoRefreshChart() async {
    final svc = widget.getCandleSeries;
    if (svc == null || !mounted) return;
    final timeframe = _timeframe;
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol.value,
        timeframe: timeframe,
      );
      final result = await svc.execute(input);
      if (!mounted || _timeframe != timeframe) return;
      setState(() {
        _series = result;
        _warmupCount = kIndicatorWarmup;
      });
      // Events are refreshed by their own 15-minute timer — not here.
      _setupFetched = false;
      _loadSetupData();
      _fragilityFetched = false;
      _loadFragilityData();
      _behaviorFetched = false;
      _loadBehaviorData();
    } catch (_) {
      // Silently ignore — next tick will retry.
    }
  }

  // ---- macro events 15m refresh ----

  /// Starts the 15-minute one-shot timer that re-fetches macro events
  /// when the user stays on the detail chart for an extended period.
  void _startEventsRefreshTimer() {
    _eventsRefreshTimer?.dispose();
    _eventsRefreshTimer = AutoRefreshTimer(
      interval: kMacroEventsRefreshDuration,
      onTick: () async => _loadEvents(),
    );
    _eventsRefreshTimer!.start();
  }

  Future<void> _loadChartConfig() async {
    final prefs = await SharedPreferences.getInstance();
    setState(() {
      _chartConfig = ChartIndicatorConfig.load(prefs);
    });
  }

  Future<void> _saveChartConfig(ChartIndicatorConfig cfg) async {
    setState(() => _chartConfig = cfg);
    final prefs = await SharedPreferences.getInstance();
    cfg.save(prefs);
  }

  Future<void> _loadExchangeConfigs() async {
    final configs = await loadExchangeConfigs();
    if (mounted) setState(() => _exchanges = configs);
  }

  Future<void> _loadExchangePreference() async {
    final prefs = await SharedPreferences.getInstance();
    final svc = PreferencesService(prefs);
    setState(() {
      _preferredExchangeId = svc.preferredExchange;
      final name = svc.customExchangeName;
      final url = svc.customExchangeUrl;
      if (name != null && url != null && name.isNotEmpty && url.isNotEmpty) {
        _customExchange = CustomExchange(name: name, urlTemplate: url);
      }
    });
  }

  Future<void> _savePreferredExchange(String id) async {
    setState(() => _preferredExchangeId = id);
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('settings.preferredExchange', id);
  }

  Future<void> _saveCustomExchange(CustomExchange? custom) async {
    setState(() => _customExchange = custom);
    final prefs = await SharedPreferences.getInstance();
    final svc = PreferencesService(prefs);
    svc.customExchangeName = custom?.name;
    svc.customExchangeUrl = custom?.urlTemplate;
  }

  Future<void> _showCustomExchangeForm() async {
    final result = await showCustomExchangeEditor(
      context,
      existing: _customExchange,
    );
    if (result != null) {
      _saveCustomExchange(result);
    }
  }

  void _wireSocialFeedCallback() {
    final svm = widget.socialFeedViewModel;
    if (svm == null) return;
    svm.onChanged = () {
      if (mounted) setState(() {});
    };
  }

  void _loadEvents() {
    final evm = widget.eventsViewModel;
    if (evm == null) return;
    evm.onChanged = () {
      if (mounted) setState(() {});
    };
    // Determine date range from the candle series
    final candles = _series.candles;
    if (candles.isEmpty) return;
    final dateFrom = _isoDate(candles.first.timestamp);
    // Extend dateTo to cover the forward projection window so that
    // future scheduled events are included in the feed.
    final projectionEnd = candles.last.timestamp.add(
      maxProjectionWindow(_timeframe),
    );
    final dateTo = _isoDate(projectionEnd);
    evm.load(dateFrom, dateTo);
  }

  String _isoDate(DateTime dt) {
    final y = dt.year.toString().padLeft(4, '0');
    final m = dt.month.toString().padLeft(2, '0');
    final d = dt.day.toString().padLeft(2, '0');
    return '$y-$m-$d';
  }

  Future<void> _loadSetupData() async {
    final api = widget.setupApi;
    if (api == null || _setupFetched) return;
    final generation = ++_setupGeneration;
    final timeframe = _timeframe;
    setState(() => _isLoadingSetup = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: timeframe,
      );
      if (!mounted ||
          generation != _setupGeneration ||
          _timeframe != timeframe) {
        return;
      }
      if (data.timeframe != timeframe) {
        setState(() {
          _setupData = null;
          _isLoadingSetup = false;
          _setupFetched = true;
        });
        return;
      }
      setState(() {
        _setupData = data;
        _isLoadingSetup = false;
        _setupFetched = true;
      });
    } catch (_) {
      if (!mounted ||
          generation != _setupGeneration ||
          _timeframe != timeframe) {
        return;
      }
      setState(() {
        _isLoadingSetup = false;
        _setupFetched = true;
        if (_setupData != null && _setupData!.timeframe != _timeframe) {
          _setupData = null;
        }
      });
    }
  }

  Future<void> _loadScorecards() {
    if (widget.setupApi == null) return Future<void>.value();
    return _scorecards.load(
      api: widget.scorecardApi,
      timeframe: _timeframe,
      notify: () {
        if (mounted) setState(() {});
      },
    );
  }

  Future<void> _loadFragilityData() async {
    final api = widget.fragilityApi;
    if (api == null || _fragilityFetched) return;
    final generation = ++_fragilityGeneration;
    final timeframe = _timeframe;
    setState(() => _isLoadingFragility = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: timeframe,
      );
      if (!mounted ||
          generation != _fragilityGeneration ||
          _timeframe != timeframe) {
        return;
      }
      if (data.timeframe != timeframe) {
        setState(() {
          _isLoadingFragility = false;
          if (_fragilityData != null &&
              _fragilityData!.timeframe != _timeframe) {
            _fragilityData = null;
          }
        });
        return;
      }
      setState(() {
        _fragilityData = data;
        _isLoadingFragility = false;
        _fragilityFetched = true;
      });
    } catch (_) {
      if (!mounted ||
          generation != _fragilityGeneration ||
          _timeframe != timeframe) {
        return;
      }
      setState(() {
        _isLoadingFragility = false;
        if (_fragilityData != null && _fragilityData!.timeframe != _timeframe) {
          _fragilityData = null;
        } else {
          _fragilityFetched = true;
        }
      });
    }
  }

  Future<void> _loadBehaviorData() async {
    final api = widget.behaviorApi;
    if (api == null || _behaviorFetched) return;
    final generation = ++_behaviorGeneration;
    final timeframe = _timeframe;
    setState(() => _isLoadingBehavior = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: timeframe,
      );
      if (!mounted ||
          generation != _behaviorGeneration ||
          _timeframe != timeframe) {
        return;
      }
      if (data.timeframe != timeframe) {
        setState(() {
          _isLoadingBehavior = false;
          if (_behaviorData != null && _behaviorData!.timeframe != _timeframe) {
            _behaviorData = null;
          }
        });
        return;
      }
      setState(() {
        _behaviorData = data;
        _isLoadingBehavior = false;
        _behaviorFetched = true;
      });
    } catch (_) {
      if (!mounted ||
          generation != _behaviorGeneration ||
          _timeframe != timeframe) {
        return;
      }
      setState(() {
        _isLoadingBehavior = false;
        if (_behaviorData != null && _behaviorData!.timeframe != _timeframe) {
          _behaviorData = null;
        } else {
          _behaviorFetched = true;
        }
      });
    }
  }

  Future<void> _loadVolatilityData() async {
    final api = widget.volatilityApi;
    if (api == null || _volatilityFetched) return;
    final generation = ++_volatilityGeneration;
    final timeframe = _timeframe;
    try {
      final data = await api.fetch(timeframe: timeframe);
      if (!mounted ||
          generation != _volatilityGeneration ||
          _timeframe != timeframe) {
        return;
      }
      setState(() {
        _volatilityData = data;
        _volatilityTimeframe = timeframe;
        _volatilityFetched = true;
      });
    } catch (_) {
      if (!mounted ||
          generation != _volatilityGeneration ||
          _timeframe != timeframe) {
        return;
      }
      setState(() {
        if (_volatilityData != null && _volatilityTimeframe != _timeframe) {
          _volatilityData = null;
          _volatilityTimeframe = null;
        } else {
          _volatilityFetched = true;
        }
      });
    }
  }

  /// Reload the current chart data (same timeframe, fresh candles).
  Future<void> _reloadChart() async {
    final svc = widget.getCandleSeries;
    if (svc == null) return;
    final timeframe = _timeframe;
    setState(() => _isLoadingTf = true);
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol.value,
        timeframe: timeframe,
      );
      final result = await svc.execute(input);
      if (!mounted) return;
      if (_timeframe != timeframe) {
        setState(() => _isLoadingTf = false);
        return;
      }
      setState(() {
        _series = result;
        _warmupCount = kIndicatorWarmup;
        _isLoadingTf = false;
        _setupFetched = false;
        _fragilityFetched = false;
        _behaviorFetched = false;
        _volatilityFetched = false;
      });
      _loadEvents();
      _loadSetupData();
      _loadScorecards();
      _loadFragilityData();
      _loadBehaviorData();
      _loadVolatilityData();
    } catch (_) {
      if (mounted) setState(() => _isLoadingTf = false);
    }
  }

  // ---- percentage helpers ----

  /// Compute the percentage change from the reference area (last
  /// [widget.initialVisibleCount] candles).  This matches the overview
  /// sparkline the user tapped on.
  double? _referenceAreaPct() {
    final candles = _series.candles;
    final n = widget.initialVisibleCount;
    if (candles.length < 2) return null;
    final startIdx = (candles.length - n).clamp(0, candles.length - 1);
    final first = candles[startIdx].close;
    final last = candles.last.close;
    if (first == 0) return null;
    return ((last - first) / first) * 100;
  }

  /// Compute the percentage change over the last 24 hours.
  double? _last24hPct() {
    final candles = _series.candles;
    if (candles.length < 2) return null;
    final now = candles.last.timestamp;
    final cutoff = now.subtract(const Duration(hours: 24));
    // Find the first candle at or after the cutoff.
    int idx = 0;
    for (var i = 0; i < candles.length; i++) {
      if (!candles[i].timestamp.isBefore(cutoff)) {
        idx = i;
        break;
      }
    }
    // If all candles are within 24h, use the first candle.
    final first = candles[idx].close;
    final last = candles.last.close;
    if (first == 0) return null;
    return ((last - first) / first) * 100;
  }

  /// Index of the start of the reference area (for the green line).
  int get _referenceStartIndex {
    final total = _series.candles.length;
    return (total - widget.initialVisibleCount).clamp(0, total - 1);
  }

  void _showInfoDialog({required String title, required String body}) {
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: Text(body),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

  String _formatVolume(double vol) {
    if (vol >= 1e9) return '\$${(vol / 1e9).toStringAsFixed(1)}B';
    if (vol >= 1e6) return '\$${(vol / 1e6).toStringAsFixed(1)}M';
    if (vol >= 1e3) return '\$${(vol / 1e3).toStringAsFixed(1)}K';
    return '\$${vol.toStringAsFixed(0)}';
  }

  String _timeRangeLabel() {
    final count = _series.candles.length;
    final tf = _timeframe;
    final hours = _tfToHours(tf);
    final totalHours = count * hours;
    String approx;
    if (totalHours >= 24) {
      final days = (totalHours / 24).round();
      approx = '~$days day${days == 1 ? '' : 's'}';
    } else {
      approx = '~${totalHours.round()} hr${totalHours.round() == 1 ? '' : 's'}';
    }
    return 'Showing last $count \u00d7 $tf candles ($approx)';
  }

  double _tfToHours(String tf) {
    switch (tf) {
      case '1m':
        return 1 / 60;
      case '5m':
        return 5 / 60;
      case '15m':
        return 15 / 60;
      case '1h':
        return 1;
      case '4h':
        return 4;
      case '1d':
        return 24;
      default:
        return 1;
    }
  }

  void _navigateToEventsList(String scrollToEventId) {
    final evm = widget.eventsViewModel;
    if (evm == null) return;
    Navigator.of(context)
        .push(
          MaterialPageRoute(
            builder: (_) => MacroEventsScreen(
              viewModel: evm,
              scrollToEventId: scrollToEventId.isNotEmpty
                  ? scrollToEventId
                  : null,
              isProUser: widget.isProUser,
            ),
          ),
        )
        .then((_) {
          // Re-attach the onChanged listener (MacroEventsScreen overrides it)
          // and reload events for the chart's date range so the chart overlay
          // reflects any updates (e.g. newly visible future events).
          _loadEvents();
        });
  }

  void _navigateToSocialFeed() {
    final svm = widget.socialFeedViewModel;
    if (svm == null) return;
    Navigator.of(context)
        .push(
          MaterialPageRoute(builder: (_) => SocialFeedScreen(viewModel: svm)),
        )
        .then((_) {
          _wireSocialFeedCallback();
        });
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final series = _series;
    final candles = series.candles;
    final ctx = widget.detailContext;

    final pct24h = _last24hPct();
    final pctRef = _referenceAreaPct();

    return WillPopScope(
      onWillPop: () async {
        Navigator.of(context).pop(isFavourite);
        return false;
      },
      child: Scaffold(
        backgroundColor: const Color.fromARGB(255, 0, 0, 0),
        extendBodyBehindAppBar: true,
        appBar: AppBar(
          backgroundColor: Colors.black,
          elevation: 0,
          leading: IconButton(
            icon: Icon(
              isFavourite ? Icons.star : Icons.star_border,
              color: isFavourite ? Colors.amber : Colors.white54,
            ),
            onPressed: () => setState(() => isFavourite = !isFavourite),
            tooltip: isFavourite ? 'Unfavourite' : 'Favourite',
          ),
          title: Row(
            children: [
              Text(
                widget.symbol.value,
                style: const TextStyle(
                  color: Colors.white,
                  fontWeight: FontWeight.bold,
                  fontSize: 20,
                ),
              ),
              const SizedBox(width: 12),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  color: Colors.white.withAlpha(30),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: widget.getCandleSeries != null
                    ? DropdownButtonHideUnderline(
                        child: DropdownButton<String>(
                          value: _timeframe,
                          isDense: true,
                          dropdownColor: const Color(0xFF1A1A2E),
                          style: const TextStyle(
                            color: Colors.white70,
                            fontSize: 14,
                          ),
                          icon: const Icon(
                            Icons.arrow_drop_down,
                            color: Colors.white54,
                            size: 18,
                          ),
                          items: kTimeframes
                              .map(
                                (tf) => DropdownMenuItem(
                                  value: tf,
                                  child: Text(tf),
                                ),
                              )
                              .toList(),
                          onChanged: _isLoadingTf
                              ? null
                              : (tf) {
                                  if (tf != null) _switchTimeframe(tf);
                                },
                        ),
                      )
                    : Text(
                        _timeframe,
                        style: const TextStyle(
                          color: Colors.white70,
                          fontSize: 14,
                        ),
                      ),
              ),
              const Spacer(),
              // Indicator settings
              IconButton(
                icon: const Icon(Icons.tune, color: Colors.white70, size: 20),
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 40, minHeight: 32),
                onPressed: () async {
                  final result = await showIndicatorPanel(
                    context,
                    _chartConfig,
                    isProUser: widget.isProUser,
                  );
                  if (result != null) _saveChartConfig(result);
                },
                tooltip: 'Indicators',
              ),
            ],
          ),
          actions: [
            if (widget.getCandleSeries != null)
              IconButton(
                icon: const Icon(Icons.refresh, color: Colors.white70),
                onPressed: _isLoadingTf ? null : _reloadChart,
                tooltip: 'Reload chart',
              ),
            IconButton(
              icon: const Icon(Icons.close, color: Colors.white),
              onPressed: () => Navigator.of(context).maybePop(isFavourite),
              tooltip: 'Close',
            ),
          ],
        ),
        body: SingleChildScrollView(
          physics: const ClampingScrollPhysics(),
          padding: EdgeInsets.only(
            left: 16,
            right: 16,
            top: 8,
            bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (ctx != null) _buildHeaderBlock(ctx, pct24h, pctRef),
              if (ctx != null) const SizedBox(height: 12),
              Text(
                _timeRangeLabel(),
                style: const TextStyle(color: Colors.white38, fontSize: 12),
              ),
              const SizedBox(height: 8),
              if (_isLoadingTf)
                const SizedBox(
                  height: 360,
                  child: Center(child: CircularProgressIndicator()),
                )
              else
                InteractiveChart(
                  series: series,
                  config: _effectiveConfig,
                  onConfigChanged: _saveChartConfig,
                  eventsViewModel: widget.eventsViewModel,
                  onNavigateToEvent: _navigateToEventsList,
                  socialFeedViewModel: widget.socialFeedViewModel,
                  onNavigateToFeed: _navigateToSocialFeed,
                  volatilityAligned: _volatilityData != null
                      ? alignBucketsToCandles(
                          candles: _series.candles,
                          bucketsByMinute: buildBucketLookup(_volatilityData!),
                          isDailyTimeframe: _timeframe == '1d',
                        )
                      : null,
                  warmupCount: _warmupCount,
                  initialVisibleCount: widget.initialVisibleCount,
                  referenceStartIndex: _referenceStartIndex,
                ),
              // Overlay controls (social feed + macro events)
              if (widget.socialFeedViewModel != null ||
                  widget.eventsViewModel != null) ...[
                const SizedBox(height: 8),
                _buildOverlayControls(),
              ],
              const SizedBox(height: 12),
              TradeActionButtons(
                symbol: widget.symbol.value,
                timeframe: _timeframe,
                preferredExchangeId: _preferredExchangeId,
                exchanges: _exchanges,
                customExchange: _customExchange,
                onExchangeChanged: _savePreferredExchange,
                onAddCustom: _showCustomExchangeForm,
                onEditCustom: _showCustomExchangeForm,
              ),
              if (pct24h != null || pctRef != null) ...[
                const SizedBox(height: 12),
                Row(
                  children: [
                    // 24h percentage
                    if (pct24h != null) ...[
                      Text(
                        '${pct24h > 0 ? '+' : ''}${pct24h.toStringAsFixed(2)}%',
                        style: TextStyle(
                          color: pct24h > 0
                              ? Colors.green
                              : (pct24h < 0 ? Colors.red : Colors.grey),
                          fontWeight: FontWeight.bold,
                          fontSize: 16,
                        ),
                      ),
                      const SizedBox(width: 2),
                      GestureDetector(
                        onTap: () => _showInfoDialog(
                          title: '24h Change',
                          body: 'Percentage change over the last 24 hours.',
                        ),
                        child: const Icon(
                          Icons.help_outline,
                          size: 13,
                          color: Colors.white30,
                        ),
                      ),
                    ],
                    // Divider
                    if (pct24h != null && pctRef != null)
                      const Padding(
                        padding: EdgeInsets.symmetric(horizontal: 8),
                        child: Text(
                          '|',
                          style: TextStyle(color: Colors.white24, fontSize: 16),
                        ),
                      ),
                    // Reference area percentage
                    if (pctRef != null) ...[
                      Text(
                        '${pctRef > 0 ? '+' : ''}${pctRef.toStringAsFixed(2)}%',
                        style: TextStyle(
                          color: pctRef > 0
                              ? Colors.green
                              : (pctRef < 0 ? Colors.red : Colors.grey),
                          fontWeight: FontWeight.bold,
                          fontSize: 14,
                        ),
                      ),
                      const SizedBox(width: 2),
                      GestureDetector(
                        onTap: () => _showInfoDialog(
                          title: 'Reference Area',
                          body:
                              'Percentage change across the reference area '
                              '(green line on time axis) — same window as the '
                              'overview sparkline you tapped on.',
                        ),
                        child: const Icon(
                          Icons.help_outline,
                          size: 13,
                          color: Colors.white30,
                        ),
                      ),
                    ],
                    const Spacer(),
                    Text(
                      formatPrice(candles.last.close),
                      style: const TextStyle(
                        color: Colors.white70,
                        fontSize: 16,
                      ),
                    ),
                  ],
                ),
              ],
              if (ctx != null) ...[
                const SizedBox(height: 20),
                _scoringWindowInfo(),
                const SizedBox(height: 6),
                _buildScoreBreakdown(ctx),
                const SizedBox(height: 20),
                _buildPriceAction(ctx),
              ],
              if (_setupData != null) ...[
                const SizedBox(height: 20),
                _buildSetupQuality(_setupData!),
              ] else if (_isLoadingSetup) ...[
                const SizedBox(height: 20),
                const Center(
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ],
              if (_fragilityData != null) ...[
                const SizedBox(height: 20),
                _buildFragility(_fragilityData!),
              ] else if (_isLoadingFragility) ...[
                const SizedBox(height: 20),
                const Center(
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ],
              if (_behaviorData != null) ...[
                const SizedBox(height: 20),
                _buildBehavior(_behaviorData!),
              ] else if (_isLoadingBehavior) ...[
                const SizedBox(height: 20),
                const Center(
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildOverlayControls() {
    final svm = widget.socialFeedViewModel;
    final evm = widget.eventsViewModel;
    final socialOn = svm?.showOnChart ?? false;
    final eventsOn = evm?.state.showEvents ?? false;

    return Row(
      children: [
        // Social feed toggle
        if (svm != null) ...[
          _overlayToggle(
            icon: Icons.rss_feed,
            label: '',
            active: socialOn,
            onTap: () => setState(() {
              svm.showOnChart = !svm.showOnChart;
            }),
          ),
          const SizedBox(width: 12),
        ],
        // Macro events toggle
        if (evm != null) ...[
          _overlayToggle(
            icon: Icons.public,
            label: 'Events',
            active: eventsOn,
            onTap: () => setState(() {
              evm.toggleShowEvents();
            }),
          ),
          // Filter level chips — only when events are shown
          if (eventsOn) ...[
            const SizedBox(width: 16),
            for (final level in EventFilterLevel.values) ...[
              GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => setState(() {
                  evm.setFilterLevel(level);
                }),
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 3,
                  ),
                  decoration: BoxDecoration(
                    color: evm.state.filterLevel == level
                        ? Colors.white.withAlpha(25)
                        : Colors.transparent,
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(
                      color: evm.state.filterLevel == level
                          ? Colors.white38
                          : Colors.white12,
                      width: 0.5,
                    ),
                  ),
                  child: Text(
                    level.label,
                    style: TextStyle(
                      color: evm.state.filterLevel == level
                          ? Colors.white70
                          : Colors.white30,
                      fontSize: 10,
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 6),
            ],
          ],
        ],
        const Spacer(),
        // "View all" link to events list
        if (evm != null)
          GestureDetector(
            onTap: () => _navigateToEventsList(''),
            child: const Text(
              'View all',
              style: TextStyle(color: Colors.white38, fontSize: 10),
            ),
          ),
      ],
    );
  }

  Widget _overlayToggle({
    required IconData icon,
    required String label,
    required bool active,
    required VoidCallback onTap,
  }) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 6),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              icon,
              size: 16,
              color: active ? Colors.white70 : Colors.white30,
            ),
            const SizedBox(width: 4),
            Text(
              label,
              style: TextStyle(
                color: active ? Colors.white70 : Colors.white30,
                fontSize: 11,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildHeaderBlock(DetailContext ctx, double? pct24h, double? pctRef) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Rank #${ctx.rank} \u2014 Sideways v2',
          style: const TextStyle(color: Colors.white54, fontSize: 13),
        ),
        const SizedBox(height: 4),
        Row(
          children: [
            if (pct24h != null)
              Text(
                '24h: ${pct24h > 0 ? '+' : ''}${pct24h.toStringAsFixed(1)}%',
                style: TextStyle(
                  color: pct24h > 0
                      ? Colors.green
                      : (pct24h < 0 ? Colors.red : Colors.grey),
                  fontSize: 13,
                ),
              ),
            if (pct24h != null && pctRef != null)
              const Text(
                ' | ',
                style: TextStyle(color: Colors.white24, fontSize: 13),
              ),
            if (pctRef != null)
              Text(
                'Ref: ${pctRef > 0 ? '+' : ''}${pctRef.toStringAsFixed(1)}%',
                style: TextStyle(
                  color: pctRef > 0
                      ? Colors.green
                      : (pctRef < 0 ? Colors.red : Colors.grey),
                  fontSize: 12,
                ),
              ),
            const SizedBox(width: 16),
            Text(
              'Vol: ${_formatVolume(ctx.volume)}',
              style: const TextStyle(color: Colors.white54, fontSize: 13),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildFragility(FragilityData data) {
    Color riskColor;
    switch (data.riskLevel) {
      case 'high':
        riskColor = Colors.redAccent;
        break;
      case 'medium':
        riskColor = Colors.orangeAccent;
        break;
      default:
        riskColor = Colors.greenAccent;
    }
    final comps = {
      'fundingExtremeness': data.components.fundingExtremeness,
      'oiExpansion': data.components.oiExpansion,
      'longShortImbalance': data.components.longShortImbalance,
      'liquidationProximity': data.components.liquidationProximity,
    };
    // Normalize sub-components to fragilityScore (same approach as
    // Metrics Breakdown → totalScore and Setup Quality → score).
    final total = data.fragilityScore.clamp(0.0, 1.0);
    final compSum = comps.values.fold(0.0, (a, b) => a + b);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Text(
              'Fragility',
              style: TextStyle(
                color: Colors.white70,
                fontWeight: FontWeight.w600,
                fontSize: 14,
              ),
            ),
            const SizedBox(width: 4),
            GestureDetector(
              onTap: () => _showInfoDialog(
                title: 'Fragility',
                body:
                    'How vulnerable the current price level is to '
                    'sudden dislocations.\n\n'
                    '• Funding Extremeness — distance of funding rate from neutral\n'
                    '• OI Expansion — open-interest growth vs baseline\n'
                    '• Long/Short Imbalance — skew in positioning\n'
                    '• Liquidation Proximity — how close price is to '
                    'liquidation clusters\n\n'
                    'High fragility suggests a stop-hunt or squeeze is more likely.',
              ),
              child: const Icon(
                Icons.help_outline,
                size: 13,
                color: Colors.white30,
              ),
            ),
            const Spacer(),
            Text(
              FragilityData.riskLabel(data.riskLevel),
              style: TextStyle(color: riskColor, fontSize: 12),
            ),
            const SizedBox(width: 6),
            Text(
              '${(total * 100).toStringAsFixed(0)}%',
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.bold,
                fontSize: 13,
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),
        if (data.dominantSide != 'neutral') ...[
          Row(
            children: [
              Text(
                FragilityData.sideLabel(data.dominantSide),
                style: TextStyle(
                  color: riskColor,
                  fontWeight: FontWeight.w600,
                  fontSize: 12,
                ),
              ),
              const SizedBox(width: 8),
              Text(
                FragilityData.squeezeLabel(data.squeezeRisk),
                style: const TextStyle(color: Colors.white54, fontSize: 12),
              ),
            ],
          ),
          const SizedBox(height: 8),
        ],
        for (final entry in comps.entries)
          _metricBar(
            '${FragilityComponents.displayName(entry.key)}:',
            compSum > 0 ? (entry.value / compSum) * total : 0.0,
            '${(compSum > 0 ? (entry.value / compSum) * total * 100 : 0.0).toStringAsFixed(0)}%',
            riskColor,
          ),
      ],
    );
  }

  Widget _buildBehavior(BehaviorData data) {
    Color colorFor(String key, double value) {
      switch (key) {
        case 'greed':
          return Colors.greenAccent;
        case 'fear':
          return Colors.orangeAccent;
        case 'patience':
          return Colors.blueAccent;
        case 'panic':
          return Colors.redAccent;
        default:
          return Colors.white54;
      }
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Text(
              'Retail Behavior',
              style: TextStyle(
                color: Colors.white70,
                fontWeight: FontWeight.w600,
                fontSize: 14,
              ),
            ),
            const SizedBox(width: 4),
            GestureDetector(
              onTap: () => _showInfoDialog(
                title: 'Retail Behavior',
                body:
                    'Inferred crowd-sentiment dimensions derived from '
                    'funding rates, open-interest dynamics, and '
                    'order-flow imbalances.\n\n'
                    '• Greed — aggressive long positioning\n'
                    '• Fear — defensive / hedging bias\n'
                    '• Patience — low activity, wait-and-see\n'
                    '• Panic — capitulation signals',
              ),
              child: const Icon(
                Icons.help_outline,
                size: 13,
                color: Colors.white30,
              ),
            ),
            const Spacer(),
            Text(
              data.summary,
              style: const TextStyle(color: Colors.white54, fontSize: 12),
            ),
          ],
        ),
        const SizedBox(height: 8),
        for (final entry in data.dimensions.entries)
          _metricBar(
            '${BehaviorData.dimensionLabel(entry.key)}:',
            entry.value,
            '${(entry.value * 100).toStringAsFixed(0)}%',
            colorFor(entry.key, entry.value),
          ),
      ],
    );
  }

  /// Tiny info line above the score fieldsets explaining the scoring window.
  Widget _scoringWindowInfo() {
    final dur = candleDuration(_timeframe) * kSparklineCandles;
    final label = _humanDuration(dur);
    return Text(
      'Scores computed over the last $kSparklineCandles candles ($_timeframe ≈ $label) — green line on chart.',
      style: const TextStyle(
        color: Colors.white38,
        fontSize: 10,
        fontStyle: FontStyle.italic,
      ),
    );
  }

  static String _humanDuration(Duration d) {
    if (d.inDays > 0) {
      final days = d.inDays;
      final hours = d.inHours % 24;
      if (hours == 0) return '$days d';
      return '$days d ${hours}h';
    }
    if (d.inHours > 0) {
      final hours = d.inHours;
      final mins = d.inMinutes % 60;
      if (mins == 0) return '${hours}h';
      return '${hours}h ${mins}m';
    }
    return '${d.inMinutes}m';
  }

  /// Confidence dot color for breakout bars, null when setup data unavailable.
  Color? get _breakoutDotColor {
    final data = _setupData;
    if (data == null) return null;
    if (data.confidence > 0.75) return Colors.green;
    if (data.confidence > 0.55) return Colors.amber;
    return Colors.red;
  }

  Widget _buildScoreBreakdown(DetailContext ctx) {
    // Normalize directional metrics to totalScore (not 100%).
    // A weak total score (e.g. 0.30) compresses all bars, conveying that
    // the overall signal confidence is low — same approach as Setup Quality.
    final rawTrend = ctx.trendScore.abs();
    final rawSideways = ctx.sidewaysScore.abs();
    final rawCompression = ctx.compressionScore.abs();
    final rawBreakoutUp = ctx.breakoutUpScore.abs();
    final rawBreakoutDown = ctx.breakoutDownScore.abs();
    final metricSum =
        rawTrend +
        rawSideways +
        rawCompression +
        rawBreakoutUp +
        rawBreakoutDown;
    final total = ctx.totalScore.clamp(0.0, 1.0);
    final trendPct = metricSum > 0 ? (rawTrend / metricSum) * total : 0.0;
    final sidewaysPct = metricSum > 0 ? (rawSideways / metricSum) * total : 0.0;
    final compressionPct = metricSum > 0
        ? (rawCompression / metricSum) * total
        : 0.0;
    final breakoutUpPct = metricSum > 0
        ? (rawBreakoutUp / metricSum) * total
        : 0.0;
    final breakoutDownPct = metricSum > 0
        ? (rawBreakoutDown / metricSum) * total
        : 0.0;

    // Direction coloring: trend uses sign, compression uses sign heuristic,
    // breakout up = green, breakout down = red, sideways = gray.
    final trendColor = ctx.trendScore >= 0 ? Colors.green : Colors.red;
    final compressionColor = Colors.amber;
    const sidewaysColor = Colors.grey;

    return _fieldset(
      'Metrics Breakdown',
      [
        _metricBar(
          'Trend:',
          trendPct,
          '${(trendPct * 100).toStringAsFixed(0)}%',
          trendColor,
        ),
        _metricBar(
          'Sideways:',
          sidewaysPct,
          '${(sidewaysPct * 100).toStringAsFixed(0)}%',
          sidewaysColor,
        ),
        _metricBar(
          'Compression:',
          compressionPct,
          '${(compressionPct * 100).toStringAsFixed(0)}%',
          compressionColor,
        ),
        _metricBar(
          'Breakout Up:',
          breakoutUpPct,
          '${(breakoutUpPct * 100).toStringAsFixed(0)}%',
          Colors.green,
          dotColor: _breakoutDotColor,
        ),
        _metricBar(
          'Breakout Down:',
          breakoutDownPct,
          '${(breakoutDownPct * 100).toStringAsFixed(0)}%',
          Colors.red,
          dotColor: _breakoutDotColor,
        ),
      ],
      hint:
          'Proportional weight of each regime detector '
          'normalised to overall conviction (total score).\n\n'
          '• Trend — directional strength (slope × R²)\n'
          '• Sideways — range-bound, low-volatility character\n'
          '• Compression — narrowing Bollinger bandwidth\n'
          '• Breakout Up / Down — price escaping a compression zone',
    );
  }

  Widget _buildPriceAction(DetailContext ctx) {
    final gain = ctx.gainScore;
    final pct = (gain * 100).abs();
    final color = gain >= 0 ? Colors.green : Colors.red;
    final label = gain >= 0 ? 'Gainer' : 'Loser';
    return _fieldset(
      'Price Action',
      [
        _metricBar(
          '$label:',
          gain.abs().clamp(0.0, 1.0),
          '${pct.toStringAsFixed(0)}%',
          color,
        ),
      ],
      hint:
          'Net return detected over the scoring window.\n\n'
          'Gainer — positive price change.\n'
          'Loser — negative price change.\n\n'
          'Bar width shows magnitude relative to 100%.',
    );
  }

  Widget _buildSetupQuality(SetupData data) {
    final totalPct = data.score; // 0..1
    final totalDisplay = '${(totalPct * 100).toStringAsFixed(0)}%';
    final colors = <String, Color>{
      'compression_breakout': Colors.purple,
      'trend_continuation': Colors.blue,
      'range_reversion': Colors.teal,
    };

    // Normalize sub-scores to the total quality percentage
    final subSum = data.scores.values.fold(0.0, (a, b) => a + b);

    // Health label color: green > 0.8, grey > 0.6, orange > 0.4, red otherwise
    Color healthColor;
    if (data.trendHealth > 0.8) {
      healthColor = Colors.green;
    } else if (data.trendHealth > 0.6) {
      healthColor = Colors.white54;
    } else if (data.trendHealth > 0.4) {
      healthColor = Colors.orange;
    } else {
      healthColor = Colors.red;
    }

    // Confidence dot color: green > 0.75, yellow > 0.55, red otherwise
    Color confidenceColor;
    if (data.confidence > 0.75) {
      confidenceColor = Colors.green;
    } else if (data.confidence > 0.55) {
      confidenceColor = Colors.amber;
    } else {
      confidenceColor = Colors.red;
    }

    final title = data.regime.isNotEmpty
        ? 'Setup Quality \u2014 $totalDisplay \u00b7 ${data.confidenceDot}'
        : 'Setup Quality \u2014 $totalDisplay';

    final mapped = setupScorecardLabel(
      bestSetup: data.bestSetup,
      regime: data.regime,
      breakoutUp: data.breakoutUp,
      breakoutDown: data.breakoutDown,
    );
    final logged = data.confidence >= setupSignalConfidence;

    return _fieldset(
      title,
      [
        for (final entry in data.scores.entries)
          _metricBar(
            '${SetupData.displayName(entry.key)}:',
            subSum > 0 ? (entry.value / subSum) * totalPct : 0.0,
            '${(subSum > 0 ? (entry.value / subSum) * totalPct * 100 : 0.0).toStringAsFixed(0)}%',
            colors[entry.key] ?? Colors.grey,
          ),
      ],
      hint:
          'Tradability assessment — how well the current '
          'price structure matches known setup archetypes.\n\n'
          '• Compression Breakout — tight range about to break\n'
          '• Trend Continuation — pullback within a strong trend\n'
          '• Range Reversion — mean-reversion at range edges\n\n'
          'Confidence: ${data.confidenceLabel}',
      titleSuffixColor: data.regime.isNotEmpty ? confidenceColor : null,
      trailing: logged
          ? setupSignalTrailing(
              label: mapped,
              chip: ReliabilityChip(
                item: setupReliabilityItem(
                  confidence: data.confidence,
                  label: mapped,
                  items: _scorecards.items,
                ),
              ),
            )
          : null,
    );
  }

  // ---- shared fieldset & metric bar helpers ----

  Widget _fieldset(
    String title,
    List<Widget> children, {
    String? hint,
    Color? titleSuffixColor,
    Widget? trailing,
  }) {
    // When titleSuffixColor is provided, colour the text after the last " · "
    // in the title using a RichText widget, while keeping everything before
    // it in the default style.
    Widget titleWidget;
    final sepIdx = title.lastIndexOf(' · ');
    if (titleSuffixColor != null && sepIdx >= 0) {
      final prefix = title.substring(0, sepIdx + 3);
      final suffix = title.substring(sepIdx + 3);
      titleWidget = RichText(
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        text: TextSpan(
          style: const TextStyle(
            color: Colors.white70,
            fontWeight: FontWeight.w600,
            fontSize: 14,
          ),
          children: [
            TextSpan(text: prefix),
            TextSpan(
              text: suffix,
              style: TextStyle(color: titleSuffixColor),
            ),
          ],
        ),
      );
    } else {
      titleWidget = Text(
        title,
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          color: Colors.white70,
          fontWeight: FontWeight.w600,
          fontSize: 14,
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FieldsetHeader(
          title: titleWidget,
          hint: hint == null
              ? null
              : GestureDetector(
                  onTap: () => _showInfoDialog(title: title, body: hint),
                  child: const Icon(
                    Icons.help_outline,
                    size: 13,
                    color: Colors.white30,
                  ),
                ),
          trailing: trailing,
        ),
        const SizedBox(height: 8),
        ...children,
      ],
    );
  }

  Widget _metricBar(
    String label,
    double fraction,
    String display,
    Color color, {
    Color? dotColor,
  }) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
          const SizedBox(height: 3),
          Row(
            children: [
              Expanded(
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(3),
                  child: LinearProgressIndicator(
                    value: fraction.clamp(0.0, 1.0),
                    backgroundColor: Colors.white12,
                    valueColor: AlwaysStoppedAnimation<Color>(color),
                    minHeight: 10,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              SizedBox(
                width: 40,
                child: Text(
                  display,
                  style: const TextStyle(color: Colors.white70, fontSize: 12),
                  textAlign: TextAlign.right,
                ),
              ),
              if (dotColor != null) ...[
                const SizedBox(width: 4),
                Text('\u25CF', style: TextStyle(color: dotColor, fontSize: 10)),
              ],
            ],
          ),
        ],
      ),
    );
  }
}

/// Title row for a detail fieldset. Extra width goes to the title. A short
/// title keeps its width. A long title gets the width it needs to wrap within
/// two lines, and the deficit comes out of the trailing label down to the chip.
class FieldsetHeader extends StatelessWidget {
  final Widget title;
  final Widget? hint;
  final Widget? trailing;

  const FieldsetHeader({
    super.key,
    required this.title,
    this.hint,
    this.trailing,
  });

  @override
  Widget build(BuildContext context) {
    return _FieldsetHeaderRow(title: title, hint: hint, trailing: trailing);
  }
}

class _FieldsetHeaderRow extends MultiChildRenderObjectWidget {
  _FieldsetHeaderRow({required Widget title, Widget? hint, Widget? trailing})
    : super(
        children: [
          _HeaderSlot(kind: _HeaderChildKind.title, child: title),
          if (hint != null)
            _HeaderSlot(kind: _HeaderChildKind.hint, child: hint),
          if (trailing != null)
            _HeaderSlot(kind: _HeaderChildKind.trailing, child: trailing),
        ],
      );

  @override
  RenderObject createRenderObject(BuildContext context) {
    return _RenderFieldsetHeader();
  }
}

enum _HeaderChildKind { title, hint, trailing }

class _HeaderSlot extends ParentDataWidget<_HeaderParentData> {
  final _HeaderChildKind kind;

  const _HeaderSlot({required this.kind, required super.child});

  @override
  void applyParentData(RenderObject renderObject) {
    final parentData = renderObject.parentData! as _HeaderParentData;
    if (parentData.kind == kind) return;
    parentData.kind = kind;
    final targetParent = renderObject.parent;
    if (targetParent is RenderObject) targetParent.markNeedsLayout();
  }

  @override
  Type get debugTypicalAncestorWidgetClass => _FieldsetHeaderRow;
}

class _HeaderParentData extends ContainerBoxParentData<RenderBox> {
  _HeaderChildKind kind = _HeaderChildKind.title;
}

class _RenderFieldsetHeader extends RenderBox
    with
        ContainerRenderObjectMixin<RenderBox, _HeaderParentData>,
        RenderBoxContainerDefaultsMixin<RenderBox, _HeaderParentData> {
  static const double _hintGap = 4;
  static const double _trailingGap = 8;

  @override
  void setupParentData(RenderBox child) {
    if (child.parentData is! _HeaderParentData) {
      child.parentData = _HeaderParentData();
    }
  }

  RenderBox? _child(_HeaderChildKind kind) {
    RenderBox? child = firstChild;
    while (child != null) {
      final parentData = child.parentData! as _HeaderParentData;
      if (parentData.kind == kind) return child;
      child = parentData.nextSibling;
    }
    return null;
  }

  @override
  void performLayout() {
    final title = _child(_HeaderChildKind.title)!;
    final hint = _child(_HeaderChildKind.hint);
    final trailing = _child(_HeaderChildKind.trailing);
    final maxWidth = constraints.hasBoundedWidth
        ? constraints.maxWidth
        : double.infinity;
    final maxHeight = constraints.maxHeight;

    final titleOneLine = title.getMaxIntrinsicWidth(maxHeight);
    final titleTwoLine = _widthWithinTwoLines(title, titleOneLine);
    final hintWidth = hint?.getMaxIntrinsicWidth(maxHeight) ?? 0;
    final hintBlock = hint == null ? 0.0 : _hintGap + hintWidth;
    final trailGap = trailing == null ? 0.0 : _trailingGap;
    final trailingNatural = trailing?.getMaxIntrinsicWidth(maxHeight) ?? 0;
    final trailingFloor = trailing == null ? 0.0 : _trailingFloor(trailing);
    final gaps = hintBlock + trailGap;
    final widths = _allocate(
      maxWidth: maxWidth,
      titleOneLine: titleOneLine,
      titleTwoLine: titleTwoLine,
      trailingNatural: trailingNatural,
      trailingFloor: trailingFloor,
      gaps: gaps,
    );

    final titleSize = _layoutAt(title, widths.title, maxHeight);
    final hintSize = hint == null
        ? Size.zero
        : _layoutAt(hint, hintWidth, maxHeight);
    final trailingSize = trailing == null
        ? Size.zero
        : _layoutAt(trailing, widths.trailing, maxHeight);
    final height = math.max(
      titleSize.height,
      math.max(hintSize.height, trailingSize.height),
    );
    final width = maxWidth.isFinite
        ? maxWidth
        : titleSize.width + gaps + trailingSize.width;
    size = constraints.constrain(Size(width, height));

    _place(title, 0, height);
    if (hint != null) {
      final hintAfter = math.min(titleOneLine, widths.title);
      _place(hint, hintAfter + _hintGap, height);
    }
    if (trailing != null) {
      _place(trailing, size.width - trailingSize.width, height);
    }
  }

  /// Smallest width at which [title] wraps onto at most two lines.
  /// A title that does not wrap returns its one-line width.
  double _widthWithinTwoLines(RenderBox title, double oneLine) {
    final narrow = title.getMinIntrinsicWidth(double.infinity);
    if (narrow >= oneLine - 0.5) return oneLine;
    final lineHeight = title.getMinIntrinsicHeight(oneLine);
    if (lineHeight <= 0) return oneLine;
    final twoLines = lineHeight * 2 + 1;
    if (title.getMinIntrinsicHeight(narrow) <= twoLines) return narrow;
    var low = narrow;
    var high = oneLine;
    for (var i = 0; i < 24; i++) {
      final mid = (low + high) / 2;
      if (title.getMinIntrinsicHeight(mid) <= twoLines) {
        high = mid;
      } else {
        low = mid;
      }
    }
    return high;
  }

  /// Width of the trailing pieces that do not scale: the gap and the chip.
  double _trailingFloor(RenderBox trailing) {
    if (trailing is! RenderFlex) {
      return trailing.getMinIntrinsicWidth(double.infinity);
    }
    var floor = 0.0;
    RenderBox? child = trailing.firstChild;
    while (child != null) {
      final parentData = child.parentData! as FlexParentData;
      if (parentData.flex == null) {
        floor += child.getMaxIntrinsicWidth(double.infinity);
      }
      child = parentData.nextSibling;
    }
    return floor;
  }

  _HeaderWidths _allocate({
    required double maxWidth,
    required double titleOneLine,
    required double titleTwoLine,
    required double trailingNatural,
    required double trailingFloor,
    required double gaps,
  }) {
    if (!maxWidth.isFinite ||
        maxWidth >= titleOneLine + gaps + trailingNatural) {
      final trailing = trailingNatural;
      final title = maxWidth.isFinite
          ? math.max(0.0, maxWidth - gaps - trailing)
          : titleOneLine;
      return _HeaderWidths(title, trailing);
    }
    final besideTwoLines = maxWidth - gaps - titleTwoLine;
    if (besideTwoLines >= trailingFloor) {
      final trailing = math.min(trailingNatural, besideTwoLines);
      final title = trailing >= trailingNatural
          ? maxWidth - gaps - trailing
          : titleTwoLine;
      return _HeaderWidths(title, trailing);
    }
    final floor = math.min(trailingFloor, math.max(0.0, maxWidth - gaps));
    return _HeaderWidths(math.max(0.0, maxWidth - gaps - floor), floor);
  }

  Size _layoutAt(RenderBox child, double width, double maxHeight) {
    final box = math.max(0.0, width);
    child.layout(
      BoxConstraints(minWidth: box, maxWidth: box, maxHeight: maxHeight),
      parentUsesSize: true,
    );
    return child.size;
  }

  void _place(RenderBox child, double x, double rowHeight) {
    final parentData = child.parentData! as _HeaderParentData;
    parentData.offset = Offset(x, (rowHeight - child.size.height) / 2);
  }

  @override
  void paint(PaintingContext context, Offset offset) {
    defaultPaint(context, offset);
  }

  @override
  bool hitTestChildren(BoxHitTestResult result, {required Offset position}) {
    return defaultHitTestChildren(result, position: position);
  }
}

class _HeaderWidths {
  final double title;
  final double trailing;

  const _HeaderWidths(this.title, this.trailing);
}

/// Mapped setup label plus its chip. The chip stays at its tap size.
Widget setupSignalTrailing({required String label, required Widget chip}) {
  return Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      Flexible(
        child: FittedBox(
          fit: BoxFit.scaleDown,
          alignment: Alignment.centerRight,
          child: Text(
            label,
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ),
      ),
      const SizedBox(width: 6),
      chip,
    ],
  );
}
