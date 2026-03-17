import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../candles/api/candle_response.dart';
import '../candles/application/get_candle_series.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../../infrastructure/preferences_service.dart';
import '../events/event_filter.dart';
import '../events/events_view_model.dart';
import '../events/macro_events_screen.dart';
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

/// DetailScreen displays a single symbol in detail with candle chart,
/// header block, time context, score breakdown, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;
  final DetailContext? detailContext;
  final bool isFavourite;
  final EventsViewModel? eventsViewModel;

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

  const DetailScreen({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.series,
    this.detailContext,
    this.isFavourite = false,
    this.eventsViewModel,
    this.setupApi,
    this.fragilityApi,
    this.behaviorApi,
    this.getCandleSeries,
    this.warmupCount = 0,
    this.initialVisibleCount = 30,
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
    _loadSetupData();
    _loadFragilityData();
    _loadBehaviorData();
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
    } catch (_) {
      if (mounted) setState(() => _isLoadingTf = false);
    }
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
          physics: const NeverScrollableScrollPhysics(),
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
                  warmupCount: _warmupCount,
                  initialVisibleCount: widget.initialVisibleCount,
                  referenceStartIndex: _referenceStartIndex,
                ),
            // Event overlay controls
            if (widget.eventsViewModel != null) ...[
              const SizedBox(height: 8),
              _buildEventControls(),
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
                    candles.last.close.toStringAsFixed(2),
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
              _buildScoreBreakdown(ctx),
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

  Widget _buildEventControls() {
    final evm = widget.eventsViewModel!;
    return Row(
      children: [
        // Show/Hide toggle
        GestureDetector(
          onTap: () => evm.toggleShowEvents(),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                evm.state.showEvents ? Icons.visibility : Icons.visibility_off,
                size: 16,
                color: evm.state.showEvents ? Colors.white70 : Colors.white30,
              ),
              const SizedBox(width: 4),
              Text(
                'Events',
                style: TextStyle(
                  color: evm.state.showEvents ? Colors.white70 : Colors.white30,
                  fontSize: 11,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 16),
        // Filter level chips
        for (final level in EventFilterLevel.values) ...[
          GestureDetector(
            onTap: () => evm.setFilterLevel(level),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
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
        const Spacer(),
        // "View all" link to events list
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

  Widget _buildSetupQuality(SetupData data) {
    final colors = <String, Color>{
      'compression_breakout': Colors.purple,
      'trend_continuation': Colors.blue,
      'range_reversion': Colors.teal,
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Text(
              'Setup Quality',
              style: TextStyle(
                color: Colors.white70,
                fontWeight: FontWeight.w600,
                fontSize: 14,
              ),
            ),
            const Spacer(),
            Text(
              SetupData.displayName(data.bestSetup),
              style: const TextStyle(
                color: Colors.white54,
                fontSize: 12,
              ),
            ),
            const SizedBox(width: 6),
            Text(
              '${(data.score * 100).toStringAsFixed(0)}%',
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.bold,
                fontSize: 13,
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),
        for (final entry in data.scores.entries) ...[
          _scoreBar(
            SetupData.displayName(entry.key),
            entry.value,
            colors[entry.key] ?? Colors.grey,
          ),
          const SizedBox(height: 6),
        ],
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
              '${(data.fragilityScore * 100).toStringAsFixed(0)}%',
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
        for (final entry in comps.entries) ...[
          _scoreBar(
            FragilityComponents.displayName(entry.key),
            entry.value,
            riskColor,
          ),
          const SizedBox(height: 6),
        ],
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
        for (final entry in data.dimensions.entries) ...[
          _scoreBar(
            BehaviorData.dimensionLabel(entry.key),
            entry.value,
            colorFor(entry.key, entry.value),
          ),
          const SizedBox(height: 6),
        ],
      ],
    );
  }

  Widget _buildScoreBreakdown(DetailContext ctx) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Score Breakdown',
          style: TextStyle(
            color: Colors.white70,
            fontWeight: FontWeight.w600,
            fontSize: 14,
          ),
        ),
        const SizedBox(height: 8),
        _scoreBar('Sideways', ctx.sidewaysScore, Colors.orange),
        const SizedBox(height: 6),
        _scoreBar('Trend', ctx.trendScore, Colors.blue),
        const SizedBox(height: 6),
        _scoreBar('Gain', ctx.gainScore, Colors.green),
      ],
    );
  }

  Widget _scoreBar(String label, double value, Color color) {
    return Row(
      children: [
        SizedBox(
          width: 70,
          child: Text(
            label,
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ),
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(3),
            child: LinearProgressIndicator(
              value: value.clamp(0.0, 1.0),
              backgroundColor: Colors.white12,
              valueColor: AlwaysStoppedAnimation<Color>(color),
              minHeight: 10,
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 36,
          child: Text(
            value.toStringAsFixed(2),
            style: const TextStyle(color: Colors.white70, fontSize: 12),
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }
}
