import 'package:flutter/material.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';

/// Overview widget that displays a scrollable grid of market sparklines.
///
/// All data and loading state is owned by [OverviewViewModel].
/// Widget rebuilds via [OverviewViewModel.onChanged] callback.
class OverviewWidget extends StatefulWidget {
  final OverviewViewModel viewModel;

  const OverviewWidget({Key? key, required this.viewModel}) : super(key: key);

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
              return _OverviewGridItem(item: item);
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
        child: Stack(
          children: [
            Padding(
              padding: const EdgeInsets.all(8.0),
              child: _buildSparkline(item.sparkline),
            ),
            Positioned(
              left: 12,
              top: 8,
              child: Text(
                item.symbol,
                style: Theme.of(context).textTheme.labelLarge?.copyWith(
                          color: Colors.white.withAlpha((0.85 * 255).round()),
                          backgroundColor:
                              Colors.black.withAlpha((0.25 * 255).round()),
                        ) ??
                    const TextStyle(),
              ),
            ),
            Positioned(
              right: 12,
              top: 8,
              child: _buildScoreBadge(context, item.totalScore),
            ),
          ],
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

  Widget _buildScoreBadge(BuildContext context, double score) {
    final color = score >= 0 ? Colors.green : Colors.red;
    return Text(
      score.toStringAsFixed(2),
      style: Theme.of(context).textTheme.labelLarge?.copyWith(
                color: color.withAlpha((0.85 * 255).round()),
                backgroundColor: Colors.black.withAlpha((0.15 * 255).round()),
              ) ??
          const TextStyle(),
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
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
