import 'dart:math' as math;
import 'package:flutter/material.dart';

import '../../../core/format_price.dart';
import '../../candles/api/candle_response.dart';

/// Widget-based Y-axis price labels that auto-scale to the visible range.
///
/// Positioned on the right side of the chart.  Uses 5–7 grid levels.
class YAxisLabels extends StatelessWidget {
  final List<CandleDto> candles;
  final int startIndex;
  final int endIndex;

  /// Optional EMA values to include in range calculation.
  final List<double>? emaFast;
  final List<double>? emaSlow;

  /// Optional externally-computed price range (e.g. with vertical scaling).
  /// When provided, overrides auto-scaling from visible candles.
  final double? priceLo;
  final double? priceHi;

  const YAxisLabels({
    Key? key,
    required this.candles,
    required this.startIndex,
    required this.endIndex,
    this.emaFast,
    this.emaSlow,
    this.priceLo,
    this.priceHi,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    if (candles.isEmpty || startIndex >= endIndex) return const SizedBox();

    double lo, hi;
    if (priceLo != null && priceHi != null) {
      lo = priceLo!;
      hi = priceHi!;
    } else {
      lo = double.infinity;
      hi = double.negativeInfinity;
      for (var i = startIndex; i < endIndex && i < candles.length; i++) {
        if (candles[i].low < lo) lo = candles[i].low;
        if (candles[i].high > hi) hi = candles[i].high;
      }
      _expandRange(emaFast, startIndex, endIndex, lo, hi, (a, b) {
        lo = a;
        hi = b;
      });
      _expandRange(emaSlow, startIndex, endIndex, lo, hi, (a, b) {
        lo = a;
        hi = b;
      });
    }
    if (lo >= hi) return const SizedBox();

    const gridLines = 5;
    const padFrac = 0.06;

    return LayoutBuilder(builder: (context, constraints) {
      final h = constraints.maxHeight;
      final pad = h * padFrac;
      final chartH = h - 2 * pad;

      return Stack(
        children: [
          for (var i = 0; i <= gridLines; i++)
            Positioned(
              top: pad + chartH * i / gridLines - 8,
              right: 2,
              child: Container(
                padding: const EdgeInsets.all(2),
                decoration: BoxDecoration(
                  color: const Color(0x801A1A2E),
                  borderRadius: BorderRadius.circular(2),
                ),
                child: Text(
                  _formatPrice(hi - (hi - lo) * i / gridLines),
                  style: const TextStyle(
                    color: Color(0x73FFFFFF),
                    fontSize: 9,
                  ),
                ),
              ),
            ),
        ],
      );
    });
  }

  void _expandRange(
    List<double>? values,
    int start,
    int end,
    double curLo,
    double curHi,
    void Function(double lo, double hi) apply,
  ) {
    if (values == null) return;
    double lo = curLo, hi = curHi;
    for (var i = start; i < end && i < values.length; i++) {
      final v = values[i];
      if (v.isNaN) continue;
      if (v < lo) lo = v;
      if (v > hi) hi = v;
    }
    apply(lo, hi);
  }

  static String _formatPrice(double price) => formatPrice(price);
}

/// Widget-based X-axis time labels adaptive to the timeframe.
///
/// Avoids overlapping by spacing labels dynamically based on candle width.
class XAxisLabels extends StatelessWidget {
  final List<CandleDto> candles;
  final int startIndex;
  final int endIndex;
  final double candleWidth;
  final double scrollPixelOffset;
  final String timeframe;

  /// Number of extra candle-width slots beyond [candles.length] for future
  /// event projection.  When > 0, labels are generated for those slots too.
  final int futureSlots;

  /// Duration of one candle — used to compute projected timestamps.
  final Duration? candleDuration;

  const XAxisLabels({
    Key? key,
    required this.candles,
    required this.startIndex,
    required this.endIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
    required this.timeframe,
    this.futureSlots = 0,
    this.candleDuration,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    if (candles.isEmpty || startIndex >= endIndex) return const SizedBox();

    // Determine label spacing: at least 70px between labels.
    final candlesPerLabel = math.max(1, (70 / candleWidth).ceil());

    // Total slots including future projection.
    final totalSlots = candles.length + futureSlots;
    final lastTs = candles.last.timestamp;
    final dur = candleDuration;

    return LayoutBuilder(builder: (context, constraints) {
      final labels = <Widget>[];

      for (var i = startIndex; i < endIndex && i < totalSlots;
          i += candlesPerLabel) {
        final cx = (i - startIndex) * candleWidth +
            candleWidth / 2 -
            scrollPixelOffset;
        if (cx < -30 || cx > constraints.maxWidth + 30) continue;

        // Compute timestamp: real candle or projected future.
        final DateTime ts;
        if (i < candles.length) {
          ts = candles[i].timestamp;
        } else if (dur != null) {
          ts = lastTs.add(dur * (i - candles.length + 1));
        } else {
          continue;
        }

        labels.add(Positioned(
          left: cx - 25,
          top: 2,
          child: SizedBox(
            width: 50,
            child: Text(
              _formatTimestamp(ts),
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: Color(0x73FFFFFF),
                fontSize: 9,
              ),
            ),
          ),
        ));
      }

      return Stack(clipBehavior: Clip.none, children: labels);
    });
  }

  String _formatTimestamp(DateTime ts) {
    final h = ts.hour.toString().padLeft(2, '0');
    final m = ts.minute.toString().padLeft(2, '0');
    final mon = ts.month.toString().padLeft(2, '0');
    final d = ts.day.toString().padLeft(2, '0');

    // For daily timeframes show date, otherwise show time.
    if (timeframe == '1d') return '$mon/$d';
    if (timeframe == '4h' || timeframe == '1h') return '$mon/$d $h:$m';
    return '$h:$m';
  }
}
