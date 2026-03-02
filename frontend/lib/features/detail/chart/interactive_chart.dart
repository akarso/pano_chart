import 'dart:math' as math;
import 'package:flutter/material.dart';

import '../../candles/api/candle_response.dart';
import '../../events/chart_event_overlay.dart';
import '../../events/chart_event_overlay_painter.dart';
import '../../events/event_marker_builder.dart';
import '../../events/events_view_model.dart';
import 'axis_layer.dart';
import 'candle_painter.dart';
import 'chart_config.dart';
import 'crosshair_overlay.dart';
import 'indicators.dart';
import 'oscillator_painter.dart';
import 'volume_painter.dart';

/// Interactive candlestick chart with pinch-to-zoom, panning,
/// and layered technical indicators (EMA clouds, RSI, ATR).
///
/// This is the PR-024 chart engine.
class InteractiveChart extends StatefulWidget {
  final CandleSeriesResponse series;
  final ChartIndicatorConfig config;
  final ValueChanged<ChartIndicatorConfig>? onConfigChanged;
  final EventsViewModel? eventsViewModel;
  final ValueChanged<String>? onNavigateToEvent;

  /// Total height of the combined chart.
  final double height;

  const InteractiveChart({
    Key? key,
    required this.series,
    required this.config,
    this.onConfigChanged,
    this.eventsViewModel,
    this.onNavigateToEvent,
    this.height = 360,
  }) : super(key: key);

  @override
  State<InteractiveChart> createState() => _InteractiveChartState();
}

class _InteractiveChartState extends State<InteractiveChart> {
  // ── zoom / pan state ──
  double _candleWidth = 10.0;
  double _scrollOffset = 0.0; // in candle units (fractional)
  double? _prevScale;
  double? _prevFocalX;
  double _prevScrollOffset = 0;

  bool _didInitialScroll = false;

  // ── cached indicators ──
  List<double>? _emaFast;
  List<double>? _emaSlow;
  List<double>? _rsi;
  List<double>? _atr;

  // ── limits ──
  static const double _minCandleWidth = 2.0;
  static const double _maxCandleWidth = 40.0;

  // ── crosshair ──
  CrosshairState? _crosshair;

  @override
  void initState() {
    super.initState();
    _recomputeIndicators();
  }

  @override
  void didUpdateWidget(covariant InteractiveChart old) {
    super.didUpdateWidget(old);
    if (!identical(old.series, widget.series) || old.config != widget.config) {
      _recomputeIndicators();
    }
  }

  void _recomputeIndicators() {
    final closes = widget.series.candles.map((c) => c.close).toList();
    final cfg = widget.config;

    _emaFast = cfg.showEmaFast ? computeEma(closes, cfg.emaFastPeriod) : null;
    _emaSlow = cfg.showEmaSlow ? computeEma(closes, cfg.emaSlowPeriod) : null;
    _rsi = cfg.showRsi ? computeRsi(closes, cfg.rsiPeriod) : null;

    if (cfg.showAtr) {
      final highs = widget.series.candles.map((c) => c.high).toList();
      final lows = widget.series.candles.map((c) => c.low).toList();
      _atr = computeAtr(highs, lows, closes, cfg.atrPeriod);
    } else {
      _atr = null;
    }
  }

  // ── visible range ──

  int _visibleStart(double viewportWidth) {
    return _scrollOffset.floor().clamp(0, widget.series.candles.length - 1);
  }

  int _visibleEnd(double viewportWidth) {
    final count = (viewportWidth / _candleWidth).ceil() + 2;
    return (_scrollOffset.floor() + count)
        .clamp(0, widget.series.candles.length);
  }

  double _scrollPixelOffset(double viewportWidth) {
    return (_scrollOffset - _scrollOffset.floor()) * _candleWidth;
  }

  void _clampScroll(double viewportWidth) {
    final maxScroll = widget.series.candles.length -
        (viewportWidth / _candleWidth).floor();
    _scrollOffset = _scrollOffset.clamp(0.0, math.max(0.0, maxScroll.toDouble()));
  }

  // ── gesture handling ──

  void _onScaleStart(ScaleStartDetails d) {
    _prevScale = _candleWidth;
    _prevFocalX = d.localFocalPoint.dx;
    _prevScrollOffset = _scrollOffset;
  }

  void _onScaleUpdate(ScaleUpdateDetails d, double viewportWidth) {
    setState(() {
      if (d.pointerCount >= 2) {
        // Pinch-to-zoom — scale candleWidth around focal point.
        // Also dismiss crosshair immediately.
        _crosshair = null;
        final newWidth = (_prevScale! * d.scale)
            .clamp(_minCandleWidth, _maxCandleWidth);

        // Keep the candle under the focal point stationary.
        final focalCandle =
            _prevScrollOffset + _prevFocalX! / _prevScale!;
        _candleWidth = newWidth;
        _scrollOffset = focalCandle - d.localFocalPoint.dx / _candleWidth;
      } else {
        // Single-finger pan.
        final dx = d.localFocalPoint.dx - _prevFocalX!;
        _scrollOffset = _prevScrollOffset - dx / _candleWidth;
      }
      _clampScroll(viewportWidth);
    });
  }

  // ── build ──

  @override
  Widget build(BuildContext context) {
    final candles = widget.series.candles;
    if (candles.isEmpty) {
      return SizedBox(
        height: widget.height,
        child: const Center(
          child: Text('No data', style: TextStyle(color: Colors.white38)),
        ),
      );
    }

    // Compute height splits.
    final hasOscillator = _rsi != null || _atr != null;
    final priceH = hasOscillator ? widget.height * 0.62 : widget.height * 0.78;
    final volumeH = hasOscillator ? widget.height * 0.14 : widget.height * 0.22;
    final oscH = hasOscillator ? widget.height * 0.22 : 0.0;
    final xAxisH = 18.0;
    final totalH = priceH + volumeH + oscH + xAxisH;

    return SizedBox(
      height: totalH,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        child: Container(
          color: const Color(0xFF1A1A2E),
          child: LayoutBuilder(builder: (context, constraints) {
            final vw = constraints.maxWidth;
            // On first build, scroll to show last candles.
            if (!_didInitialScroll) {
              _didInitialScroll = true;
              final visibleCount = (vw / _candleWidth).floor();
              if (candles.length > visibleCount) {
                _scrollOffset =
                    (candles.length - visibleCount).toDouble();
              }
            }
            _clampScroll(vw);

            final start = _visibleStart(vw);
            final end = _visibleEnd(vw);
            final pixOff = _scrollPixelOffset(vw);

            // Y-axis width reservation.
            const yAxisW = 44.0;
            final chartW = vw - yAxisW;

            return GestureDetector(
              onScaleStart: _onScaleStart,
              onScaleUpdate: (d) => _onScaleUpdate(d, chartW),
              onLongPressStart: (d) => _onCrosshairStart(d.localPosition, chartW, start, priceH),
              onLongPressMoveUpdate: (d) => _onCrosshairUpdate(d.localPosition, chartW, start, priceH),
              onLongPressEnd: (_) => _onCrosshairEnd(),
              child: Stack(
                children: [
                  // ── Price chart layer ──
                  Positioned(
                    left: 0,
                    top: 0,
                    width: chartW,
                    height: priceH,
                    child: RepaintBoundary(
                      child: CustomPaint(
                        size: Size(chartW, priceH),
                        painter: CandlePainter(
                          candles: candles,
                          startIndex: start,
                          endIndex: end,
                          candleWidth: _candleWidth,
                          scrollPixelOffset: pixOff,
                          emaFast: _emaFast,
                          emaSlow: _emaSlow,
                        ),
                      ),
                    ),
                  ),

                  // ── Event overlay ──
                  if (widget.eventsViewModel != null &&
                      widget.eventsViewModel!.state.showEvents)
                    Positioned(
                      left: 0,
                      top: 0,
                      width: chartW,
                      height: priceH,
                      child: ChartEventOverlay(
                        markers: _buildEventMarkers(chartW),
                        onNavigateToEvent: widget.onNavigateToEvent,
                      ),
                    ),

                  // ── Y-axis labels ──
                  Positioned(
                    right: 0,
                    top: 0,
                    width: yAxisW,
                    height: priceH,
                    child: IgnorePointer(
                      child: YAxisLabels(
                        candles: candles,
                        startIndex: start,
                        endIndex: end,
                        emaFast: _emaFast,
                        emaSlow: _emaSlow,
                      ),
                    ),
                  ),

                  // ── Volume layer ──
                  Positioned(
                    left: 0,
                    top: priceH,
                    width: chartW,
                    height: volumeH,
                    child: RepaintBoundary(
                      child: CustomPaint(
                        size: Size(chartW, volumeH),
                        painter: VolumePainter(
                          candles: candles,
                          startIndex: start,
                          endIndex: end,
                          candleWidth: _candleWidth,
                          scrollPixelOffset: pixOff,
                        ),
                      ),
                    ),
                  ),

                  // ── Oscillator layer (RSI + ATR) ──
                  if (hasOscillator)
                    Positioned(
                      left: 0,
                      top: priceH + volumeH,
                      width: chartW,
                      height: oscH,
                      child: RepaintBoundary(
                        child: Container(
                          decoration: const BoxDecoration(
                            border: Border(
                              top: BorderSide(color: Color(0x33FFFFFF), width: 0.5),
                            ),
                          ),
                          child: CustomPaint(
                            size: Size(chartW, oscH),
                            painter: OscillatorPainter(
                              rsi: _rsi,
                              atr: _atr,
                              startIndex: start,
                              endIndex: end,
                              candleWidth: _candleWidth,
                              scrollPixelOffset: pixOff,
                            ),
                          ),
                        ),
                      ),
                    ),

                  // ── X-axis time labels ──
                  Positioned(
                    left: 0,
                    top: priceH + volumeH + oscH,
                    width: chartW,
                    height: xAxisH,
                    child: IgnorePointer(
                      child: XAxisLabels(
                        candles: candles,
                        startIndex: start,
                        endIndex: end,
                        candleWidth: _candleWidth,
                        scrollPixelOffset: pixOff,
                        timeframe: widget.series.timeframe,
                      ),
                    ),
                  ),

                  // ── Crosshair overlay ──
                  if (_crosshair != null)
                    Positioned(
                      left: 0,
                      top: 0,
                      width: chartW,
                      height: priceH + volumeH + oscH,
                      child: IgnorePointer(
                        child: CrosshairOverlay(
                          state: _crosshair!,
                          symbol: widget.series.symbol,
                          timeframe: widget.series.timeframe,
                          priceHeight: priceH,
                          volumeHeight: volumeH,
                          oscillatorHeight: oscH,
                          chartWidth: chartW,
                          priceLo: _visiblePriceLo(start, end),
                          priceHi: _visiblePriceHi(start, end),
                          rsiPeriod: widget.config.showRsi ? widget.config.rsiPeriod : null,
                          atrPeriod: widget.config.showAtr ? widget.config.atrPeriod : null,
                          emaFastPeriod: widget.config.showEmaFast ? widget.config.emaFastPeriod : null,
                          emaSlowPeriod: widget.config.showEmaSlow ? widget.config.emaSlowPeriod : null,
                        ),
                      ),
                    ),

                  // ── Oscillator labels ──
                  if (_rsi != null)
                    Positioned(
                      right: yAxisW + 4,
                      top: priceH + volumeH + 2,
                      child: const Text(
                        'RSI',
                        style: TextStyle(
                          color: Color(0x88AB47BC),
                          fontSize: 9,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                  if (_atr != null)
                    Positioned(
                      right: yAxisW + 4,
                      top: priceH + volumeH + 14,
                      child: const Text(
                        'ATR',
                        style: TextStyle(
                          color: Color(0x8826A69A),
                          fontSize: 9,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                ],
              ),
            );
          }),
        ),
      ),
    );
  }

  List<EventMarker> _buildEventMarkers(double chartWidth) {
    final evm = widget.eventsViewModel;
    if (evm == null) return [];

    // Build markers for the VISIBLE portion only.
    final visibleCandles = widget.series.candles.sublist(
      _visibleStart(chartWidth),
      math.min(_visibleEnd(chartWidth), widget.series.candles.length),
    );
    if (visibleCandles.isEmpty) return [];

    // Create a temporary CandleSeriesResponse for the visible portion.
    final visibleSeries = CandleSeriesResponse(
      symbol: widget.series.symbol,
      timeframe: widget.series.timeframe,
      candles: visibleCandles,
    );

    return buildEventMarkers(
      series: visibleSeries,
      events: evm.state.filteredEvents,
      filterLevel: evm.state.filterLevel,
      candleWidth: _candleWidth,
    );
  }

  // ── Crosshair ──

  void _onCrosshairStart(Offset local, double chartW, int visStart, double priceH) {
    _updateCrosshair(local, chartW, visStart, priceH);
  }

  void _onCrosshairUpdate(Offset local, double chartW, int visStart, double priceH) {
    _updateCrosshair(local, chartW, visStart, priceH);
  }

  void _onCrosshairEnd() {
    setState(() => _crosshair = null);
  }

  void _updateCrosshair(Offset local, double chartW, int visStart, double priceH) {
    final candles = widget.series.candles;
    if (candles.isEmpty) return;

    final pixOff = _scrollPixelOffset(chartW);
    // Determine which candle the touch is nearest to.
    final rawIdx = ((local.dx + pixOff) / _candleWidth).floor();
    final absIdx = (visStart + rawIdx).clamp(0, candles.length - 1);

    // Snap X to candle center.
    final snappedX =
        (absIdx - visStart) * _candleWidth + _candleWidth / 2 - pixOff;

    final c = candles[absIdx];

    double? emaF, emaS, rsiV, atrV;
    if (_emaFast != null && absIdx < _emaFast!.length) emaF = _emaFast![absIdx];
    if (_emaSlow != null && absIdx < _emaSlow!.length) emaS = _emaSlow![absIdx];
    if (_rsi != null && absIdx < _rsi!.length) rsiV = _rsi![absIdx];
    if (_atr != null && absIdx < _atr!.length) atrV = _atr![absIdx];

    setState(() {
      _crosshair = CrosshairState(
        candleIndex: absIdx,
        x: snappedX,
        touchY: local.dy.clamp(0, priceH),
        candle: c,
        emaFast: emaF,
        emaSlow: emaS,
        rsi: rsiV,
        atr: atrV,
      );
    });
  }

  // ── Visible price range (shared with crosshair Y-tag) ──

  double _visiblePriceLo(int start, int end) {
    double lo = double.infinity;
    for (var i = start; i < end && i < widget.series.candles.length; i++) {
      if (widget.series.candles[i].low < lo) lo = widget.series.candles[i].low;
    }
    _expandForCrosshair(_emaFast, start, end, (v) { if (v < lo) lo = v; });
    _expandForCrosshair(_emaSlow, start, end, (v) { if (v < lo) lo = v; });
    return lo;
  }

  double _visiblePriceHi(int start, int end) {
    double hi = double.negativeInfinity;
    for (var i = start; i < end && i < widget.series.candles.length; i++) {
      if (widget.series.candles[i].high > hi) hi = widget.series.candles[i].high;
    }
    _expandForCrosshair(_emaFast, start, end, (v) { if (v > hi) hi = v; });
    _expandForCrosshair(_emaSlow, start, end, (v) { if (v > hi) hi = v; });
    return hi;
  }

  void _expandForCrosshair(List<double>? vals, int start, int end, void Function(double) apply) {
    if (vals == null) return;
    for (var i = start; i < end && i < vals.length; i++) {
      if (!vals[i].isNaN) apply(vals[i]);
    }
  }
}
