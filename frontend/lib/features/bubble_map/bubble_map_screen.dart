import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:sensors_plus/sensors_plus.dart';

import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../candles/application/get_candle_series.dart';
import '../detail/chart_navigation.dart';
import '../detail/detail_context.dart';
import '../detail/detail_screen.dart';
import '../events/events_view_model.dart';
import 'bubble_map_state.dart';
import 'bubble_map_view_model.dart';
import 'bubble_packer.dart';
import 'bubble_painter.dart';
import 'bubble_physics.dart';

/// Available timeframes for the bubble map.
const _timeframes = ['1m', '5m', '15m', '1h', '4h', '1d'];

/// Page labels shown in the dropdown.
const _pageLabels = ['Top 50', '51 – 100', '101 – 150'];

/// Bubble map screen showing tokens as size/colour-coded circles.
class BubbleMapScreen extends StatefulWidget {
  final BubbleMapViewModel viewModel;
  final GetCandleSeries getCandleSeries;
  final EventsViewModel? eventsViewModel;

  const BubbleMapScreen({
    Key? key,
    required this.viewModel,
    required this.getCandleSeries,
    this.eventsViewModel,
  }) : super(key: key);

  @override
  State<BubbleMapScreen> createState() => _BubbleMapScreenState();
}

class _BubbleMapScreenState extends State<BubbleMapScreen>
    with SingleTickerProviderStateMixin {
  late final BubbleMapViewModel vm;
  int _highlightIndex = -1;

  // ---- physics mode ----
  bool _physicsMode = false;
  final BubblePhysics _physics = BubblePhysics();
  List<PhysicsBody> _bodies = [];
  Ticker? _ticker;
  Duration _lastTick = Duration.zero;
  StreamSubscription<AccelerometerEvent>? _accelSub;
  double _accelX = 0.0;
  double _accelY = 0.0;
  double _viewWidth = 0;
  double _viewHeight = 0;

  /// Bubble positions driven by physics (overrides packed positions while
  /// physics mode is active or frozen).
  List<PackedBubble>? _physicsBubbles;
  List<double>? _physicsAngles;

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;
    vm.onChanged = () {
      if (mounted) {
        // When data reloads, re-init physics bodies if mode is on.
        if (_physicsMode) {
          _initBodies(vm.state.bubbles);
        } else {
          _physicsBubbles = null;
          _physicsAngles = null;
        }
        setState(() {});
      }
    };
  }

  @override
  void dispose() {
    _stopPhysics();
    vm.onChanged = null;
    super.dispose();
  }

  // ---- physics helpers ----

  void _togglePhysics(bool on) {
    setState(() => _physicsMode = on);
    if (on) {
      _initBodies(vm.state.bubbles);
      _startPhysics();
    } else {
      _stopPhysics();
      // Freeze bubbles at current physics positions — _physicsBubbles stays.
    }
  }

  void _initBodies(List<PackedBubble> bubbles) {
    // Use physics-modified positions if available (freeze → re-enable).
    final source = _physicsBubbles ?? bubbles;
    _bodies = List.generate(source.length, (i) {
      final b = source[i];
      return PhysicsBody(
        x: b.x,
        y: b.y,
        radius: b.radius,
        // Preserve existing velocity if re-initing from same set.
        vx: i < _bodies.length ? _bodies[i].vx : 0,
        vy: i < _bodies.length ? _bodies[i].vy : 0,
        angle: i < _bodies.length ? _bodies[i].angle : 0,
        angularVelocity:
            i < _bodies.length ? _bodies[i].angularVelocity : 0,
      );
    });
    _syncPhysicsBubbles();
  }

  void _startPhysics() {
    _lastTick = Duration.zero;
    _ticker?.dispose();
    _ticker = createTicker(_onTick)..start();

    _accelSub?.cancel();
    _accelSub = accelerometerEventStream().listen((event) {
      // Device accelerometer: x = sideways, y = up/down.
      // For screen coords: positive x → right, positive y → down.
      // Accelerometer y is negative when tilted forward (phone top tilts
      // away from you), so we flip: screen-gravity-y = accel.y negated.
      // Accelerometer x: tilting right gives positive x, which should
      // push bubbles right.  But sensor convention varies — we negate x
      // too for the most natural feel (tilt phone left → bubbles roll
      // left = negative screen x).
      _accelX = -event.x * 120; // scale to px/s²
      _accelY = event.y * 120;
    });
  }

  void _stopPhysics() {
    _ticker?.stop();
    _ticker?.dispose();
    _ticker = null;
    _accelSub?.cancel();
    _accelSub = null;
  }

  void _onTick(Duration elapsed) {
    if (_bodies.isEmpty) return;
    final dt = _lastTick == Duration.zero
        ? 1 / 60
        : (elapsed - _lastTick).inMicroseconds / 1e6;
    _lastTick = elapsed;

    // Clamp dt to avoid explosion on resume from background.
    final safeDt = dt.clamp(0.001, 0.05);

    _physics.step(
      _bodies,
      dt: safeDt,
      width: _viewWidth,
      height: _viewHeight,
      gravityX: _accelX,
      gravityY: _accelY,
    );

    _syncPhysicsBubbles();
    setState(() {});
  }

  void _syncPhysicsBubbles() {
    final orig = vm.state.bubbles;
    if (_bodies.length != orig.length) return;

    _physicsBubbles = List.generate(orig.length, (i) {
      return PackedBubble(
        token: orig[i].token,
        x: _bodies[i].x,
        y: _bodies[i].y,
        radius: orig[i].radius,
        colorValue: orig[i].colorValue,
      );
    });
    _physicsAngles =
        _bodies.map((b) => b.angle).toList(growable: false);
  }

  // ---- navigation ----

  String _detailTimeframe(String overviewTf) {
    final idx = _timeframes.indexOf(overviewTf);
    if (idx <= 0) return overviewTf;
    return _timeframes[idx - 1];
  }

  Future<void> _onBubbleTap(PackedBubble bubble) async {
    final token = bubble.token;
    final now = DateTime.now().toUtc();
    final detailTf = _detailTimeframe(vm.state.timeframe);

    final input = buildDetailChartInput(
      symbol: token.symbol,
      timeframe: detailTf,
      now: now,
    );

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (_) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      Navigator.of(context).pop(); // dismiss loading
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(token.symbol),
            timeframe: Timeframe(detailTf),
            series: series,
            warmupCount: kIndicatorWarmup,
            initialVisibleCount: kSparklineCandles,
            eventsViewModel: widget.eventsViewModel,
            detailContext: DetailContext(
              rank: 0,
              totalScore: token.totalScore,
              trendScore: token.trendScore,
              sidewaysScore: token.sidewaysScore,
              gainScore: token.gainScore,
              volume: token.volume,
            ),
          ),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load chart: $e')),
      );
    }
  }

  // ---- hit test ----

  int _hitTest(Offset position) {
    final bubbles = _physicsBubbles ?? vm.state.bubbles;
    // Check in reverse so topmost (last drawn) wins.
    for (var i = bubbles.length - 1; i >= 0; i--) {
      final b = bubbles[i];
      final dx = position.dx - b.x;
      final dy = position.dy - b.y;
      if (dx * dx + dy * dy <= b.radius * b.radius) return i;
    }
    return -1;
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final state = vm.state;

    return Scaffold(
      backgroundColor: const Color(0xFF1A1A2E),
      appBar: AppBar(
        backgroundColor: const Color(0xFF1A1A2E),
        title: const Text(
          'Bubble Map',
          style: TextStyle(
            color: Color(0xFF00e6c0),
            fontWeight: FontWeight.w700,
          ),
        ),
        iconTheme: const IconThemeData(color: Colors.white),
        actions: [
          // Timeframe dropdown
          _dropdown<String>(
            value: state.timeframe,
            items: _timeframes,
            label: (v) => v,
            onChanged: (v) {
              if (v == null) return;
              _reload(timeframe: v);
            },
          ),
          const SizedBox(width: 8),
          // Page dropdown
          _dropdown<int>(
            value: state.pageIndex,
            items: List.generate(_pageLabels.length, (i) => i),
            label: (v) => _pageLabels[v],
            onChanged: (v) {
              if (v == null) return;
              _reload(pageIndex: v);
            },
          ),
          const SizedBox(width: 8),
          // Size-by toggle
          IconButton(
            icon: Icon(
              state.sizeBy == 'volume'
                  ? Icons.bar_chart
                  : Icons.show_chart,
              color: Colors.white,
            ),
            tooltip:
                state.sizeBy == 'volume' ? 'Size by volume' : 'Size by change',
            onPressed: () {
              vm.changeSizeBy(
                  state.sizeBy == 'volume' ? 'change' : 'volume');
            },
          ),
        ],
      ),
      body: SafeArea(
        top: false,
        child: Column(
          children: [
            _buildLegend(state),
            Expanded(child: _buildBody(state)),
          ],
        ),
      ),
    );
  }

  Widget _buildLegend(BubbleMapState state) {
    final isVolume = state.sizeBy == 'volume';
    final sizeLabel = isVolume ? 'Volume' : '|Price Change|';
    final colorLabel = isVolume ? 'Volume rank' : 'Price change';

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      color: const Color(0xFF22223A),
      child: Row(
        children: [
          // Size legend
          const Icon(Icons.circle, size: 10, color: Colors.white54),
          const SizedBox(width: 4),
          Text(
            'Size = $sizeLabel',
            style: const TextStyle(color: Colors.white70, fontSize: 11),
          ),
          const SizedBox(width: 16),
          // Colour legend
          Container(width: 10, height: 10, color: const Color(0xFFFF1744)),
          Container(width: 10, height: 10, color: const Color(0xFF555555)),
          Container(width: 10, height: 10, color: const Color(0xFF00C853)),
          const SizedBox(width: 4),
          Flexible(
            child: Text(
              'Color = $colorLabel',
              style: const TextStyle(color: Colors.white70, fontSize: 11),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          const Spacer(),
          // Physics mode toggle
          GestureDetector(
            onTap: () => _togglePhysics(!_physicsMode),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  _physicsMode ? Icons.check_box : Icons.check_box_outline_blank,
                  size: 18,
                  color: _physicsMode
                      ? const Color(0xFF00e6c0)
                      : Colors.white54,
                ),
                const SizedBox(width: 4),
                Text(
                  'Fancy',
                  style: TextStyle(
                    color: _physicsMode
                        ? const Color(0xFF00e6c0)
                        : Colors.white54,
                    fontSize: 11,
                    fontWeight:
                        _physicsMode ? FontWeight.w600 : FontWeight.normal,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildBody(BubbleMapState state) {
    if (state.isLoading && state.bubbles.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.bubbles.isEmpty) {
      return Center(
        child: Text(state.error!, style: const TextStyle(color: Colors.white)),
      );
    }

    return LayoutBuilder(
      builder: (context, constraints) {
        final w = constraints.maxWidth;
        final h = constraints.maxHeight;
        _viewWidth = w;
        _viewHeight = h;

        // Trigger initial load or relayout when size changes.
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (state.bubbles.isEmpty && !state.isLoading) {
            vm.load(
              timeframe: state.timeframe,
              pageIndex: state.pageIndex,
              width: w,
              height: h,
            );
          } else {
            vm.relayout(w, h);
          }
        });

        final displayBubbles = _physicsBubbles ?? state.bubbles;

        return GestureDetector(
          onTapDown: (details) {
            final idx = _hitTest(details.localPosition);
            if (idx != _highlightIndex) {
              setState(() => _highlightIndex = idx);
            }
          },
          onTapUp: (details) {
            final idx = _hitTest(details.localPosition);
            if (idx >= 0) {
              _onBubbleTap(displayBubbles[idx]);
            }
            setState(() => _highlightIndex = -1);
          },
          onTapCancel: () {
            if (_highlightIndex != -1) {
              setState(() => _highlightIndex = -1);
            }
          },
          child: CustomPaint(
            size: Size(w, h),
            painter: BubblePainter(
              bubbles: displayBubbles,
              highlightIndex: _highlightIndex,
              angles: _physicsAngles,
            ),
          ),
        );
      },
    );
  }

  // ---- helpers ----

  void _reload({String? timeframe, int? pageIndex}) {
    final w = context.size?.width ?? 0;
    final h = (context.size?.height ?? 0) - kToolbarHeight;
    vm.load(
      timeframe: timeframe ?? vm.state.timeframe,
      pageIndex: pageIndex ?? vm.state.pageIndex,
      width: w,
      height: math.max(h, 0),
    );
  }

  Widget _dropdown<T>({
    required T value,
    required List<T> items,
    required String Function(T) label,
    required ValueChanged<T?> onChanged,
  }) {
    return DropdownButtonHideUnderline(
      child: DropdownButton<T>(
        value: value,
        dropdownColor: const Color(0xFF2A2A4A),
        style: const TextStyle(color: Colors.white, fontSize: 13),
        items: items
            .map((e) => DropdownMenuItem(value: e, child: Text(label(e))))
            .toList(),
        onChanged: onChanged,
      ),
    );
  }
}
