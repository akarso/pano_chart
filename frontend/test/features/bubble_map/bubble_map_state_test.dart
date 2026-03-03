import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/bubble_map/bubble_map_state.dart';

void main() {
  group('BubbleMapState', () {
    test('initial factory', () {
      final state = BubbleMapState.initial();
      expect(state.isLoading, false);
      expect(state.bubbles, isEmpty);
      expect(state.timeframe, '15m');
      expect(state.pageIndex, 0);
      expect(state.sizeBy, 'change');
      expect(state.error, isNull);
    });

    test('copyWith replaces fields', () {
      final state = BubbleMapState.initial();
      final updated = state.copyWith(
        isLoading: true,
        timeframe: '1h',
        pageIndex: 2,
        sizeBy: 'change',
        error: 'oops',
      );

      expect(updated.isLoading, true);
      expect(updated.timeframe, '1h');
      expect(updated.pageIndex, 2);
      expect(updated.sizeBy, 'change');
      expect(updated.error, 'oops');
      // Unchanged
      expect(updated.bubbles, isEmpty);
    });

    test('copyWith preserves unset fields', () {
      final state = BubbleMapState.initial().copyWith(timeframe: '4h');
      final copy = state.copyWith(isLoading: true);
      expect(copy.timeframe, '4h');
    });
  });
}
