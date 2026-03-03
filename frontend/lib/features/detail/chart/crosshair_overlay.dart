import 'package:flutter/material.dart';

import '../../candles/api/candle_response.dart';

/// Immutable snapshot of crosshair state passed to the overlay.
class CrosshairState {
  /// Snapped candle index (absolute, into the full candles list).
  /// May exceed candles.length when in the future zone.
  final int candleIndex;

  /// Touch X in chart-local coords (already snapped to candle center).
  final double x;

  /// Raw touch Y in chart-local coords (price panel).
  final double touchY;

  /// The candle data. In future zone, this is the last real candle
  /// (used as a placeholder; OHLC won't be displayed).
  final CandleDto candle;

  /// Indicator values at [candleIndex], or null/NaN.
  final double? emaFast;
  final double? emaSlow;
  final double? rsi;
  final double? atr;

  /// True when the crosshair is in the future projection zone
  /// (beyond the last candle). Price/OHLC info is not shown.
  final bool isFutureZone;

  /// Projected timestamp for future zone crosshair, or null.
  final DateTime? futureTimestamp;

  const CrosshairState({
    required this.candleIndex,
    required this.x,
    required this.touchY,
    required this.candle,
    this.emaFast,
    this.emaSlow,
    this.rsi,
    this.atr,
    this.isFutureZone = false,
    this.futureTimestamp,
  });
}

/// Overlay that paints crosshair lines, axis highlights, and an OHLC tooltip.
///
/// This widget is meant to be placed as a `Positioned.fill` child inside the
/// chart Stack, above all painters but below gesture detection.
class CrosshairOverlay extends StatelessWidget {
  final CrosshairState state;
  final String symbol;
  final String timeframe;

  /// Layout heights for each section.
  final double priceHeight;
  final double volumeHeight;
  final double oscillatorHeight;
  final double chartWidth;

  /// Price Y-axis range for the visible window (needed to draw the
  /// horizontal line and the Y-axis price label).
  final double priceLo;
  final double priceHi;

  /// Config periods for tooltip labels.
  final int? rsiPeriod;
  final int? atrPeriod;
  final int? emaFastPeriod;
  final int? emaSlowPeriod;

  const CrosshairOverlay({
    Key? key,
    required this.state,
    required this.symbol,
    required this.timeframe,
    required this.priceHeight,
    required this.volumeHeight,
    required this.oscillatorHeight,
    required this.chartWidth,
    required this.priceLo,
    required this.priceHi,
    this.rsiPeriod,
    this.atrPeriod,
    this.emaFastPeriod,
    this.emaSlowPeriod,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final totalH = priceHeight + volumeHeight + oscillatorHeight;
    return SizedBox(
      width: chartWidth,
      height: totalH,
      child: CustomPaint(
        painter: _CrosshairPainter(
          state: state,
          priceHeight: priceHeight,
          volumeHeight: volumeHeight,
          oscillatorHeight: oscillatorHeight,
          chartWidth: chartWidth,
          priceLo: priceLo,
          priceHi: priceHi,
        ),
        child: Stack(children: [
          // ── Tooltip ──
          if (state.isFutureZone)
            _buildFutureTooltip(context)
          else
            _buildTooltip(context),
          // ── Y-axis price tag (hidden in future zone) ──
          if (!state.isFutureZone) _buildYAxisTag(),
          // ── X-axis time tag ──
          _buildXAxisTag(),
        ]),
      ),
    );
  }

  // ── Tooltip ──

  /// Minimal tooltip for the future zone — just timestamp, no OHLC.
  Widget _buildFutureTooltip(BuildContext context) {
    final ts = state.futureTimestamp ?? state.candle.timestamp;
    final lines = <_TooltipLine>[
      _TooltipLine(symbol, isBold: true),
      _TooltipLine(_formatTimestamp(ts)),
      _TooltipLine(''),
      const _TooltipLine('Future zone', color: Color(0x88FFFFFF)),
      const _TooltipLine('No price data'),
    ];

    const tooltipW = 150.0;
    const lineH = 14.0;
    final tooltipH = lines.length * lineH + 16;

    double tx = state.x + 16;
    if (tx + tooltipW > chartWidth) tx = state.x - tooltipW - 16;
    tx = tx.clamp(4.0, chartWidth - tooltipW - 4);

    final totalH = priceHeight + volumeHeight + oscillatorHeight;
    double ty = state.touchY - tooltipH / 2;
    ty = ty.clamp(4.0, totalH - tooltipH - 4);

    return Positioned(
      left: tx,
      top: ty,
      child: IgnorePointer(
        child: Container(
          width: tooltipW,
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
          decoration: BoxDecoration(
            color: const Color(0xE6121224),
            borderRadius: BorderRadius.circular(6),
            border: Border.all(color: const Color(0x44FFFFFF), width: 0.5),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final line in lines)
                line.text.isEmpty
                    ? const SizedBox(height: 4)
                    : Text(
                        line.text,
                        style: TextStyle(
                          color: line.color ?? Colors.white70,
                          fontSize: 10,
                          fontWeight:
                              line.isBold ? FontWeight.w700 : FontWeight.normal,
                          height: 1.35,
                        ),
                      ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildTooltip(BuildContext context) {
    final c = state.candle;
    final lines = <_TooltipLine>[
      _TooltipLine(symbol, isBold: true),
      _TooltipLine(_formatTimestamp(c.timestamp)),
      _TooltipLine(''),
      _TooltipLine('O: ${_fmtPrice(c.open)}'),
      _TooltipLine('H: ${_fmtPrice(c.high)}'),
      _TooltipLine('L: ${_fmtPrice(c.low)}'),
      _TooltipLine('C: ${_fmtPrice(c.close)}'),
      _TooltipLine('Vol: ${_fmtVol(c.volume)}'),
    ];

    if (state.emaFast != null && !state.emaFast!.isNaN) {
      lines.add(_TooltipLine(
        'EMA(${emaFastPeriod ?? '?'}): ${_fmtPrice(state.emaFast!)}',
        color: const Color(0xFF42A5F5),
      ));
    }
    if (state.emaSlow != null && !state.emaSlow!.isNaN) {
      lines.add(_TooltipLine(
        'EMA(${emaSlowPeriod ?? '?'}): ${_fmtPrice(state.emaSlow!)}',
        color: const Color(0xFFFFAB40),
      ));
    }
    if (state.rsi != null && !state.rsi!.isNaN) {
      lines.add(_TooltipLine(
        'RSI(${rsiPeriod ?? 14}): ${state.rsi!.toStringAsFixed(1)}',
        color: const Color(0xFFAB47BC),
      ));
    }
    if (state.atr != null && !state.atr!.isNaN) {
      lines.add(_TooltipLine(
        'ATR(${atrPeriod ?? 14}): ${_fmtPrice(state.atr!)}',
        color: const Color(0xFF26A69A),
      ));
    }

    const tooltipW = 150.0;
    const lineH = 14.0;
    final tooltipH = lines.length * lineH + 16;

    // Position tooltip: avoid screen edges.
    double tx = state.x + 16;
    if (tx + tooltipW > chartWidth) {
      tx = state.x - tooltipW - 16;
    }
    tx = tx.clamp(4.0, chartWidth - tooltipW - 4);

    double ty = state.touchY - tooltipH / 2;
    final totalH = priceHeight + volumeHeight + oscillatorHeight;
    ty = ty.clamp(4.0, totalH - tooltipH - 4);

    return Positioned(
      left: tx,
      top: ty,
      child: IgnorePointer(
        child: Container(
          width: tooltipW,
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
          decoration: BoxDecoration(
            color: const Color(0xE6121224),
            borderRadius: BorderRadius.circular(6),
            border: Border.all(color: const Color(0x44FFFFFF), width: 0.5),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final line in lines)
                line.text.isEmpty
                    ? const SizedBox(height: 4)
                    : Text(
                        line.text,
                        style: TextStyle(
                          color: line.color ?? Colors.white70,
                          fontSize: 10,
                          fontWeight:
                              line.isBold ? FontWeight.w700 : FontWeight.normal,
                          height: 1.35,
                        ),
                      ),
            ],
          ),
        ),
      ),
    );
  }

  // ── Y-axis price tag ──

  Widget _buildYAxisTag() {
    final range = priceHi - priceLo;
    if (range <= 0) return const SizedBox();

    const padFrac = 0.06;
    final pad = priceHeight * padFrac;
    final chartH = priceHeight - 2 * pad;
    // Invert the toY formula: touchY = pad + chartH * (1 - (price - lo) / range)
    final price = priceLo + range * (1 - (state.touchY - pad) / chartH);
    final clampedY = state.touchY.clamp(pad, pad + chartH);

    return Positioned(
      left: chartWidth - 2, // straddle the Y-axis boundary
      top: clampedY - 9,
      child: IgnorePointer(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
          decoration: BoxDecoration(
            color: const Color(0xCC00E5FF),
            borderRadius: BorderRadius.circular(3),
          ),
          child: Text(
            _fmtPrice(price),
            style: const TextStyle(
              color: Colors.black,
              fontSize: 9,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
      ),
    );
  }

  // ── X-axis time tag ──

  Widget _buildXAxisTag() {
    final totalH = priceHeight + volumeHeight + oscillatorHeight;
    final ts = state.isFutureZone
        ? (state.futureTimestamp ?? state.candle.timestamp)
        : state.candle.timestamp;
    final label = _formatTimeShort(ts);
    return Positioned(
      left: (state.x - 30).clamp(0.0, chartWidth - 60),
      top: totalH - 2, // just below the oscillator / at the x-axis line
      child: IgnorePointer(
        child: Container(
          width: 60,
          alignment: Alignment.center,
          padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
          decoration: BoxDecoration(
            color: const Color(0xCC00E5FF),
            borderRadius: BorderRadius.circular(3),
          ),
          child: Text(
            label,
            style: const TextStyle(
              color: Colors.black,
              fontSize: 8,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
      ),
    );
  }

  // ── Helpers ──

  static String _fmtPrice(double price) {
    if (price >= 1000) return price.toStringAsFixed(0);
    if (price >= 1) return price.toStringAsFixed(2);
    if (price >= 0.01) return price.toStringAsFixed(4);
    return price.toStringAsFixed(6);
  }

  static String _fmtVol(double vol) {
    if (vol >= 1e9) return '${(vol / 1e9).toStringAsFixed(1)}B';
    if (vol >= 1e6) return '${(vol / 1e6).toStringAsFixed(1)}M';
    if (vol >= 1e3) return '${(vol / 1e3).toStringAsFixed(1)}K';
    return vol.toStringAsFixed(0);
  }

  static String _formatTimestamp(DateTime ts) {
    final utc = ts.toUtc();
    final local = ts.toLocal();
    final months = [
      '', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'
    ];
    final datePart =
        '${utc.day.toString().padLeft(2, '0')} ${months[utc.month]} ${utc.year}';
    final utcTime =
        '${utc.hour.toString().padLeft(2, '0')}:${utc.minute.toString().padLeft(2, '0')} UTC';
    final localTime =
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')} Local';
    return '$datePart  $utcTime\n$localTime';
  }

  static String _formatTimeShort(DateTime ts) {
    final utc = ts.toUtc();
    final m = utc.month.toString().padLeft(2, '0');
    final d = utc.day.toString().padLeft(2, '0');
    final h = utc.hour.toString().padLeft(2, '0');
    final min = utc.minute.toString().padLeft(2, '0');
    return '$m/$d $h:$min';
  }
}

// ── Painter ──

class _CrosshairPainter extends CustomPainter {
  final CrosshairState state;
  final double priceHeight;
  final double volumeHeight;
  final double oscillatorHeight;
  final double chartWidth;
  final double priceLo;
  final double priceHi;

  _CrosshairPainter({
    required this.state,
    required this.priceHeight,
    required this.volumeHeight,
    required this.oscillatorHeight,
    required this.chartWidth,
    required this.priceLo,
    required this.priceHi,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final totalH = priceHeight + volumeHeight + oscillatorHeight;
    final linePaint = Paint()
      ..color = const Color(0x88FFFFFF)
      ..strokeWidth = 0.5;

    // ── Vertical line (snapped to candle center, full height) ──
    canvas.drawLine(
      Offset(state.x, 0),
      Offset(state.x, totalH),
      linePaint,
    );

    // ── Horizontal line (at touch Y, full width, only in price area) ──
    // Hidden in the future zone where there is no price reference.
    if (!state.isFutureZone) {
      final clampedY = state.touchY.clamp(0.0, priceHeight);
      canvas.drawLine(
        Offset(0, clampedY),
        Offset(chartWidth, clampedY),
        linePaint,
      );
    }

    // ── Volume highlight ──
    // Subtly brighten the volume bar under the selected candle.
    // Skipped in future zone where no volume bars exist.
    if (!state.isFutureZone) {
      final volTop = priceHeight;
      final volBot = priceHeight + volumeHeight;
      final halfCW = 6.0; // approximate half-candle visual width
      canvas.drawRect(
        Rect.fromLTRB(
          state.x - halfCW,
          volTop,
          state.x + halfCW,
          volBot,
        ),
        Paint()..color = const Color(0x18FFFFFF),
      );
    }

    // ── RSI / ATR dot markers on oscillator ──
    if (oscillatorHeight > 0 && !state.isFutureZone) {
      final oscTop = priceHeight + volumeHeight;
      final oscH = oscillatorHeight;

      // RSI dot (fixed 0-100 scale)
      if (state.rsi != null && !state.rsi!.isNaN) {
        final rsiY = oscTop + oscH * (1 - state.rsi! / 100);
        canvas.drawCircle(
          Offset(state.x, rsiY),
          3,
          Paint()..color = const Color(0xFFAB47BC),
        );
      }

      // ATR dot — use price-relative scaling
      if (state.atr != null && !state.atr!.isNaN) {
        // ATR is drawn at a rough position; painter uses full-data scale
        // so we just put a dot at a reasonable Y. We mark it at the bottom
        // third of the oscillator area as a visual cue.
        final atrY = oscTop + oscH * 0.7;
        canvas.drawCircle(
          Offset(state.x, atrY),
          3,
          Paint()..color = const Color(0xFF26A69A),
        );
      }
    }
  }

  @override
  bool shouldRepaint(covariant _CrosshairPainter old) =>
      old.state.candleIndex != state.candleIndex ||
      old.state.x != state.x ||
      old.state.touchY != state.touchY;
}

class _TooltipLine {
  final String text;
  final bool isBold;
  final Color? color;
  const _TooltipLine(this.text, {this.isBold = false, this.color});
}
