import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../candles/api/candle_response.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../events/event_filter.dart';
import '../events/events_list_screen.dart';
import '../events/events_view_model.dart';
import 'chart/chart_config.dart';
import 'chart/indicator_panel.dart';
import 'chart/interactive_chart.dart';
import 'detail_context.dart';
import 'trade/trade_action_buttons.dart';
import 'trade/trade_links.dart';

/// DetailScreen displays a single symbol in detail with candle chart,
/// header block, time context, score breakdown, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;
  final DetailContext? detailContext;
  final bool isFavourite;
  final EventsViewModel? eventsViewModel;

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
    this.warmupCount = 0,
    this.initialVisibleCount = 30,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
  ChartIndicatorConfig _chartConfig = const ChartIndicatorConfig();
  late bool isFavourite;
  Exchange _exchange = Exchange.binance;

  @override
  void initState() {
    super.initState();
    isFavourite = widget.isFavourite;
    _loadChartConfig();
    _loadExchangePreference();
    _loadEvents();
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

  Future<void> _loadExchangePreference() async {
    final prefs = await SharedPreferences.getInstance();
    final key = prefs.getString('settings.preferredExchange') ?? 'binance';
    setState(() {
      _exchange = ExchangeLabel.fromKey(key);
    });
  }

  Future<void> _saveExchange(Exchange ex) async {
    setState(() => _exchange = ex);
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('settings.preferredExchange', ex.key);
  }

  void _loadEvents() {
    final evm = widget.eventsViewModel;
    if (evm == null) return;
    evm.onChanged = () {
      if (mounted) setState(() {});
    };
    // Determine date range from the candle series
    final candles = widget.series.candles;
    if (candles.isEmpty) return;
    final dateFrom = _isoDate(candles.first.timestamp);
    final dateTo = _isoDate(candles.last.timestamp);
    evm.load(dateFrom, dateTo);
  }

  String _isoDate(DateTime dt) {
    final y = dt.year.toString().padLeft(4, '0');
    final m = dt.month.toString().padLeft(2, '0');
    final d = dt.day.toString().padLeft(2, '0');
    return '$y-$m-$d';
  }

  // ---- helpers ----

  String _formatVolume(double vol) {
    if (vol >= 1e9) return '\$${(vol / 1e9).toStringAsFixed(1)}B';
    if (vol >= 1e6) return '\$${(vol / 1e6).toStringAsFixed(1)}M';
    if (vol >= 1e3) return '\$${(vol / 1e3).toStringAsFixed(1)}K';
    return '\$${vol.toStringAsFixed(0)}';
  }

  String _timeRangeLabel() {
    final count = widget.series.candles.length;
    final tf = widget.timeframe.value;
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
        builder: (_) => EventsListScreen(
          events: evm.state.events,
          filterLevel: evm.state.filterLevel,
          scrollToEventId: scrollToEventId,
        ),
      ),
    );
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final series = widget.series;
    final candles = series.candles;
    final ctx = widget.detailContext;

    double? percentChange;
    if (candles.length >= 2) {
      final first = candles.first.close;
      final last = candles.last.close;
      percentChange = ((last - first) / first) * 100;
    }

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
                child: Text(
                  widget.timeframe.value,
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
            IconButton(
              icon: const Icon(Icons.close, color: Colors.white),
              onPressed: () => Navigator.of(context).maybePop(isFavourite),
              tooltip: 'Close',
            ),
          ],
        ),
        body: SingleChildScrollView(
          padding: EdgeInsets.only(
            left: 16, right: 16, top: 8,
            bottom: 8 + MediaQuery.viewPaddingOf(context).bottom,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (ctx != null) _buildHeaderBlock(ctx, percentChange),
              if (ctx != null) const SizedBox(height: 12),
              Text(
                _timeRangeLabel(),
                style: const TextStyle(
                  color: Colors.white38,
                  fontSize: 12,
                ),
              ),
              const SizedBox(height: 8),
              InteractiveChart(
                series: series,
                config: _chartConfig,
                onConfigChanged: _saveChartConfig,
                eventsViewModel: widget.eventsViewModel,
                onNavigateToEvent: _navigateToEventsList,
                warmupCount: widget.warmupCount,
                initialVisibleCount: widget.initialVisibleCount,
              ),
            // Event overlay controls
            if (widget.eventsViewModel != null) ...[
              const SizedBox(height: 8),
              _buildEventControls(),
            ],
            const SizedBox(height: 12),
            TradeActionButtons(
              symbol: widget.symbol.value,
              timeframe: widget.timeframe.value,
              exchange: _exchange,
              onExchangeChanged: _saveExchange,
            ),
            if (percentChange != null) ...[
              const SizedBox(height: 12),
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    '${percentChange > 0 ? '+' : ''}${percentChange.toStringAsFixed(2)}%',
                    style: TextStyle(
                      color: percentChange > 0 ? Colors.green : Colors.red,
                      fontWeight: FontWeight.bold,
                      fontSize: 16,
                    ),
                  ),
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

  Widget _buildHeaderBlock(DetailContext ctx, double? percentChange) {
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
            if (percentChange != null)
              Text(
                '24h Change: ${percentChange > 0 ? '+' : ''}${percentChange.toStringAsFixed(1)}%',
                style: TextStyle(
                  color: percentChange > 0 ? Colors.green : Colors.red,
                  fontSize: 13,
                ),
              ),
            if (percentChange != null) const SizedBox(width: 16),
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
