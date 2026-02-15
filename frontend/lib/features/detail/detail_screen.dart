import 'package:flutter/material.dart';
import '../candles/api/candle_response.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import 'candle_series_chart_renderer.dart';
import '../../domain/series_view_mode.dart';

/// DetailScreen displays a single symbol in detail with candle chart, metadata, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;

  const DetailScreen({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.series,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
  bool isFavourite = false;

  @override
  Widget build(BuildContext context) {
    final timeframe = widget.timeframe;
    final series = widget.series;
    final candles = series.candles;
    double? percentChange;
    double? lastPrice;
    if (candles.length >= 2) {
      final first = candles.first.close;
      final last = candles.last.close;
      percentChange = ((last - first) / first) * 100;
      lastPrice = last;
    }

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.close, color: Colors.white),
          onPressed: () => Navigator.of(context).maybePop(),
          tooltip: 'Back',
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
                timeframe.value,
                style: const TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                ),
              ),
            ),
            const Spacer(),
            IconButton(
              icon: Icon(
                isFavourite ? Icons.star : Icons.star_border,
                color: isFavourite ? Colors.amber : Colors.white54,
              ),
              onPressed: () => setState(() => isFavourite = !isFavourite),
              tooltip: isFavourite ? 'Unfavourite' : 'Favourite',
            ),
          ],
        ),
      ),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            AspectRatio(
              aspectRatio: 2.5,
              child: Card(
                color: Colors.grey[900],
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: CandleSeriesChartRenderer().build(
                    context,
                    series: series,
                    viewMode: SeriesViewMode.candles,
                  ),
                ),
              ),
            ),
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                if (percentChange != null)
                  Text(
                    '${percentChange > 0 ? '+' : ''}${percentChange.toStringAsFixed(2)}%',
                    style: TextStyle(
                      color: percentChange > 0 ? Colors.green : Colors.red,
                      fontWeight: FontWeight.bold,
                      fontSize: 16,
                    ),
                  ),
                if (lastPrice != null)
                  Text(
                    '${lastPrice.toStringAsFixed(2)}',
                    style: const TextStyle(
                      color: Colors.white70,
                      fontSize: 16,
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
