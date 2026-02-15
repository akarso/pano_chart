import 'package:flutter/material.dart';
import '../candles/api/candle_response.dart';
import '../detail/detail_screen.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';

/// Overview widget that displays a scrollable grid of market charts.
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
              return _OverviewGridItem(
                item: item,
                timeframe: _timeframe,
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
  final String timeframe;

  const _OverviewGridItem({
    required this.item,
    required this.timeframe,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: () {
        Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => DetailScreen(
              symbol: AppSymbol(item.symbol),
              timeframe: Timeframe(timeframe),
              series: CandleSeriesResponse(
                symbol: item.symbol,
                timeframe: timeframe,
                candles: item.candles,
              ),
            ),
          ),
        );
      },
      child: Card(
        child: AspectRatio(
          aspectRatio: 2.5,
          child: Stack(
            children: [
              Padding(
                padding: const EdgeInsets.all(8.0),
                child: _buildChartArea(item.candles),
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
                child: _buildPercentChange(context, item.candles),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildChartArea(List<CandleDto> candles) {
    if (candles.isEmpty) return const Center(child: Text('No data'));
    return CustomPaint(
      painter: MiniChartRenderer(candles),
      size: Size.infinite,
    );
  }

  Widget _buildPercentChange(BuildContext context, List<CandleDto> candles) {
    if (candles.length < 2) return const SizedBox.shrink();
    final first = candles.first.close;
    final last = candles.last.close;
    final pct = ((last - first) / first) * 100;
    final color = pct >= 0 ? Colors.green : Colors.red;
    return Text(
      '${pct >= 0 ? '+' : ''}${pct.toStringAsFixed(2)}%',
      style: Theme.of(context).textTheme.labelLarge?.copyWith(
                color: color.withAlpha((0.85 * 255).round()),
                backgroundColor: Colors.black.withAlpha((0.15 * 255).round()),
              ) ??
          const TextStyle(),
    );
  }
}

class MiniChartRenderer extends CustomPainter {
  final List<CandleDto> candles;
  MiniChartRenderer(this.candles);

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = Colors.blue;
    if (candles.isEmpty) return;

    final minVal = candles.map((c) => c.low).reduce((a, b) => a < b ? a : b);
    final maxVal = candles.map((c) => c.high).reduce((a, b) => a > b ? a : b);
    final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);

    final w = size.width / candles.length;
    for (var i = 0; i < candles.length; i++) {
      final c = candles[i];
      final openY = size.height - ((c.open - minVal) / range) * size.height;
      final closeY = size.height - ((c.close - minVal) / range) * size.height;
      final top = size.height - ((c.high - minVal) / range) * size.height;
      final bottom = size.height - ((c.low - minVal) / range) * size.height;

      // Draw wick
      final x = i * w + w / 2;
      canvas.drawLine(
          Offset(x, top), Offset(x, bottom), paint..strokeWidth = 1);

      // Draw body
      final rect = Rect.fromLTRB(i * w + 1, openY, (i + 1) * w - 1, closeY);
      canvas.drawRect(rect, paint..style = PaintingStyle.fill);
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
