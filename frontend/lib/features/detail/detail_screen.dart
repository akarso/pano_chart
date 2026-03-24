import 'package:flutter/material.dart';
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
import 'trade/exchange_config.dart';
import 'trade/trade_action_buttons.dart';
import '../volatility/volatility_alignment.dart';
import '../volatility/volatility_model.dart';
import '../volatility/volatility_widget.dart';

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

  /// Optional intraday volatility profile (1440 minute-of-day buckets).
  final List<VolatilityBucket>? volatilityData;

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
    this.volatilityData,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
  ChartIndicatorConfig _chartConfig = const ChartIndicatorConfig();
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

  // ---- fragility state ----
  FragilityData? _fragilityData;
  bool _isLoadingFragility = false;
  bool _fragilityFetched = false;

  // ---- behavior state ----
  BehaviorData? _behaviorData;
  bool _isLoadingBehavior = false;
  bool _behaviorFetched = false;

  // ---- auto-refresh (pro only) ----
  AutoRefreshTimer? _autoRefreshTimer;

  // ---- macro events 15m refresh timer ----
  AutoRefreshTimer? _eventsRefreshTimer;

  // ---- lifecycle registration ----
  Pausable? _pausable;

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
    _loadFragilityData();
    _loadBehaviorData();
    _startAutoRefresh();
    _startEventsRefreshTimer();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_pausable == null) {
      final mgr = AppLifecycleScope.of(context);
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
    final mgr = AppLifecycleScope.of(context);
    if (_pausable != null) mgr?.removePausable(_pausable!);
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
      });
      _loadEvents(); // reload events for new date range
      _setupFetched = false;
      _loadSetupData(); // reload setup for new timeframe
      _fragilityFetched = false;
      _loadFragilityData(); // reload fragility for new timeframe
      _behaviorFetched = false;
      _loadBehaviorData(); // reload behavior for new timeframe
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
  Future<void> _autoRefreshChart() async {
    final svc = widget.getCandleSeries;
    if (svc == null || !mounted) return;
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol.value,
        timeframe: _timeframe,
      );
      final result = await svc.execute(input);
      if (!mounted) return;
      setState(() {
        _series = result;
        _warmupCount = kIndicatorWarmup;
      });
      _loadEvents();
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
    final projectionEnd =
        candles.last.timestamp.add(maxProjectionWindow(_timeframe));
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
    setState(() => _isLoadingSetup = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: _timeframe,
      );
      if (!mounted) return;
      setState(() {
        _setupData = data;
        _isLoadingSetup = false;
        _setupFetched = true;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _isLoadingSetup = false;
          _setupFetched = true;
        });
      }
    }
  }

  Future<void> _loadFragilityData() async {
    final api = widget.fragilityApi;
    if (api == null || _fragilityFetched) return;
    setState(() => _isLoadingFragility = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: _timeframe,
      );
      if (!mounted) return;
      setState(() {
        _fragilityData = data;
        _isLoadingFragility = false;
        _fragilityFetched = true;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _isLoadingFragility = false;
          _fragilityFetched = true;
        });
      }
    }
  }

  Future<void> _loadBehaviorData() async {
    final api = widget.behaviorApi;
    if (api == null || _behaviorFetched) return;
    setState(() => _isLoadingBehavior = true);
    try {
      final data = await api.fetch(
        symbol: widget.symbol.value,
        timeframe: _timeframe,
      );
      if (!mounted) return;
      setState(() {
        _behaviorData = data;
        _isLoadingBehavior = false;
        _behaviorFetched = true;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _isLoadingBehavior = false;
          _behaviorFetched = true;
        });
      }
    }
  }

  /// Reload the current chart data (same timeframe, fresh candles).
  Future<void> _reloadChart() async {
    final svc = widget.getCandleSeries;
    if (svc == null) return;
    setState(() => _isLoadingTf = true);
    try {
      final input = buildDetailChartInput(
        symbol: widget.symbol.value,
        timeframe: _timeframe,
      );
      final result = await svc.execute(input);
      if (!mounted) return;
      setState(() {
        _series = result;
        _warmupCount = kIndicatorWarmup;
        _isLoadingTf = false;
      });
      _loadEvents();
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
      approx =
          '~${totalHours.round()} hr${totalHours.round() == 1 ? '' : 's'}';
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
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => MacroEventsScreen(
          viewModel: evm,
          scrollToEventId:
              scrollToEventId.isNotEmpty ? scrollToEventId : null,
        ),
      ),
    ).then((_) {
      // Re-attach the onChanged listener (MacroEventsScreen overrides it)
      // and reload events for the chart's date range so the chart overlay
      // reflects any updates (e.g. newly visible future events).
      _loadEvents();
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
                              .map((tf) => DropdownMenuItem(
                                    value: tf,
                                    child: Text(tf),
                                  ))
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
                constraints: const BoxConstraints(minWidth: 36, minHeight: 32),
                onPressed: () async {
                  final result = await showIndicatorPanel(context, _chartConfig);
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
            left: 16, right: 16, top: 8,
            bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (ctx != null) _buildHeaderBlock(ctx, pct24h, pctRef),
              if (ctx != null) const SizedBox(height: 12),
              Text(
                _timeRangeLabel(),
                style: const TextStyle(
                  color: Colors.white38,
                  fontSize: 12,
                ),
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
                  config: _chartConfig,
                  onConfigChanged: _saveChartConfig,
                  eventsViewModel: widget.eventsViewModel,
                  onNavigateToEvent: _navigateToEventsList,
                  socialFeedViewModel: widget.socialFeedViewModel,
                  warmupCount: _warmupCount,
                  initialVisibleCount: widget.initialVisibleCount,
                  referenceStartIndex: _referenceStartIndex,
                ),
            // Intraday activity profile
            if (_chartConfig.showVolatility &&
                widget.volatilityData != null &&
                widget.volatilityData!.isNotEmpty) ...[
              const SizedBox(height: 4),
              VolatilityWidget(
                bars: alignBucketsToCandles(
                  candles: _series.candles,
                  bucketsByMinute: buildBucketLookup(widget.volatilityData!),
                ),
              ),
            ],
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
                        color: pct24h > 0 ? Colors.green : (pct24h < 0 ? Colors.red : Colors.grey),
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
                      child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
                    ),
                  ],
                  // Divider
                  if (pct24h != null && pctRef != null)
                    const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 8),
                      child: Text('|', style: TextStyle(color: Colors.white24, fontSize: 16)),
                    ),
                  // Reference area percentage
                  if (pctRef != null) ...[
                    Text(
                      '${pctRef > 0 ? '+' : ''}${pctRef.toStringAsFixed(2)}%',
                      style: TextStyle(
                        color: pctRef > 0 ? Colors.green : (pctRef < 0 ? Colors.red : Colors.grey),
                        fontWeight: FontWeight.bold,
                        fontSize: 14,
                      ),
                    ),
                    const SizedBox(width: 2),
                    GestureDetector(
                      onTap: () => _showInfoDialog(
                        title: 'Reference Area',
                        body: 'Percentage change across the reference area '
                            '(green line on time axis) — same window as the '
                            'overview sparkline you tapped on.',
                      ),
                      child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
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
                  width: 20, height: 20,
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
                  width: 20, height: 20,
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
                  width: 20, height: 20,
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
    return Row(
      children: [
        // Social feed toggle
        if (widget.socialFeedViewModel != null) ...[
          GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () {
              widget.socialFeedViewModel!.showOnChart =
                  !widget.socialFeedViewModel!.showOnChart;
            },
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    Icons.rss_feed,
                    size: 16,
                    color: widget.socialFeedViewModel!.showOnChart
                        ? Colors.white70
                        : Colors.white30,
                  ),
                  const SizedBox(width: 4),
                ],
              ),
            ),
          ),
          const SizedBox(width: 12),
        ],
        // Macro events toggle + filter chips
        if (widget.eventsViewModel != null) ...[
          GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () => widget.eventsViewModel!.toggleShowEvents(),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    Icons.public,
                    size: 16,
                    color: widget.eventsViewModel!.state.showEvents
                        ? Colors.white70
                        : Colors.white30,
                  ),
                  const SizedBox(width: 4),
                  Text(
                    'Events',
                    style: TextStyle(
                      color: widget.eventsViewModel!.state.showEvents
                          ? Colors.white70
                          : Colors.white30,
                      fontSize: 11,
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(width: 16),
          // Filter level chips
          for (final level in EventFilterLevel.values) ...[
            GestureDetector(
              onTap: () => widget.eventsViewModel!.setFilterLevel(level),
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: BoxDecoration(
                  color: widget.eventsViewModel!.state.filterLevel == level
                      ? Colors.white.withAlpha(25)
                      : Colors.transparent,
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                    color:
                        widget.eventsViewModel!.state.filterLevel == level
                            ? Colors.white38
                            : Colors.white12,
                    width: 0.5,
                  ),
                ),
                child: Text(
                  level.label,
                  style: TextStyle(
                    color:
                        widget.eventsViewModel!.state.filterLevel == level
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
        const Spacer(),
        // "View all" link to events list
        if (widget.eventsViewModel != null)
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

  Widget _buildHeaderBlock(DetailContext ctx, double? pct24h, double? pctRef) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Rank #${ctx.rank} \u2014 Sideways v2',
          style: const TextStyle(
            color: Colors.white54,
            fontSize: 13,
          ),
        ),
        const SizedBox(height: 4),
        Row(
          children: [
            if (pct24h != null)
              Text(
                '24h: ${pct24h > 0 ? '+' : ''}${pct24h.toStringAsFixed(1)}%',
                style: TextStyle(
                  color: pct24h > 0 ? Colors.green : (pct24h < 0 ? Colors.red : Colors.grey),
                  fontSize: 13,
                ),
              ),
            if (pct24h != null && pctRef != null)
              const Text(' | ', style: TextStyle(color: Colors.white24, fontSize: 13)),
            if (pctRef != null)
              Text(
                'Ref: ${pctRef > 0 ? '+' : ''}${pctRef.toStringAsFixed(1)}%',
                style: TextStyle(
                  color: pctRef > 0 ? Colors.green : (pctRef < 0 ? Colors.red : Colors.grey),
                  fontSize: 12,
                ),
              ),
            const SizedBox(width: 16),
            Text(
              'Vol: ${_formatVolume(ctx.volume)}',
              style: const TextStyle(
                color: Colors.white54,
                fontSize: 13,
              ),
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
                body: 'How vulnerable the current price level is to '
                    'sudden dislocations.\n\n'
                    '• Funding Extremeness — distance of funding rate from neutral\n'
                    '• OI Expansion — open-interest growth vs baseline\n'
                    '• Long/Short Imbalance — skew in positioning\n'
                    '• Liquidation Proximity — how close price is to '
                    'liquidation clusters\n\n'
                    'High fragility suggests a stop-hunt or squeeze is more likely.',
              ),
              child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
            ),
            const Spacer(),
            Text(
              FragilityData.riskLabel(data.riskLevel),
              style: TextStyle(
                color: riskColor,
                fontSize: 12,
              ),
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
        if (data.dominantSide != 'neutral') ...[          Row(
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
                style: const TextStyle(
                  color: Colors.white54,
                  fontSize: 12,
                ),
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
                body: 'Inferred crowd-sentiment dimensions derived from '
                    'funding rates, open-interest dynamics, and '
                    'order-flow imbalances.\n\n'
                    '• Greed — aggressive long positioning\n'
                    '• Fear — defensive / hedging bias\n'
                    '• Patience — low activity, wait-and-see\n'
                    '• Panic — capitulation signals',
              ),
              child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
            ),
            const Spacer(),
            Text(
              data.summary,
              style: const TextStyle(
                color: Colors.white54,
                fontSize: 12,
              ),
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

  Widget _buildScoreBreakdown(DetailContext ctx) {
    // Normalize directional metrics to totalScore (not 100%).
    // A weak total score (e.g. 0.30) compresses all bars, conveying that
    // the overall signal confidence is low — same approach as Setup Quality.
    final rawTrend = ctx.trendScore.abs();
    final rawSideways = ctx.sidewaysScore.abs();
    final rawCompression = ctx.compressionScore.abs();
    final rawBreakoutUp = ctx.breakoutUpScore.abs();
    final rawBreakoutDown = ctx.breakoutDownScore.abs();
    final metricSum = rawTrend + rawSideways + rawCompression + rawBreakoutUp + rawBreakoutDown;
    final total = ctx.totalScore.clamp(0.0, 1.0);
    final trendPct = metricSum > 0 ? (rawTrend / metricSum) * total : 0.0;
    final sidewaysPct = metricSum > 0 ? (rawSideways / metricSum) * total : 0.0;
    final compressionPct = metricSum > 0 ? (rawCompression / metricSum) * total : 0.0;
    final breakoutUpPct = metricSum > 0 ? (rawBreakoutUp / metricSum) * total : 0.0;
    final breakoutDownPct = metricSum > 0 ? (rawBreakoutDown / metricSum) * total : 0.0;

    // Direction coloring: trend uses sign, compression uses sign heuristic,
    // breakout up = green, breakout down = red, sideways = gray.
    final trendColor = ctx.trendScore >= 0 ? Colors.green : Colors.red;
    final compressionColor = Colors.amber;
    const sidewaysColor = Colors.grey;

    return _fieldset('Metrics Breakdown', [
      _metricBar('Trend:', trendPct, '${(trendPct * 100).toStringAsFixed(0)}%', trendColor),
      _metricBar('Sideways:', sidewaysPct, '${(sidewaysPct * 100).toStringAsFixed(0)}%', sidewaysColor),
      _metricBar('Compression:', compressionPct, '${(compressionPct * 100).toStringAsFixed(0)}%', compressionColor),
      _metricBar('Breakout Up:', breakoutUpPct, '${(breakoutUpPct * 100).toStringAsFixed(0)}%', Colors.green),
      _metricBar('Breakout Down:', breakoutDownPct, '${(breakoutDownPct * 100).toStringAsFixed(0)}%', Colors.red),
    ], hint: 'Proportional weight of each regime detector '
        'normalised to overall conviction (total score).\n\n'
        '• Trend — directional strength (slope × R²)\n'
        '• Sideways — range-bound, low-volatility character\n'
        '• Compression — narrowing Bollinger bandwidth\n'
        '• Breakout Up / Down — price escaping a compression zone');
  }

  Widget _buildPriceAction(DetailContext ctx) {
    final gain = ctx.gainScore;
    final pct = (gain * 100).abs();
    final color = gain >= 0 ? Colors.green : Colors.red;
    final label = gain >= 0 ? 'Gainer' : 'Loser';
    return _fieldset('Price Action', [
      _metricBar('$label:', gain.abs().clamp(0.0, 1.0), '${pct.toStringAsFixed(0)}%', color),
    ], hint: 'Net return detected over the scoring window.\n\n'
        'Gainer — positive price change.\n'
        'Loser — negative price change.\n\n'
        'Bar width shows magnitude relative to 100%.');
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

    return _fieldset('Setup Quality — $totalDisplay', [
      for (final entry in data.scores.entries)
        _metricBar(
          '${SetupData.displayName(entry.key)}:',
          subSum > 0 ? (entry.value / subSum) * totalPct : 0.0,
          '${(subSum > 0 ? (entry.value / subSum) * totalPct * 100 : 0.0).toStringAsFixed(0)}%',
          colors[entry.key] ?? Colors.grey,
        ),
    ], hint: 'Tradability assessment — how well the current '
        'price structure matches known setup archetypes.\n\n'
        '• Compression Breakout — tight range about to break\n'
        '• Trend Continuation — pullback within a strong trend\n'
        '• Range Reversion — mean-reversion at range edges');
  }

  // ---- shared fieldset & metric bar helpers ----

  Widget _fieldset(String title, List<Widget> children, {String? hint}) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Text(
              title,
              style: const TextStyle(
                color: Colors.white70,
                fontWeight: FontWeight.w600,
                fontSize: 14,
              ),
            ),
            if (hint != null) ...[
              const SizedBox(width: 4),
              GestureDetector(
                onTap: () => _showInfoDialog(title: title, body: hint),
                child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
              ),
            ],
          ],
        ),
        const SizedBox(height: 8),
        ...children,
      ],
    );
  }

  Widget _metricBar(String label, double fraction, String display, Color color) {
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
            ],
          ),
        ],
      ),
    );
  }
}
