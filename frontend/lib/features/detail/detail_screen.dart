import 'package:flutter/material.dart';
import '../candles/api/candle_response.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import 'candle_series_chart_renderer.dart';
import '../../domain/series_view_mode.dart';
import '../events/chart_event_overlay.dart';
import '../events/event_filter.dart';
import '../events/event_marker_builder.dart';
import '../events/events_list_screen.dart';
import '../events/events_view_model.dart';
import '../overview/line_series_chart_renderer.dart';
import 'detail_context.dart';
import 'sticky_price_labels.dart';

/// DetailScreen displays a single symbol in detail with candle chart,
/// header block, time context, score breakdown, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;
  final DetailContext? detailContext;
  final bool isFavourite;
  final EventsViewModel? eventsViewModel;

  const DetailScreen({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.series,
    this.detailContext,
    this.isFavourite = false,
    this.eventsViewModel,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
    SeriesViewMode _viewMode = SeriesViewMode.candles;
  late bool isFavourite;

  @override
  void initState() {
    super.initState();
    isFavourite = widget.isFavourite;
    _loadEvents();
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

  bool get _shouldShowEvents =>
      widget.eventsViewModel != null && widget.eventsViewModel!.state.showEvents;

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
              // Chart view toggle
              ToggleButtons(
                isSelected: [
                  _viewMode == SeriesViewMode.candles,
                  _viewMode == SeriesViewMode.line,
                ],
                borderRadius: BorderRadius.circular(8),
                constraints: const BoxConstraints(minWidth: 36, minHeight: 32),
                onPressed: (idx) {
                  setState(() {
                    _viewMode = idx == 0 ? SeriesViewMode.candles : SeriesViewMode.line;
                  });
                },
                children: const [
                  Icon(Icons.candlestick_chart, color: Colors.white),
                  Icon(Icons.show_chart, color: Colors.white),
                ],
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
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
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
              Container(
                decoration: BoxDecoration(                  
                  borderRadius: BorderRadius.circular(8),
                ),
                child: SizedBox(
                  height: 260,
                  child: LayoutBuilder(
                    builder: (context, constraints) {
                      final candleCount = series.candles.length;
                      const double candleWidth = 14; // px per candle
                      final chartWidth = (candleCount * candleWidth).clamp(constraints.maxWidth, 9999.0);
                      // Overlay price labels on the right, sticky and high z-index
                      return Stack(
                        children: [
                          Stack(
                            children: [
                              Row(
                                children: [
                                  Expanded(
                                    child: SingleChildScrollView(
                                      scrollDirection: Axis.horizontal,
                                      physics: const BouncingScrollPhysics(),
                                      child: Card(
                                        color: Colors.grey[900],
                                        shape: RoundedRectangleBorder(
                                          borderRadius: BorderRadius.circular(12),
                                        ),
                                        child: Padding(
                                          padding: const EdgeInsets.all(16),
                                          child: SizedBox(
                                            width: chartWidth,
                                            height: 220,
                                            child: Stack(
                                              children: [
                                                _viewMode == SeriesViewMode.candles
                                                    ? CandleSeriesChartRenderer().build(
                                                        context,
                                                        series: series,
                                                        viewMode: SeriesViewMode.candles,
                                                        candleWidth: candleWidth,
                                                      )
                                                    : LineSeriesChartRenderer().build(
                                                        context,
                                                        series: series,
                                                        viewMode: SeriesViewMode.line,
                                                      ),
                                                // Event badge overlay
                                                if (_shouldShowEvents)
                                                  Positioned.fill(
                                                    child: ChartEventOverlay(
                                                      markers: buildEventMarkers(
                                                        series: series,
                                                        events: widget.eventsViewModel?.state.filteredEvents ?? [],
                                                        filterLevel: widget.eventsViewModel?.state.filterLevel ?? EventFilterLevel.highAndMedium,
                                                        candleWidth: candleWidth,
                                                      ),
                                                      onNavigateToEvent: (eventId) => _navigateToEventsList(eventId),
                                                    ),
                                                  ),
                                              ],
                                            ),
                                          ),
                                        ),
                                      ),
                                    ),
                                  ),
                                ],
                              ),
                              // Sticky price labels overlay (full width, higher z-index)
                              Positioned.fill(
                                child: IgnorePointer(
                                  child: StickyPriceLabels(series: series),
                                ),
                              ),
                            ],
                          ),
                          // Scroll cue overlay (fade left/right)
                          Positioned.fill(
                            child: IgnorePointer(
                              child: Row(
                                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                                children: [
                                  Container(
                                    width: 24,
                                    decoration: const BoxDecoration(
                                      gradient: LinearGradient(
                                        begin: Alignment.centerLeft,
                                        end: Alignment.centerRight,
                                        colors: [Colors.black54, Colors.transparent],
                                      ),
                                    ),
                                    child: const Align(
                                      alignment: Alignment.centerLeft,
                                      child: Icon(Icons.arrow_back_ios, size: 14, color: Colors.white38),
                                    ),
                                  ),
                                  Container(
                                    width: 24,
                                    decoration: const BoxDecoration(
                                      gradient: LinearGradient(
                                        begin: Alignment.centerRight,
                                        end: Alignment.centerLeft,
                                        colors: [Colors.black54, Colors.transparent],
                                      ),
                                    ),
                                    child: const Align(
                                      alignment: Alignment.centerRight,
                                      child: Icon(Icons.arrow_forward_ios, size: 14, color: Colors.white38),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ],
                      );
                    },
                  ),
                ),
              ),
            // Event overlay controls
            if (widget.eventsViewModel != null) ...[
              const SizedBox(height: 8),
              _buildEventControls(),
            ],
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
