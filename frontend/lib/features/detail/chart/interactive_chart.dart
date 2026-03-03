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

  /// Leading candles used only for indicator warmup (not scrollable).
  final int warmupCount;

  /// Number of candles to fill the viewport width initially.
  final int initialVisibleCount;

  const InteractiveChart({
    Key? key,
    required this.series,
    required this.config,
    this.onConfigChanged,
    this.eventsViewModel,
    this.onNavigateToEvent,
    this.height = 360,
    this.warmupCount = 0,
    this.initialVisibleCount = 30,
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

  // ── vertical scale ──
  double _priceScaleY = 1.0; // 1.0 = auto-fit, >1 = zoomed in

  bool _didInitialScroll = false;

  // ── cached indicators ──
  List<double>? _emaFast;
  List<double>? _emaSlow;
  List<double>? _rsi;
  List<double>? _atr;

  // ── limits ──
  static const double _minCandleWidth = 2.0;
  static const double _maxCandleWidth = 40.0;

  // ── axis overlay sizing ──
  static const double _yAxisW = 44.0;
  static const double _yAxisDragW = 50.0;
  static const double _xAxisDragH = 24.0;

  // ── gesture zone tracking ──
  _GestureZone _activeGesture = _GestureZone.chart;
  double _yAxisDragStartY = 0;
  double _yAxisDragStartScale = 1.0;
  double _xAxisDragStartX = 0;
  double _xAxisDragStartWidth = 10.0;
  double _xAxisDragStartScroll = 0;

  // ── layout cache (set in build before gesture callbacks fire) ──
  double _layoutVw = 0;
  double _layoutTotalH = 0;
  double _layoutChartBottom = 0;

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

  /// Effective warmup clamped to available candle count.
  int get _effectiveWarmup =>
      math.min(widget.warmupCount, widget.series.candles.length - 1);

  void _clampScroll(double viewportWidth) {
    final maxScroll = widget.series.candles.length -
        (viewportWidth / _candleWidth).floor();
    final minScroll = _effectiveWarmup.toDouble();
    _scrollOffset = _scrollOffset.clamp(
      minScroll,
      math.max(minScroll, maxScroll.toDouble()),
    );
  }

  // ── gesture handling ──

  void _onScaleStart(ScaleStartDetails d) {
    final pos = d.localFocalPoint;

    // Y-axis drag zone: right strip within chart content area.
    if (d.pointerCount == 1 &&
        pos.dx > _layoutVw - _yAxisDragW &&
        pos.dy < _layoutChartBottom) {
      _activeGesture = _GestureZone.yAxis;
      _yAxisDragStartY = pos.dy;
      _yAxisDragStartScale = _priceScaleY;
      return;
    }

    // X-axis drag zone: bottom strip.
    if (d.pointerCount == 1 &&
        pos.dy > _layoutTotalH - _xAxisDragH) {
      _activeGesture = _GestureZone.xAxis;
      _xAxisDragStartX = pos.dx;
      _xAxisDragStartWidth = _candleWidth;
      _xAxisDragStartScroll = _scrollOffset;
      return;
    }

    // Default: chart pan / pinch.
    _activeGesture = _GestureZone.chart;
    _prevScale = _candleWidth;
    _prevFocalX = d.localFocalPoint.dx;
    _prevScrollOffset = _scrollOffset;
  }

  void _onScaleUpdate(ScaleUpdateDetails d, double viewportWidth) {
    setState(() {
      switch (_activeGesture) {
        case _GestureZone.yAxis:
          if (d.pointerCount == 1) {
            final delta = d.localFocalPoint.dy - _yAxisDragStartY;
            _priceScaleY = (_yAxisDragStartScale * math.exp(delta * 0.008))
                .clamp(0.3, 10.0);
          }
          break;
        case _GestureZone.xAxis:
          if (d.pointerCount == 1) {
            final delta = d.localFocalPoint.dx - _xAxisDragStartX;
            final newWidth =
                (_xAxisDragStartWidth * math.exp(delta * 0.008))
                    .clamp(_minCandleWidth, _maxCandleWidth);
            final viewCenter = _xAxisDragStartScroll +
                viewportWidth / (2 * _xAxisDragStartWidth);
            _candleWidth = newWidth;
            _scrollOffset =
                viewCenter - viewportWidth / (2 * _candleWidth);
            _clampScroll(viewportWidth);
          }
          break;
        case _GestureZone.chart:
          if (d.pointerCount >= 2) {
            _crosshair = null;
            // Horizontal pinch zoom only (vertical pinch disabled).
            final newWidth = (_prevScale! * d.horizontalScale)
                .clamp(_minCandleWidth, _maxCandleWidth);
            final focalCandle =
                _prevScrollOffset + _prevFocalX! / _prevScale!;
            _candleWidth = newWidth;
            _scrollOffset =
                focalCandle - d.localFocalPoint.dx / _candleWidth;
          } else {
            // Single-finger pan.
            final dx = d.localFocalPoint.dx - _prevFocalX!;
            _scrollOffset = _prevScrollOffset - dx / _candleWidth;
          }
          _clampScroll(viewportWidth);
          break;
      }
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
            // Chart renders full width; Y-axis labels overlay on top.
            final chartW = vw;

            // Cache layout dimensions for gesture zone detection.
            _layoutVw = vw;
            _layoutTotalH = priceH + volumeH + oscH + xAxisH;
            _layoutChartBottom = priceH + volumeH + oscH;

            // On first build, set candleWidth to match sparkline ratio
            // and scroll to show the last initialVisibleCount candles.
            if (!_didInitialScroll) {
              _didInitialScroll = true;
              final ivCount = widget.initialVisibleCount.clamp(1, candles.length);
              _candleWidth = (chartW / ivCount).clamp(_minCandleWidth, _maxCandleWidth);
              final visibleCount = (chartW / _candleWidth).floor();
              _scrollOffset = math.max(
                _effectiveWarmup.toDouble(),
                (candles.length - visibleCount).toDouble(),
              );
            }
            _clampScroll(chartW);

            final start = _visibleStart(chartW);
            final end = _visibleEnd(chartW);
            final pixOff = _scrollPixelOffset(chartW);

            // ── Compute scaled price range ──
            double baseLo = double.infinity, baseHi = double.negativeInfinity;
            for (var i = start; i < end && i < candles.length; i++) {
              if (candles[i].low < baseLo) baseLo = candles[i].low;
              if (candles[i].high > baseHi) baseHi = candles[i].high;
            }
            _expandForRange(_emaFast, start, end, (v) {
              if (v < baseLo) baseLo = v;
              if (v > baseHi) baseHi = v;
            });
            _expandForRange(_emaSlow, start, end, (v) {
              if (v < baseLo) baseLo = v;
              if (v > baseHi) baseHi = v;
            });
            final baseRange = (baseHi - baseLo) == 0 ? 1.0 : (baseHi - baseLo);
            final baseCenter = (baseHi + baseLo) / 2;
            final scaledRange = baseRange / _priceScaleY;
            final priceLo = baseCenter - scaledRange / 2;
            final priceHi = baseCenter + scaledRange / 2;

            // Whether user has scrolled to the hard candle limit.
            final atHardLimit = _scrollOffset <= _effectiveWarmup + 0.5;

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
                          priceLo: priceLo,
                          priceHi: priceHi,
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

                  // ── Y-axis labels (overlay with background) ──
                  Positioned(
                    right: 0,
                    top: 0,
                    width: _yAxisW,
                    height: priceH,
                    child: IgnorePointer(
                      child: YAxisLabels(
                        candles: candles,
                        startIndex: start,
                        endIndex: end,
                        emaFast: _emaFast,
                        emaSlow: _emaSlow,
                        priceLo: priceLo,
                        priceHi: priceHi,
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
                          priceLo: priceLo,
                          priceHi: priceHi,
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
                      right: _yAxisW + 4,
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
                      right: _yAxisW + 4,
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

                  // ── Hard candle limit label ──
                  if (atHardLimit)
                    Positioned(
                      left: 2,
                      top: priceH / 2 - 40,
                      child: Transform.rotate(
                        angle: -math.pi / 2,
                        child: Text(
                          'Hard candle limit reached',
                          style: TextStyle(
                            color: Colors.red.withOpacity(0.7),
                            fontSize: 9,
                            fontWeight: FontWeight.w500,
                          ),
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

  // ── Helpers (shared with crosshair, build, etc.) ──

  void _expandForRange(List<double>? vals, int start, int end, void Function(double) apply) {
    if (vals == null) return;
    for (var i = start; i < end && i < vals.length; i++) {
      if (!vals[i].isNaN) apply(vals[i]);
    }
  }
}

/// Which zone of the chart the current gesture started in.
enum _GestureZone { chart, yAxis, xAxis }
