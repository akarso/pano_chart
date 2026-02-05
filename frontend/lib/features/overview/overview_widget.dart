import 'package:flutter/material.dart';
import '../candles/application/get_candle_series.dart';
import '../candles/application/get_candle_series_input.dart';
import '../candles/api/candle_response.dart';
import '../detail/detail_screen.dart';
import '../../domain/symbol.dart';

/// Simple overview widget that loads multiple time series via the
/// provided [GetCandleSeries] use case and displays them in a scrollable list.
///
/// Default view mode: [SeriesViewMode.line] for overview, [SeriesViewMode.candles] for detail.
class OverviewWidget extends StatefulWidget {
  final GetCandleSeries useCase;
  final List<GetCandleSeriesInput> items;

  final dynamic viewModel; // Add this if not present in your codebase
  const OverviewWidget(
      {Key? key,
      required this.useCase,
      required this.items,
      required this.viewModel})
      : super(key: key);

  @override
  OverviewWidgetState createState() => OverviewWidgetState();
}

class OverviewWidgetState extends State<OverviewWidget> {
  bool _isLoading = true;
  late List<CandleSeriesResponse?> _responses;
  int _columns = 2;
  String _timeframe = '1h';

  @override
  void initState() {
    super.initState();
    _responses = List<CandleSeriesResponse?>.filled(widget.items.length, null);
    _loadAll();
  }

  Future<void> _loadAll() async {
    setState(() => _isLoading = true);
    final futures = <Future<void>>[];
    for (var i = 0; i < widget.items.length; i++) {
      final idx = i;
      final input = widget.items[i];
      futures.add(widget.useCase.execute(input).then((resp) {
        _responses[idx] = resp;
      }));
    }
    await Future.wait(futures);
    if (mounted) setState(() => _isLoading = false);
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) return const Center(child: CircularProgressIndicator());

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
                onChanged: (v) => setState(() => _timeframe = v ?? '1h'),
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
            itemCount: widget.items.length,
            itemBuilder: (context, index) {
              final input = widget.items[index];
              final resp = _responses[index];
              return _OverviewGridItem(
                symbol: input.symbol,
                timeframe: input.timeframe,
                response: resp,
                viewModel: widget.viewModel,
              );
            },
          ),
        ),
      ],
    );
  }
}

class _OverviewGridItem extends StatelessWidget {
  final String symbol;
  final String timeframe;
  final CandleSeriesResponse? response;
  final dynamic viewModel;
  const _OverviewGridItem({
    required this.symbol,
    required this.timeframe,
    required this.response,
    required this.viewModel,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: () {
        Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => DetailScreen(
              symbol: AppSymbol(symbol),
              viewModel: viewModel,
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
                child: _buildChartArea(response),
              ),
              Positioned(
                left: 12,
                top: 8,
                child: Text(
                  symbol,
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
                child: _buildPercentChange(context, response),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildChartArea(CandleSeriesResponse? resp) {
    if (resp == null) return const SizedBox.shrink();
    if (resp.candles.isEmpty) return const Center(child: Text('No data'));
    return CustomPaint(
      painter: MiniChartRenderer(resp.candles),
      size: Size.infinite,
    );
  }

  Widget _buildPercentChange(BuildContext context, CandleSeriesResponse? resp) {
    if (resp == null || resp.candles.length < 2) return const SizedBox.shrink();
    final first = resp.candles.first.close;
    final last = resp.candles.last.close;
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
