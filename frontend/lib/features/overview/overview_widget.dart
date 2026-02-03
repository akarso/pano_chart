import 'package:flutter/material.dart';
import '../candles/application/get_candle_series.dart';
import '../candles/application/get_candle_series_input.dart';
import '../candles/api/candle_response.dart';

/// Simple overview widget that loads multiple candle series via the
/// provided [GetCandleSeries] use case and displays them in a scrollable list.
class OverviewWidget extends StatefulWidget {
  final GetCandleSeries useCase;
  final List<GetCandleSeriesInput> items;

  const OverviewWidget({Key? key, required this.useCase, required this.items})
      : super(key: key);

  @override
  _OverviewWidgetState createState() => _OverviewWidgetState();
}

class _OverviewWidgetState extends State<OverviewWidget> {
  bool _isLoading = true;
  late List<CandleSeriesResponse?> _responses;

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

    return ListView.builder(
      itemCount: widget.items.length,
      itemBuilder: (context, index) {
        final input = widget.items[index];
        final resp = _responses[index];
        return SizedBox(
          height: 120,
          child: Card(
            margin: const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
            child: Padding(
              padding: const EdgeInsets.all(8.0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('${input.symbol} • ${input.timeframe}',
                      style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  Expanded(
                    child: _buildChartArea(resp),
                  ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  Widget _buildChartArea(CandleSeriesResponse? resp) {
    if (resp == null) return const SizedBox.shrink();
    if (resp.candles.isEmpty) return const Center(child: Text('No candles'));

    // Minimal visualisation: draw simple bars using a CustomPaint
    return CustomPaint(
      painter: _MiniChartPainter(resp.candles),
      size: Size.infinite,
    );
  }
}

class _MiniChartPainter extends CustomPainter {
  final List<CandleDto> candles;
  _MiniChartPainter(this.candles);

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
