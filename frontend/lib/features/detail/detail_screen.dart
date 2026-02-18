import 'package:flutter/material.dart';
import '../candles/api/candle_response.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import 'candle_series_chart_renderer.dart';
import '../../domain/series_view_mode.dart';
import 'detail_context.dart';

/// DetailScreen displays a single symbol in detail with candle chart,
/// header block, time context, score breakdown, and favourite toggle.
class DetailScreen extends StatefulWidget {
  final AppSymbol symbol;
  final Timeframe timeframe;
  final CandleSeriesResponse series;
  final DetailContext? detailContext;

  const DetailScreen({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.series,
    this.detailContext,
  }) : super(key: key);

  @override
  State<DetailScreen> createState() => _DetailScreenState();
}

class _DetailScreenState extends State<DetailScreen> {
  bool isFavourite = false;

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

    return Scaffold(
      backgroundColor: Colors.black,
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
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.close, color: Colors.white),
            onPressed: () => Navigator.of(context).maybePop(),
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
            AspectRatio(
              aspectRatio: 1.8,
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
