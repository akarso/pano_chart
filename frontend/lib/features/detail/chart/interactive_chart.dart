import 'dart:math' as math;
import 'package:flutter/gestures.dart' show GestureBinding, PointerScrollEvent;
import 'package:flutter/material.dart';

import '../../candles/api/candle_response.dart';
import '../../events/chart_event_overlay.dart';
import '../../events/chart_event_overlay_painter.dart';
import '../../events/event_marker_builder.dart';
import '../../events/events_view_model.dart';
import '../../social/chart/chart_social_overlay.dart';
import '../../social/chart/social_chart_overlay_painter.dart';
import '../../social/chart/social_marker_builder.dart';
import '../../social/social_feed_view_model.dart';
import '../chart_navigation.dart';
import 'axis_layer.dart';
import 'behavior_oscillator_painter.dart';
import 'candle_painter.dart';
import 'chart_config.dart';
import 'crosshair_overlay.dart';
import 'indicators.dart';
import 'oscillator_painter.dart';
import 'volume_painter.dart';
import '../../volatility/volatility_model.dart';
import '../../volatility/volatility_painter.dart';

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
  final SocialFeedViewModel? socialFeedViewModel;
  final VoidCallback? onNavigateToFeed;

  /// Per-candle volatility buckets (aligned 1:1 with series.candles).
  /// Null entries mean no bucket matched that candle.
  final List<VolatilityBucket?>? volatilityAligned;

  /// Total height of the combined chart.
  final double height;

  /// Leading candles used only for indicator warmup (not scrollable).
  final int warmupCount;

  /// Number of candles to fill the viewport width initially.
  final int initialVisibleCount;

  /// Absolute candle index where the reference area (overview sparkline)
  /// starts.  A green line is drawn above the X-axis from this index to
  /// the last candle.  `null` hides the reference line.
  final int? referenceStartIndex;

  const InteractiveChart({
    Key? key,
    required this.series,
    required this.config,
    this.onConfigChanged,
    this.eventsViewModel,
    this.onNavigateToEvent,
    this.socialFeedViewModel,
    this.onNavigateToFeed,
    this.volatilityAligned,
    this.height = 360,
    this.warmupCount = 0,
    this.initialVisibleCount = 30,
    this.referenceStartIndex,
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

  // ── behavioral indicators ──
  BehaviorIndicators? _behavior;

  // ── limits ──
  static const double _minCandleWidth = 2.0;
  static const double _maxCandleWidth = 40.0;

  // ── axis overlay sizing ──
  static const double _yAxisW = 44.0;
  static const double _yAxisDragW = 50.0;

  // ── Y-axis drag state ──
  double _yAxisDragStartY = 0;
  double _yAxisDragStartScale = 1.0;

  // ── X-axis drag state ──
  double _xAxisDragStartX = 0;
  double _xAxisDragStartWidth = 10.0;
  double _xAxisDragStartScroll = 0;

  // ── layout cache (set in build before gesture callbacks fire) ──
  double _layoutVw = 0;
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
    final candles = widget.series.candles;
    final closes = candles.map((c) => c.close).toList();
    final cfg = widget.config;

    _emaFast = cfg.showEmaFast ? computeEma(closes, cfg.emaFastPeriod) : null;
    _emaSlow = cfg.showEmaSlow ? computeEma(closes, cfg.emaSlowPeriod) : null;
    _rsi = cfg.showRsi ? computeRsi(closes, cfg.rsiPeriod) : null;

    final highs = candles.map((c) => c.high).toList();
    final lows = candles.map((c) => c.low).toList();

    if (cfg.showAtr) {
      _atr = computeAtr(highs, lows, closes, cfg.atrPeriod);
    } else {
      _atr = null;
    }

    if (cfg.showBehaviorPanel) {
      final volumes = candles.map((c) => c.volume).toList();
      _behavior =
          computeBehaviorIndicators(closes, highs, lows, volumes, cfg.behaviorWindow);
    } else {
      _behavior = null;
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

  /// Effective warmup clamped so that at least a handful of candles remain
  /// visible.  For very short series (fresh tokens) the warmup is reduced
  /// to zero rather than hiding nearly every candle.
  int get _effectiveWarmup {
    final total = widget.series.candles.length;
    final raw = widget.warmupCount;
    if (raw <= 0 || total <= 1) return 0;
    // Reserve at least 2 candles (or the full series if tiny) for display.
    final minVisible = total < 10 ? total : 10;
    return math.max(0, math.min(raw, total - minVisible));
  }

  void _clampScroll(double viewportWidth) {
    final maxScroll = (widget.series.candles.length + _futureSlots) -
        (viewportWidth / _candleWidth).floor();
    final minScroll = _effectiveWarmup.toDouble();
    _scrollOffset = _scrollOffset.clamp(
      minScroll,
      math.max(minScroll, maxScroll.toDouble()),
    );
  }

  /// Number of extra candle-width slots appended to the right for the
  /// future event projection zone.  Always shows the full projection
  /// window when events are enabled so users know the zone exists.
  int get _futureSlots {
    final evm = widget.eventsViewModel;
    if (evm == null || !evm.state.showEvents) return 0;
    final candles = widget.series.candles;
    if (candles.isEmpty) return 0;

    final tf = widget.series.timeframe;
    final maxSlots = maxProjectionSlots(tf);
    if (maxSlots <= 0) return 0;

    // Always show the full projection window with right padding.
    final yAxisSlots = (_yAxisW / _candleWidth).ceil() + 1;
    final minPad = math.max(yAxisSlots, 3);
    return maxSlots + minPad;
  }

  /// Whether any future event markers exist in the current projection.
  bool get _hasFutureEvents {
    final evm = widget.eventsViewModel;
    if (evm == null || !evm.state.showEvents) return false;
    final candles = widget.series.candles;
    if (candles.isEmpty) return false;

    final lastTs = candles.last.timestamp;
    final tf = widget.series.timeframe;
    final dur = candleDuration(tf);
    final maxSlots = maxProjectionSlots(tf);
    if (maxSlots <= 0 || dur.inMilliseconds <= 0) return false;

    final maxTs = lastTs.add(dur * maxSlots);
    for (final e in evm.state.filteredEvents) {
      if (e.timestamp.isAfter(lastTs) && !e.timestamp.isAfter(maxTs)) {
        return true;
      }
    }
    return false;
  }

  // ── gesture handling (chart area: pan + pinch) ──

  void _onScaleStart(ScaleStartDetails d) {
    _prevScale = _candleWidth;
    _prevFocalX = d.localFocalPoint.dx;
    _prevScrollOffset = _scrollOffset;
  }

  void _onScaleUpdate(ScaleUpdateDetails d, double viewportWidth) {
    setState(() {
      if (d.pointerCount >= 2) {
        _crosshair = null;
        final newWidth = (_prevScale! * d.horizontalScale)
            .clamp(_minCandleWidth, _maxCandleWidth);
        final focalCandle =
            _prevScrollOffset + _prevFocalX! / _prevScale!;
        _candleWidth = newWidth;
        _scrollOffset =
            focalCandle - d.localFocalPoint.dx / _candleWidth;
      } else {
        final dx = d.localFocalPoint.dx - _prevFocalX!;
        _scrollOffset = _prevScrollOffset - dx / _candleWidth;
      }
      _clampScroll(viewportWidth);
    });
  }

  // ── Y-axis drag (vertical price scaling) ──

  void _onYAxisDragStart(DragStartDetails d) {
    _yAxisDragStartY = d.localPosition.dy;
    _yAxisDragStartScale = _priceScaleY;
  }

  void _onYAxisDragUpdate(DragUpdateDetails d) {
    setState(() {
      final delta = d.localPosition.dy - _yAxisDragStartY;
      _priceScaleY =
          (_yAxisDragStartScale * math.exp(delta * 0.008)).clamp(0.3, 10.0);
    });
  }

  void _resetPriceScale() {
    setState(() => _priceScaleY = 1.0);
  }

  // ── X-axis drag (horizontal time scaling) ──

  void _onXAxisDragStart(DragStartDetails d, double viewportWidth) {
    _xAxisDragStartX = d.localPosition.dx;
    _xAxisDragStartWidth = _candleWidth;
    _xAxisDragStartScroll = _scrollOffset;
  }

  void _onXAxisDragUpdate(DragUpdateDetails d, double viewportWidth) {
    setState(() {
      final delta = d.localPosition.dx - _xAxisDragStartX;
      final newWidth =
          (_xAxisDragStartWidth * math.exp(delta * 0.008))
              .clamp(_minCandleWidth, _maxCandleWidth);
      final viewCenter = _xAxisDragStartScroll +
          viewportWidth / (2 * _xAxisDragStartWidth);
      _candleWidth = newWidth;
      _scrollOffset = viewCenter - viewportWidth / (2 * _candleWidth);
      _clampScroll(viewportWidth);
    });
  }

  void _resetTimeScale(double viewportWidth) {
    setState(() {
      final candles = widget.series.candles;
      final ivCount = widget.initialVisibleCount.clamp(1, candles.length);
      _candleWidth =
          (viewportWidth / ivCount).clamp(_minCandleWidth, _maxCandleWidth);
      _clampScroll(viewportWidth);
    });
  }

  // ── pointer scroll (trackpad / mouse wheel) ──

  void _handlePointerScroll(PointerScrollEvent event, double chartW) {
    final pos = event.localPosition;
    setState(() {
      if (pos.dx > _layoutVw - _yAxisDragW &&
          pos.dy < _layoutChartBottom) {
        // Y-axis zone: scroll → vertical price zoom.
        _priceScaleY =
            (_priceScaleY * math.exp(-event.scrollDelta.dy * 0.003))
                .clamp(0.3, 10.0);
      } else if (pos.dy > _layoutChartBottom) {
        // X-axis zone: scroll → horizontal time zoom.
        final newWidth =
            (_candleWidth * math.exp(-event.scrollDelta.dy * 0.003))
                .clamp(_minCandleWidth, _maxCandleWidth);
        final viewCenter = _scrollOffset + chartW / (2 * _candleWidth);
        _candleWidth = newWidth;
        _scrollOffset = viewCenter - chartW / (2 * _candleWidth);
        _clampScroll(chartW);
      } else {
        // Chart area: scroll → horizontal pan.
        _scrollOffset += event.scrollDelta.dy / _candleWidth;
        _clampScroll(chartW);
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
    final hasBehavior = _behavior != null;
    final hasVolatility = widget.config.showVolatility &&
        widget.volatilityAligned != null &&
        widget.volatilityAligned!.isNotEmpty;
    final panelCount = (hasOscillator ? 1 : 0) +
        (hasBehavior ? 1 : 0) +
        (hasVolatility ? 1 : 0);
    late final double priceH;
    late final double volumeH;
    late final double oscH;
    late final double behH;
    late final double volH;
    switch (panelCount) {
      case 0:
        priceH = widget.height * 0.78;
        volumeH = widget.height * 0.22;
        oscH = 0;
        behH = 0;
        volH = 0;
        break;
      case 1:
        priceH = widget.height * 0.58;
        volumeH = widget.height * 0.14;
        oscH = hasOscillator ? widget.height * 0.26 : 0.0;
        behH = hasBehavior ? widget.height * 0.26 : 0.0;
        volH = hasVolatility ? widget.height * 0.26 : 0.0;
        break;
      case 2:
        priceH = widget.height * 0.46;
        volumeH = widget.height * 0.12;
        oscH = hasOscillator ? widget.height * 0.20 : 0.0;
        behH = hasBehavior ? widget.height * 0.20 : 0.0;
        volH = hasVolatility ? widget.height * 0.20 : 0.0;
        break;
      default: // 3 panels
        priceH = widget.height * 0.40;
        volumeH = widget.height * 0.10;
        oscH = hasOscillator ? widget.height * 0.16 : 0.0;
        behH = hasBehavior ? widget.height * 0.16 : 0.0;
        volH = hasVolatility ? widget.height * 0.16 : 0.0;
        break;
    }
    final xAxisH = 18.0;
    final totalH = priceH + volumeH + volH + oscH + behH + xAxisH + 20;

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

            // Cache layout dimensions for pointer-scroll zone detection.
            _layoutVw = vw;
            _layoutChartBottom = priceH + volumeH + volH + oscH + behH;

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

            return Listener(
              onPointerSignal: (event) {
                if (event is PointerScrollEvent) {
                  GestureBinding.instance.pointerSignalResolver
                      .register(event, (e) {
                    _handlePointerScroll(e as PointerScrollEvent, chartW);
                  });
                }
              },
              child: GestureDetector(
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

                  // ── Social overlay (blue markers — below events) ──
                  if (widget.socialFeedViewModel != null &&
                      widget.socialFeedViewModel!.showOnChart &&
                      widget.socialFeedViewModel!.state.posts.isNotEmpty)
                    Positioned(
                      left: 0,
                      top: 0,
                      width: chartW,
                      height: priceH + volumeH + volH + oscH + behH + xAxisH,
                      child: ChartSocialOverlay(
                        markers: _buildSocialMarkers(chartW),
                        priceAreaHeight: priceH,
                        onNavigateToFeed: widget.onNavigateToFeed,
                      ),
                    ),

                  // ── Event overlay (on top — higher tap priority) ──
                  if (widget.eventsViewModel != null &&
                      widget.eventsViewModel!.state.showEvents)
                    Positioned(
                      left: 0,
                      top: 0,
                      width: chartW,
                      height: priceH + volumeH + volH + oscH + behH + xAxisH,
                      child: ChartEventOverlay(
                        markers: _buildEventMarkers(chartW),
                        onNavigateToEvent: widget.onNavigateToEvent,
                        priceAreaHeight: priceH,
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

                  // ── Volatility layer (intraday activity profile) ──
                  if (hasVolatility)
                    Positioned(
                      left: 0,
                      top: priceH + volumeH,
                      width: chartW,
                      height: volH,
                      child: RepaintBoundary(
                        child: Container(
                          decoration: const BoxDecoration(
                            border: Border(
                              top: BorderSide(color: Color(0x33FFFFFF), width: 0.5),
                            ),
                          ),
                          child: CustomPaint(
                            size: Size(chartW, volH),
                            painter: VolatilityPainter(
                              aligned: widget.volatilityAligned!,
                              startIndex: start,
                              endIndex: end,
                              candleWidth: _candleWidth,
                              scrollPixelOffset: pixOff,
                            ),
                          ),
                        ),
                      ),
                    ),

                  // ── Oscillator layer (RSI + ATR) ──
                  if (hasOscillator)
                    Positioned(
                      left: 0,
                      top: priceH + volumeH + volH,
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

                  // ── Behavioral oscillator layer ──
                  if (hasBehavior)
                    Positioned(
                      left: 0,
                      top: priceH + volumeH + volH + oscH,
                      width: chartW,
                      height: behH,
                      child: RepaintBoundary(
                        child: Container(
                          decoration: const BoxDecoration(
                            border: Border(
                              top: BorderSide(color: Color(0x33FFFFFF), width: 0.5),
                            ),
                          ),
                          child: CustomPaint(
                            size: Size(chartW, behH),
                            painter: BehaviorOscillatorPainter(
                              greed: widget.config.showGreed ? _behavior!.greed : null,
                              fear: widget.config.showFear ? _behavior!.fear : null,
                              patience: widget.config.showPatience ? _behavior!.patience : null,
                              panic: widget.config.showPanic ? _behavior!.panic : null,
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
                    top: priceH + volumeH + volH + oscH + behH,
                    width: chartW,
                    height: xAxisH,
                    child: IgnorePointer(
                      child: XAxisLabels(
                        candles: candles,
                        startIndex: start,
                        endIndex: end + _futureSlots,
                        candleWidth: _candleWidth,
                        scrollPixelOffset: pixOff,
                        timeframe: widget.series.timeframe,
                        futureSlots: _futureSlots,
                        candleDuration: candleDuration(widget.series.timeframe),
                      ),
                    ),
                  ),

                  // ── Reference area green line (just above X-axis) ──
                  if (widget.referenceStartIndex != null)
                    Positioned(
                      left: 0,
                      top: priceH + volumeH + volH + oscH + behH - 2,
                      width: chartW,
                      height: 3,
                      child: IgnorePointer(
                        child: CustomPaint(
                          size: Size(chartW, 3),
                          painter: _ReferenceLinePainter(
                            referenceStartIndex: widget.referenceStartIndex!,
                            referenceEndIndex: candles.length - 1,
                            visibleStartIndex: start,
                            candleWidth: _candleWidth,
                            scrollPixelOffset: pixOff,
                          ),
                        ),
                      ),
                    ),

                  // ── Y-axis drag zone ──
                  Positioned(
                    right: 0,
                    top: 0,
                    width: _yAxisDragW,
                    height: priceH + volumeH + volH + oscH + behH,
                    child: GestureDetector(
                      behavior: HitTestBehavior.opaque,
                      onVerticalDragStart: _onYAxisDragStart,
                      onVerticalDragUpdate: _onYAxisDragUpdate,
                      onDoubleTap: _resetPriceScale,
                      child: const Center(
                        child: Icon(
                          Icons.drag_handle,
                          color: Color(0x33FFFFFF),
                          size: 16,
                        ),
                      ),
                    ),
                  ),

                  // ── X-axis drag zone ──
                  Positioned(
                    left: 0,
                    top: priceH + volumeH + volH + oscH + behH,
                    width: chartW,
                    height: xAxisH + 20,
                    child: GestureDetector(
                      behavior: HitTestBehavior.opaque,
                      onHorizontalDragStart: (d) =>
                          _onXAxisDragStart(d, chartW),
                      onHorizontalDragUpdate: (d) =>
                          _onXAxisDragUpdate(d, chartW),
                      onDoubleTap: () => _resetTimeScale(chartW),
                    ),
                  ),

                  // ── Crosshair overlay ──
                  if (_crosshair != null)
                    Positioned(
                      left: 0,
                      top: 0,
                      width: chartW,
                      height: priceH + volumeH + volH + oscH + behH,
                      child: IgnorePointer(
                        child: CrosshairOverlay(
                          state: _crosshair!,
                          symbol: widget.series.symbol,
                          timeframe: widget.series.timeframe,
                          priceHeight: priceH,
                          volumeHeight: volumeH,
                      oscillatorHeight: oscH + behH + volH,
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

                  // ── Volatility label ──
                  if (hasVolatility)
                    Positioned(
                      right: _yAxisW + 4,
                      top: priceH + volumeH + 2,
                      child: const Text(
                        'Activity',
                        style: TextStyle(
                          color: Color(0x884CAF50),
                          fontSize: 9,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),

                  // ── Oscillator labels ──
                  if (_rsi != null)
                    Positioned(
                      right: _yAxisW + 4,
                      top: priceH + volumeH + volH + 2,
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
                      top: priceH + volumeH + volH + 14,
                      child: const Text(
                        'ATR',
                        style: TextStyle(
                          color: Color(0x8826A69A),
                          fontSize: 9,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),

                  // ── Behavioral indicator labels ──
                  if (hasBehavior) ...[
                    if (widget.config.showGreed)
                      Positioned(
                        right: _yAxisW + 4,
                        top: priceH + volumeH + volH + oscH + 2,
                        child: Text(
                          'Greed',
                          style: TextStyle(
                            color: BehaviorOscillatorPainter.greedColor.withOpacity(0.55),
                            fontSize: 9,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    if (widget.config.showFear)
                      Positioned(
                        right: _yAxisW + 4,
                        top: priceH + volumeH + volH + oscH + 14,
                        child: Text(
                          'Fear',
                          style: TextStyle(
                            color: BehaviorOscillatorPainter.fearColor.withOpacity(0.55),
                            fontSize: 9,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    if (widget.config.showPatience)
                      Positioned(
                        right: _yAxisW + 4,
                        top: priceH + volumeH + volH + oscH + 26,
                        child: Text(
                          'Patience',
                          style: TextStyle(
                            color: BehaviorOscillatorPainter.patienceColor.withOpacity(0.55),
                            fontSize: 9,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    if (widget.config.showPanic)
                      Positioned(
                        right: _yAxisW + 4,
                        top: priceH + volumeH + volH + oscH + 38,
                        child: Text(
                          'Panic',
                          style: TextStyle(
                            color: BehaviorOscillatorPainter.panicColor.withOpacity(0.55),
                            fontSize: 9,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                  ],

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

                  // ── Future events placeholder label ──
                  // Anchored to the midpoint of the future zone so it
                  // scrolls with the chart data instead of sticking to
                  // the viewport edge.
                  if (_futureSlots > 0 && !_hasFutureEvents)
                    Positioned(
                      left: (candles.length + _futureSlots / 2.0 - start) *
                              _candleWidth -
                          pixOff,
                      top: priceH / 2 - 40,
                      child: Transform.rotate(
                        angle: math.pi / 2,
                        child: const Text(
                          'future events will appear here',
                          style: TextStyle(
                            color: Color(0xB3FF0000),
                            fontSize: 9,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                      ),
                    ),
                ],
              ),
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

    final candles = widget.series.candles;
    final start = _visibleStart(chartWidth);
    final end = math.min(_visibleEnd(chartWidth), candles.length);
    final pixOff = _scrollPixelOffset(chartWidth);

    // Build markers for the VISIBLE portion of existing candles.
    final visibleCandles = candles.sublist(start, end);
    if (visibleCandles.isEmpty) return [];

    final visibleSeries = CandleSeriesResponse(
      symbol: widget.series.symbol,
      timeframe: widget.series.timeframe,
      candles: visibleCandles,
    );

    final pastMarkers = buildEventMarkers(
      series: visibleSeries,
      events: evm.state.filteredEvents,
      filterLevel: evm.state.filterLevel,
      candleWidth: _candleWidth,
      scrollPixelOffset: pixOff,
    );

    // Build markers for future events (beyond last candle).
    final tf = widget.series.timeframe;
    final dur = candleDuration(tf);
    final maxSlots = maxProjectionSlots(tf);
    final futureMarkers = buildFutureEventMarkers(
      lastCandleTimestamp: candles.last.timestamp,
      totalCandleCount: candles.length,
      events: evm.state.filteredEvents,
      filterLevel: evm.state.filterLevel,
      candleWidth: _candleWidth,
      candleDuration: dur,
      maxSlots: maxSlots,
      visibleStartIndex: start,
      scrollPixelOffset: pixOff,
    );

    return [...pastMarkers, ...futureMarkers];
  }

  List<SocialMarker> _buildSocialMarkers(double chartWidth) {
    final svm = widget.socialFeedViewModel;
    if (svm == null) return [];

    final candles = widget.series.candles;
    final start = _visibleStart(chartWidth);
    final end = math.min(_visibleEnd(chartWidth), candles.length);
    final pixOff = _scrollPixelOffset(chartWidth);

    final visibleCandles = candles.sublist(start, end);
    if (visibleCandles.isEmpty) return [];

    final visibleSeries = CandleSeriesResponse(
      symbol: widget.series.symbol,
      timeframe: widget.series.timeframe,
      candles: visibleCandles,
    );

    return buildSocialMarkers(
      series: visibleSeries,
      posts: svm.state.posts,
      candleWidth: _candleWidth,
      scrollPixelOffset: pixOff,
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
    final absIdx = visStart + rawIdx;

    // Allow crosshair to extend into the future projection zone.
    final fSlots = _futureSlots;
    final maxIdx = candles.length - 1 + fSlots;
    final clampedIdx = absIdx.clamp(0, maxIdx);

    // Snap X to candle center.
    final snappedX =
        (clampedIdx - visStart) * _candleWidth + _candleWidth / 2 - pixOff;

    if (clampedIdx < candles.length) {
      // Normal zone — existing candle.
      final c = candles[clampedIdx];

      double? emaF, emaS, rsiV, atrV;
      if (_emaFast != null && clampedIdx < _emaFast!.length) emaF = _emaFast![clampedIdx];
      if (_emaSlow != null && clampedIdx < _emaSlow!.length) emaS = _emaSlow![clampedIdx];
      if (_rsi != null && clampedIdx < _rsi!.length) rsiV = _rsi![clampedIdx];
      if (_atr != null && clampedIdx < _atr!.length) atrV = _atr![clampedIdx];

      setState(() {
        _crosshair = CrosshairState(
          candleIndex: clampedIdx,
          x: snappedX,
          touchY: local.dy.clamp(0, priceH),
          candle: c,
          emaFast: emaF,
          emaSlow: emaS,
          rsi: rsiV,
          atr: atrV,
        );
      });
    } else {
      // Future zone — beyond last candle. Show timestamp only.
      final lastTs = candles.last.timestamp;
      final tf = widget.series.timeframe;
      final dur = candleDuration(tf);
      final slotsAhead = clampedIdx - (candles.length - 1);
      final futureTs = lastTs.add(dur * slotsAhead);

      setState(() {
        _crosshair = CrosshairState(
          candleIndex: clampedIdx,
          x: snappedX,
          touchY: local.dy.clamp(0, priceH),
          candle: candles.last, // placeholder
          isFutureZone: true,
          futureTimestamp: futureTs,
        );
      });
    }
  }

  // ── Helpers (shared with crosshair, build, etc.) ──

  void _expandForRange(List<double>? vals, int start, int end, void Function(double) apply) {
    if (vals == null) return;
    for (var i = start; i < end && i < vals.length; i++) {
      if (!vals[i].isNaN) apply(vals[i]);
    }
  }
}



/// Paints a green reference line above the X-axis indicating the overview
/// sparkline window (the candles the user saw before opening detail).
class _ReferenceLinePainter extends CustomPainter {
  final int referenceStartIndex;
  final int referenceEndIndex;
  final int visibleStartIndex;
  final double candleWidth;
  final double scrollPixelOffset;

  _ReferenceLinePainter({
    required this.referenceStartIndex,
    required this.referenceEndIndex,
    required this.visibleStartIndex,
    required this.candleWidth,
    required this.scrollPixelOffset,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final startX = (referenceStartIndex - visibleStartIndex) * candleWidth -
        scrollPixelOffset;
    final endX = (referenceEndIndex - visibleStartIndex + 1) * candleWidth -
        scrollPixelOffset;

    // Only paint the portion that's on screen.
    final clampedStart = startX.clamp(0.0, size.width);
    final clampedEnd = endX.clamp(0.0, size.width);
    if (clampedStart >= clampedEnd) return;

    final paint = Paint()
      ..color = const Color(0xAA00C853) // green with some alpha
      ..strokeWidth = 2.5
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round;

    canvas.drawLine(
      Offset(clampedStart, size.height / 2),
      Offset(clampedEnd, size.height / 2),
      paint,
    );
  }

  @override
  bool shouldRepaint(covariant _ReferenceLinePainter old) {
    return referenceStartIndex != old.referenceStartIndex ||
        referenceEndIndex != old.referenceEndIndex ||
        visibleStartIndex != old.visibleStartIndex ||
        candleWidth != old.candleWidth ||
        scrollPixelOffset != old.scrollPixelOffset;
  }
}
