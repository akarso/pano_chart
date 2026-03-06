import 'package:flutter/widgets.dart';

/// Configuration for a single sequential visual effect.
///
/// Each step has two phases:
///  1. **appear** — item goes from invisible to laid-out (e.g. display: none → block)
///  2. **animate** — a property transitions (e.g. opacity 0 → 1)
///
/// Delays between items are multiplied by the item's *viewport-order index*
/// (i.e. only items currently in the viewport participate, numbered 0, 1, 2…).
class SequentialEffectConfig {
  /// Stagger delay between items for the appear phase.
  final Duration appearStagger;

  /// Stagger delay between items for the animate phase.
  final Duration animateStagger;

  /// Duration of the animate phase per item.
  final Duration animateDuration;

  /// Curve for the animate phase.
  final Curve animateCurve;

  const SequentialEffectConfig({
    this.appearStagger = const Duration(milliseconds: 10),
    this.animateStagger = const Duration(milliseconds: 20),
    this.animateDuration = const Duration(milliseconds: 300),
    this.animateCurve = Curves.easeIn,
  });
}

/// Tracks per-item animation state for the [SequentialVisualExecutor].
class _ItemState {
  bool appeared = false;
  bool animating = false;
  double animationValue = 0.0;
  AnimationController? controller;
}

/// A reusable, decoupled orchestrator that runs a staggered visual effect
/// across a list of keyed items that enter the viewport.
///
/// **Usage pattern:**
///
/// 1. Create an instance (typically in `State.initState`).
/// 2. Call [prepare] with the total item count whenever the list changes
///    (e.g. after data load / refresh). This resets all animation state.
/// 3. Wrap each grid/list child with [buildChild], which returns the child
///    widget after applying the current visual effect state.
/// 4. Call [execute] to kick off the staggered sequence for all items
///    currently in the viewport.
/// 5. Call [dispose] in `State.dispose`.
///
/// The executor is **animation-agnostic**: callers provide an
/// [effectBuilder] callback that receives the child widget and a `double`
/// progress value (0 → 1) and returns the transformed widget.
/// This lets the same executor drive opacity, scale, slide, color — anything.
class SequentialVisualExecutor {
  final SequentialEffectConfig config;
  final TickerProvider vsync;

  /// (child, progress 0→1) → transformed widget.
  final Widget Function(Widget child, double progress) effectBuilder;

  /// Optional callback fired when the full sequence completes.
  final VoidCallback? onComplete;

  final List<_ItemState> _items = [];
  bool _disposed = false;
  int _generation = 0;

  SequentialVisualExecutor({
    required this.config,
    required this.vsync,
    required this.effectBuilder,
    this.onComplete,
  });

  /// Reset state for [count] items. Call before [execute] whenever the
  /// backing data set changes.
  void prepare(int count) {
    _generation++;
    for (final item in _items) {
      item.controller?.dispose();
    }
    _items.clear();
    for (var i = 0; i < count; i++) {
      _items.add(_ItemState());
    }
  }

  /// Whether item [index] has completed its animation (progress == 1).
  bool isComplete(int index) {
    if (index < 0 || index >= _items.length) return true;
    return _items[index].animationValue >= 1.0;
  }

  /// Current progress (0→1) for item [index].
  double progress(int index) {
    if (index < 0 || index >= _items.length) return 1.0;
    return _items[index].animationValue;
  }

  /// Whether item [index] has appeared (is laid out).
  bool hasAppeared(int index) {
    if (index < 0 || index >= _items.length) return true;
    return _items[index].appeared;
  }

  /// Wraps [child] at [index] with the current visual effect state.
  Widget buildChild(int index, Widget child) {
    if (index < 0 || index >= _items.length) return child;
    final state = _items[index];
    if (!state.appeared) {
      // Item hasn't appeared yet — invisible (zero-size).
      return const SizedBox.shrink();
    }
    return effectBuilder(child, state.animationValue);
  }

  /// Kick off the staggered animation for items at [visibleIndices].
  ///
  /// Only items that haven't already animated will participate.
  /// The appear delay is `index-in-visible-list * appearStagger`, and the
  /// animate delay is `index-in-visible-list * animateStagger` added after
  /// all appear delays.
  void execute(
    List<int> visibleIndices, {
    VoidCallback? onFrame,
  }) {
    if (_disposed) return;
    final gen = _generation;

    // Filter to un-animated items.
    final targets = visibleIndices
        .where((i) => i >= 0 && i < _items.length && !_items[i].appeared)
        .toList();
    if (targets.isEmpty) return;

    for (var order = 0; order < targets.length; order++) {
      final idx = targets[order];
      final appearDelay = config.appearStagger * order;
      final animateDelay = config.animateStagger * order;

      // Phase 1: appear
      Future.delayed(appearDelay, () {
        if (_disposed || gen != _generation) return;
        _items[idx].appeared = true;
        onFrame?.call();

        // Phase 2: animate
        Future.delayed(animateDelay, () {
          if (_disposed || gen != _generation) return;
          final ctrl = AnimationController(
            vsync: vsync,
            duration: config.animateDuration,
          );
          _items[idx].controller = ctrl;
          ctrl.addListener(() {
            if (_disposed || gen != _generation) return;
            _items[idx].animationValue = config.animateCurve.transform(ctrl.value);
            onFrame?.call();
          });
          ctrl.addStatusListener((status) {
            if (status == AnimationStatus.completed) {
              _items[idx].animationValue = 1.0;
              onFrame?.call();
              // Check if all done.
              if (_items.every((s) => s.animationValue >= 1.0)) {
                onComplete?.call();
              }
            }
          });
          ctrl.forward();
        });
      });
    }
  }

  /// Mark all items as fully animated (skip animation).
  void skipAll() {
    for (final item in _items) {
      item.appeared = true;
      item.animationValue = 1.0;
    }
  }

  void dispose() {
    _disposed = true;
    for (final item in _items) {
      item.controller?.dispose();
    }
    _items.clear();
  }
}
