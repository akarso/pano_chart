import 'package:flutter/material.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../candles/application/get_candle_series.dart';
import '../candles/application/get_candle_series_input.dart';
import '../detail/detail_screen.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';

/// Overview widget that displays a scrollable grid of market sparklines.
///
/// All data and loading state is owned by [OverviewViewModel].
/// Widget rebuilds via [OverviewViewModel.onChanged] callback.
class OverviewWidget extends StatefulWidget {
  final OverviewViewModel viewModel;
  final GetCandleSeries getCandleSeries;

  const OverviewWidget({
    Key? key,
    required this.viewModel,
    required this.getCandleSeries,
  }) : super(key: key);

  @override
  OverviewWidgetState createState() => OverviewWidgetState();
}

class OverviewWidgetState extends State<OverviewWidget> {
  late final OverviewViewModel vm;
  int _columns = 2;
  String _timeframe = '1h';

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;
    vm.onChanged = () => setState(() {});
    vm.loadInitial(_timeframe);
  }

  @override
  void dispose() {
    vm.onChanged = null;
    super.dispose();
  }

  /// Returns the duration of one candle for the given timeframe string.
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

  /// Number of candles to fetch — must match backend sparkline precision.
  static const int _precision = 30;

  Future<void> _onItemTapped(OverviewItem item) async {
    final now = DateTime.now().toUtc();
    final from = now.subtract(_candleDuration(_timeframe) * _precision);
    final input = GetCandleSeriesInput(
      symbol: item.symbol,
      timeframe: _timeframe,
      from: from,
      to: now,
    );

    // Show a loading dialog while fetching candles.
    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (_) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      Navigator.of(context).pop(); // dismiss loading dialog
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(item.symbol),
            timeframe: Timeframe(_timeframe),
            series: series,
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop(); // dismiss loading dialog
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load chart: $e')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = vm.state;

    if (state.isLoading && state.items.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }

    if (state.error != null && state.items.isEmpty) {
      return Center(child: Text(state.error!));
    }

    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          child: Row(
            children: [
              const Text('Columns:'),
              const SizedBox(width: 8),
              DropdownButton<int>(
                value: _columns,
                items: const [1, 2, 3]
                    .map((c) => DropdownMenuItem(value: c, child: Text('$c')))
                    .toList(),
                onChanged: (v) => setState(() => _columns = v ?? 2),
              ),
              const SizedBox(width: 24),
              const Text('Timeframe:'),
              const SizedBox(width: 8),
              DropdownButton<String>(
                value: _timeframe,
                items: const ['1m', '5m', '15m', '1h', '4h', '1d']
                    .map((tf) => DropdownMenuItem(value: tf, child: Text(tf)))
                    .toList(),
                onChanged: (v) {
                  setState(() => _timeframe = v ?? '1h');
                  vm.loadInitial(_timeframe);
                },
              ),
            ],
          ),
        ),
        Expanded(
          child: GridView.builder(
            padding: const EdgeInsets.all(8),
            gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
              crossAxisCount: _columns,
              crossAxisSpacing: 8,
              mainAxisSpacing: 8,
              childAspectRatio: 2.5,
            ),
            itemCount: state.items.length,
            itemBuilder: (context, index) {
              final item = state.items[index];
              return GestureDetector(
                onTap: () => _onItemTapped(item),
                child: _OverviewGridItem(item: item),
              );
            },
          ),
        ),
      ],
    );
  }
}

class _OverviewGridItem extends StatelessWidget {
  final OverviewItem item;

  const _OverviewGridItem({required this.item});

  @override
  Widget build(BuildContext context) {
    return Card(
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
                  child: _buildSparkline(item.sparkline),
                ),
                Positioned(
                  left: pad + 4,
                  top: pad,
                  child: Text(
                    item.symbol,
                    style: TextStyle(
                      fontSize: fontSize,
                      fontWeight: FontWeight.w600,
                      color: Colors.white.withAlpha((0.85 * 255).round()),
                      backgroundColor:
                          Colors.black.withAlpha((0.25 * 255).round()),
                    ),
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _buildSparkline(List<double> points) {
    if (points.isEmpty) return const Center(child: Text('No data'));
    return CustomPaint(
      painter: SparklineRenderer(points),
      size: Size.infinite,
    );
  }
}

/// Draws a simple sparkline (line chart) from a list of values.
class SparklineRenderer extends CustomPainter {
  final List<double> points;
  SparklineRenderer(this.points);

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2) return;

    final minVal = points.reduce((a, b) => a < b ? a : b);
    final maxVal = points.reduce((a, b) => a > b ? a : b);
    final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);

    final paint = Paint()
      ..color = points.last >= points.first ? Colors.green : Colors.red
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final path = Path();
    for (var i = 0; i < points.length; i++) {
      final x = (i / (points.length - 1)) * size.width;
      final y = size.height - ((points[i] - minVal) / range) * size.height;
      if (i == 0) {
        path.moveTo(x, y);
      } else {
        path.lineTo(x, y);
      }
    }
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant SparklineRenderer oldDelegate) {
    return !identical(points, oldDelegate.points);
  }
}
