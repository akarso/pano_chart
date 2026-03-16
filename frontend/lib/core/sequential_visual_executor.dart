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
    this.animateStagger = const Duration(milliseconds: 10),
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

  /// Once true, the item is permanently visible (progress == 1)
  /// and will never be hidden again, even across data refreshes.
  bool frozen = false;
}

/// A reusable, decoupled orchestrator that runs a staggered visual effect
/// across a list of keyed items that enter the viewport.
///
/// **Lifecycle rules:**
///
/// - Once an item's animation completes, it is **frozen**: permanently visible
///   at progress 1.0. Scrolling, viewport changes, or load-more appends
///   cannot hide frozen items.
/// - [grow] appends new items without disturbing existing ones.  Use when
///   paginated data arrives.
/// - [reset] clears everything and re-prepares for [count] items.  Use when
///   the full data set changes (refresh, sort, timeframe change).
///
/// **Usage pattern:**
///
/// 1. Create an instance (typically in `State.initState`).
/// 2. Call [reset] or [grow] whenever the backing list changes.
/// 3. Wrap each grid/list child with [buildChild].
/// 4. Call [execute] to kick off the staggered sequence for newly-added items.
/// 5. Call [dispose] in `State.dispose`.
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

  /// When true, all animations are skipped and items appear instantly.
  bool disabled = false;

  SequentialVisualExecutor({
    required this.config,
    required this.vsync,
    required this.effectBuilder,
    this.onComplete,
  });

  /// Hard-reset: dispose all controllers, clear state, create [count]
  /// fresh (un-animated) items.  Use for refresh / sort / timeframe change.
  void reset(int count) {
    _generation++;
    for (final item in _items) {
      item.controller?.dispose();
    }
    _items.clear();
    for (var i = 0; i < count; i++) {
      _items.add(_ItemState());
    }
  }

  /// Backwards-compatible alias for [reset].
  void prepare(int count) => reset(count);

  /// Grow to [newTotal] items.  Existing items are preserved (including
  /// frozen ones).  New items are appended in un-animated state.
  /// If [newTotal] <= current length, this is a no-op.
  void grow(int newTotal) {
    if (newTotal <= _items.length) return;
    final toAdd = newTotal - _items.length;
    for (var i = 0; i < toAdd; i++) {
      _items.add(_ItemState());
    }
  }

  /// Whether item [index] has completed its animation (progress == 1).
  bool isComplete(int index) {
    if (index < 0 || index >= _items.length) return true;
    return _items[index].frozen || _items[index].animationValue >= 1.0;
  }

  /// Current progress (0→1) for item [index].
  double progress(int index) {
    if (index < 0 || index >= _items.length) return 1.0;
    if (_items[index].frozen) return 1.0;
    return _items[index].animationValue;
  }

  /// Whether item [index] has appeared (is laid out).
  bool hasAppeared(int index) {
    if (index < 0 || index >= _items.length) return true;
    return _items[index].frozen || _items[index].appeared;
  }

  /// Wraps [child] at [index] with the current visual effect state.
  ///
  /// Frozen items are returned as-is (fully visible, no wrapper).
  /// Un-appeared items return the child wrapped at opacity 0 (laid out but
  /// invisible) so grid/list layout is preserved.
  Widget buildChild(int index, Widget child) {
    if (disabled) return child;
    if (index < 0 || index >= _items.length) return child;
    final state = _items[index];
    if (state.frozen) return child;
    if (!state.appeared) {
      // Keep the child laid-out (same size) but fully transparent so the
      // grid layout remains stable.  SizedBox.shrink was causing items to
      // collapse and never recover.
      return Opacity(opacity: 0, child: child);
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
    if (_disposed || disabled) {
      // If disabled, mark everything as frozen immediately.
      if (disabled) skipAll();
      return;
    }
    final gen = _generation;

    // Filter to un-animated, non-frozen items.
    final targets = visibleIndices
        .where((i) =>
            i >= 0 &&
            i < _items.length &&
            !_items[i].appeared &&
            !_items[i].frozen)
        .toList();
    if (targets.isEmpty) return;

    for (var order = 0; order < targets.length; order++) {
      final idx = targets[order];
      final appearDelay = config.appearStagger * order;
      final animateDelay = config.animateStagger * order;

      // Phase 1: appear
      Future.delayed(appearDelay, () {
        if (_disposed || gen != _generation) return;
        if (idx >= _items.length) return;
        _items[idx].appeared = true;
        onFrame?.call();

        // Phase 2: animate
        Future.delayed(animateDelay, () {
          if (_disposed || gen != _generation) return;
          if (idx >= _items.length) return;
          final ctrl = AnimationController(
            vsync: vsync,
            duration: config.animateDuration,
          );
          _items[idx].controller = ctrl;
          ctrl.addListener(() {
            if (_disposed || gen != _generation) return;
            if (idx >= _items.length) return;
            _items[idx].animationValue = config.animateCurve.transform(ctrl.value);
            onFrame?.call();
          });
          ctrl.addStatusListener((status) {
            if (status == AnimationStatus.completed) {
              if (idx < _items.length) {
                _freeze(idx);
              }
              onFrame?.call();
              // Check if all done.
              if (_items.every((s) => s.frozen)) {
                onComplete?.call();
              }
            }
          });
          ctrl.forward();
        });
      });
    }
  }

  /// Mark item at [index] as permanently visible and dispose its controller.
  void _freeze(int index) {
    final item = _items[index];
    item.frozen = true;
    item.appeared = true;
    item.animationValue = 1.0;
    item.controller?.dispose();
    item.controller = null;
  }

  /// Mark all items as fully animated (skip animation).
  void skipAll() {
    for (var i = 0; i < _items.length; i++) {
      _freeze(i);
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
