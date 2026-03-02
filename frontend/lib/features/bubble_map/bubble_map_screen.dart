import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../candles/application/get_candle_series.dart';
import '../candles/application/get_candle_series_input.dart';
import '../detail/detail_context.dart';
import '../detail/detail_screen.dart';
import '../events/events_view_model.dart';
import 'bubble_map_state.dart';
import 'bubble_map_view_model.dart';
import 'bubble_packer.dart';
import 'bubble_painter.dart';

/// Available timeframes for the bubble map.
const _timeframes = ['1m', '5m', '15m', '1h', '4h', '1d'];

/// Page labels shown in the dropdown.
const _pageLabels = ['Top 50', '51 – 100', '101 – 150'];

/// Bubble map screen showing tokens as size/colour-coded circles.
class BubbleMapScreen extends StatefulWidget {
  final BubbleMapViewModel viewModel;
  final GetCandleSeries getCandleSeries;
  final EventsViewModel? eventsViewModel;

  const BubbleMapScreen({
    Key? key,
    required this.viewModel,
    required this.getCandleSeries,
    this.eventsViewModel,
  }) : super(key: key);

  @override
  State<BubbleMapScreen> createState() => _BubbleMapScreenState();
}

class _BubbleMapScreenState extends State<BubbleMapScreen> {
  late final BubbleMapViewModel vm;
  int _highlightIndex = -1;

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;
    vm.onChanged = () {
      if (mounted) setState(() {});
    };
  }

  @override
  void dispose() {
    vm.onChanged = null;
    super.dispose();
  }

  // ---- navigation ----

  Duration _candleDuration(String tf) {
    switch (tf) {
      case '1m':
        return const Duration(minutes: 1);
      case '5m':
        return const Duration(minutes: 5);
      case '15m':
        return const Duration(minutes: 15);
      case '1h':
        return const Duration(hours: 1);
      case '4h':
        return const Duration(hours: 4);
      case '1d':
        return const Duration(days: 1);
      default:
        return const Duration(hours: 1);
    }
  }

  static const int _sparklineCandles = 30;

  String _detailTimeframe(String overviewTf) {
    final idx = _timeframes.indexOf(overviewTf);
    if (idx <= 0) return overviewTf;
    return _timeframes[idx - 1];
  }

  Future<void> _onBubbleTap(PackedBubble bubble) async {
    final token = bubble.token;
    final now = DateTime.now().toUtc();
    final tf = vm.state.timeframe;
    final timespan = _candleDuration(tf) * _sparklineCandles;
    final from = now.subtract(timespan);
    final detailTf = _detailTimeframe(tf);

    final input = GetCandleSeriesInput(
      symbol: token.symbol,
      timeframe: detailTf,
      from: from,
      to: now,
    );

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (_) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      Navigator.of(context).pop(); // dismiss loading
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(token.symbol),
            timeframe: Timeframe(detailTf),
            series: series,
            eventsViewModel: widget.eventsViewModel,
            detailContext: DetailContext(
              rank: 0,
              totalScore: token.totalScore,
              trendScore: token.trendScore,
              sidewaysScore: token.sidewaysScore,
              gainScore: token.gainScore,
              volume: token.volume,
            ),
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load chart: $e')),
      );
    }
  }

  // ---- hit test ----

  int _hitTest(Offset position) {
    final bubbles = vm.state.bubbles;
    // Check in reverse so topmost (last drawn) wins.
    for (var i = bubbles.length - 1; i >= 0; i--) {
      final b = bubbles[i];
      final dx = position.dx - b.x;
      final dy = position.dy - b.y;
      if (dx * dx + dy * dy <= b.radius * b.radius) return i;
    }
    return -1;
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final state = vm.state;

    return Scaffold(
      backgroundColor: const Color(0xFF1A1A2E),
      appBar: AppBar(
        backgroundColor: const Color(0xFF1A1A2E),
        title: const Text(
          'Bubble Map',
          style: TextStyle(
            color: Color(0xFF00e6c0),
            fontWeight: FontWeight.w700,
          ),
        ),
        iconTheme: const IconThemeData(color: Colors.white),
        actions: [
          // Timeframe dropdown
          _dropdown<String>(
            value: state.timeframe,
            items: _timeframes,
            label: (v) => v,
            onChanged: (v) {
              if (v == null) return;
              _reload(timeframe: v);
            },
          ),
          const SizedBox(width: 8),
          // Page dropdown
          _dropdown<int>(
            value: state.pageIndex,
            items: List.generate(_pageLabels.length, (i) => i),
            label: (v) => _pageLabels[v],
            onChanged: (v) {
              if (v == null) return;
              _reload(pageIndex: v);
            },
          ),
          const SizedBox(width: 8),
          // Size-by toggle
          IconButton(
            icon: Icon(
              state.sizeBy == 'volume'
                  ? Icons.bar_chart
                  : Icons.show_chart,
              color: Colors.white,
            ),
            tooltip:
                state.sizeBy == 'volume' ? 'Size by volume' : 'Size by change',
            onPressed: () {
              vm.changeSizeBy(
                  state.sizeBy == 'volume' ? 'change' : 'volume');
            },
          ),
        ],
      ),
      body: _buildBody(state),
    );
  }

  Widget _buildBody(BubbleMapState state) {
    if (state.isLoading && state.bubbles.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.bubbles.isEmpty) {
      return Center(
        child: Text(state.error!, style: const TextStyle(color: Colors.white)),
      );
    }

    return LayoutBuilder(
      builder: (context, constraints) {
        final w = constraints.maxWidth;
        final h = constraints.maxHeight;

        // Trigger initial load or relayout when size changes.
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (state.bubbles.isEmpty && !state.isLoading) {
            vm.load(
              timeframe: state.timeframe,
              pageIndex: state.pageIndex,
              width: w,
              height: h,
            );
          } else {
            vm.relayout(w, h);
          }
        });

        return GestureDetector(
          onTapDown: (details) {
            final idx = _hitTest(details.localPosition);
            if (idx != _highlightIndex) {
              setState(() => _highlightIndex = idx);
            }
          },
          onTapUp: (details) {
            final idx = _hitTest(details.localPosition);
            if (idx >= 0) {
              _onBubbleTap(state.bubbles[idx]);
            }
            setState(() => _highlightIndex = -1);
          },
          onTapCancel: () {
            if (_highlightIndex != -1) {
              setState(() => _highlightIndex = -1);
            }
          },
          child: CustomPaint(
            size: Size(w, h),
            painter: BubblePainter(
              bubbles: state.bubbles,
              highlightIndex: _highlightIndex,
            ),
          ),
        );
      },
    );
  }

  // ---- helpers ----

  void _reload({String? timeframe, int? pageIndex}) {
    final w = context.size?.width ?? 0;
    final h = (context.size?.height ?? 0) - kToolbarHeight;
    vm.load(
      timeframe: timeframe ?? vm.state.timeframe,
      pageIndex: pageIndex ?? vm.state.pageIndex,
      width: w,
      height: math.max(h, 0),
    );
  }

  Widget _dropdown<T>({
    required T value,
    required List<T> items,
    required String Function(T) label,
    required ValueChanged<T?> onChanged,
  }) {
    return DropdownButtonHideUnderline(
      child: DropdownButton<T>(
        value: value,
        dropdownColor: const Color(0xFF2A2A4A),
        style: const TextStyle(color: Colors.white, fontSize: 13),
        items: items
            .map((e) => DropdownMenuItem(value: e, child: Text(label(e))))
            .toList(),
        onChanged: onChanged,
      ),
    );
  }
}
