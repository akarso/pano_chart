import 'package:flutter/animation.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/core/sequential_visual_executor.dart';

void main() {
  group('SequentialVisualExecutor', () {
    testWidgets('prepare resets items', (tester) async {
      late final SequentialVisualExecutor executor;
      await tester.pumpWidget(
        _TestWidget(onCreate: (vsync) {
          executor = SequentialVisualExecutor(
            config: const SequentialEffectConfig(),
            vsync: vsync,
            effectBuilder: (child, p) => Opacity(opacity: p, child: child),
          );
        }),
      );

      executor.prepare(5);
      expect(executor.hasAppeared(0), isFalse);
      expect(executor.progress(0), 0.0);
      expect(executor.isComplete(0), isFalse);
      expect(executor.isComplete(10), isTrue, reason: 'out-of-range = complete');

      executor.dispose();
    });

    testWidgets('skipAll marks everything complete', (tester) async {
      late final SequentialVisualExecutor executor;
      await tester.pumpWidget(
        _TestWidget(onCreate: (vsync) {
          executor = SequentialVisualExecutor(
            config: const SequentialEffectConfig(),
            vsync: vsync,
            effectBuilder: (child, p) => Opacity(opacity: p, child: child),
          );
        }),
      );

      executor.prepare(3);
      executor.skipAll();
      expect(executor.isComplete(0), isTrue);
      expect(executor.isComplete(1), isTrue);
      expect(executor.isComplete(2), isTrue);
      expect(executor.hasAppeared(0), isTrue);

      executor.dispose();
    });

    testWidgets('buildChild returns SizedBox.shrink before appear',
        (tester) async {
      late final SequentialVisualExecutor executor;
      await tester.pumpWidget(
        _TestWidget(onCreate: (vsync) {
          executor = SequentialVisualExecutor(
            config: const SequentialEffectConfig(),
            vsync: vsync,
            effectBuilder: (child, p) => Opacity(opacity: p, child: child),
          );
        }),
      );

      executor.prepare(1);
      final widget = executor.buildChild(0, const Text('hello'));
      // Should be a SizedBox.shrink since not yet appeared.
      expect(widget, isA<SizedBox>());

      executor.dispose();
    });

    testWidgets('execute triggers appear and animation', (tester) async {
      late final SequentialVisualExecutor executor;
      int frameCount = 0;
      await tester.pumpWidget(
        _TestWidget(onCreate: (vsync) {
          executor = SequentialVisualExecutor(
            config: const SequentialEffectConfig(
              appearStagger: Duration(milliseconds: 10),
              animateStagger: Duration(milliseconds: 20),
              animateDuration: Duration(milliseconds: 100),
            ),
            vsync: vsync,
            effectBuilder: (child, p) => Opacity(opacity: p, child: child),
          );
        }),
      );

      executor.prepare(2);
      executor.execute([0, 1], onFrame: () => frameCount++);

      // Wait for appear staggers + animation.
      await tester.pump(const Duration(milliseconds: 50));
      expect(executor.hasAppeared(0), isTrue);

      await tester.pump(const Duration(milliseconds: 200));
      await tester.pump(const Duration(milliseconds: 200));

      // After enough time, should be complete.
      expect(executor.isComplete(0), isTrue);
      expect(frameCount, greaterThan(0));

      executor.dispose();
    });

    testWidgets('prepare during execute cancels previous generation',
        (tester) async {
      late final SequentialVisualExecutor executor;
      await tester.pumpWidget(
        _TestWidget(onCreate: (vsync) {
          executor = SequentialVisualExecutor(
            config: const SequentialEffectConfig(
              appearStagger: Duration(milliseconds: 50),
              animateDuration: Duration(milliseconds: 100),
            ),
            vsync: vsync,
            effectBuilder: (child, p) => Opacity(opacity: p, child: child),
          );
        }),
      );

      executor.prepare(2);
      executor.execute([0, 1]);

      // Re-prepare should reset everything.
      executor.prepare(1);
      expect(executor.hasAppeared(0), isFalse);
      expect(executor.progress(0), 0.0);

      // Flush any pending timers from the cancelled generation.
      await tester.pumpAndSettle();

      executor.dispose();
    });
  });
}

/// Minimal widget that provides a TickerProvider for tests.
class _TestWidget extends StatefulWidget {
  final void Function(TickerProvider vsync) onCreate;
  const _TestWidget({required this.onCreate});

  @override
  State<_TestWidget> createState() => _TestWidgetState();
}

class _TestWidgetState extends State<_TestWidget>
    with TickerProviderStateMixin {
  @override
  void initState() {
    super.initState();
    widget.onCreate(this);
  }

  @override
  Widget build(BuildContext context) => const SizedBox();
}
