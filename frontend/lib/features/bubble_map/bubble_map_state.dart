import 'bubble_packer.dart';

/// Immutable state for the bubble map screen.
class BubbleMapState {
  final bool isLoading;
  final List<PackedBubble> bubbles;
  final String timeframe;
  final int pageIndex; // 0 = 1–50, 1 = 51–100, 2 = 101–150
  final String sizeBy; // 'volume' or 'change'
  final String? error;

  const BubbleMapState({
    required this.isLoading,
    required this.bubbles,
    required this.timeframe,
    required this.pageIndex,
    required this.sizeBy,
    this.error,
  });

  factory BubbleMapState.initial() => const BubbleMapState(
        isLoading: false,
        bubbles: [],
        timeframe: '15m',
        pageIndex: 0,
        sizeBy: 'volume',
      );

  BubbleMapState copyWith({
    bool? isLoading,
    List<PackedBubble>? bubbles,
    String? timeframe,
    int? pageIndex,
    String? sizeBy,
    String? error,
  }) {
    return BubbleMapState(
      isLoading: isLoading ?? this.isLoading,
      bubbles: bubbles ?? this.bubbles,
      timeframe: timeframe ?? this.timeframe,
      pageIndex: pageIndex ?? this.pageIndex,
      sizeBy: sizeBy ?? this.sizeBy,
      error: error,
    );
  }
}
