import 'package:flutter/material.dart';

import 'scorecard_data.dart';

/// Small pill `58% · n=412`. Hidden when [item] is null or `n < 30`.
class ReliabilityChip extends StatelessWidget {
  final ScorecardSummaryItem? item;
  final bool dense;

  const ReliabilityChip({super.key, required this.item, this.dense = false});

  static Color colorFor(ReliabilityTone tone) {
    switch (tone) {
      case ReliabilityTone.green:
        return Colors.green;
      case ReliabilityTone.grey:
        return const Color(0xFF616161);
      case ReliabilityTone.red:
        return Colors.red;
    }
  }

  @override
  Widget build(BuildContext context) {
    final row = item;
    if (row == null) return const SizedBox.shrink();
    final tone = reliabilityTone(row);
    if (tone == null) return const SizedBox.shrink();
    final fontSize = dense ? 8.0 : 11.0;
    return GestureDetector(
      key: ValueKey('reliability-chip-${row.kind}|${row.label}'),
      behavior: HitTestBehavior.opaque,
      onTap: () => _showExplanation(context, row),
      child: ConstrainedBox(
        constraints: dense
            ? const BoxConstraints()
            : const BoxConstraints(minWidth: 32, minHeight: 24),
        child: Align(
          widthFactor: 1,
          heightFactor: 1,
          child: Container(
            padding: EdgeInsets.symmetric(
              horizontal: dense ? 3 : 6,
              vertical: dense ? 1 : 2,
            ),
            decoration: BoxDecoration(
              color: colorFor(tone).withAlpha((0.85 * 255).round()),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Text(
              reliabilityChipLabel(row),
              style: TextStyle(
                color: Colors.white,
                fontSize: fontSize,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
      ),
    );
  }

  void _showExplanation(BuildContext context, ScorecardSummaryItem row) {
    showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text(scorecardRowTitle(row.kind, row.label)),
        content: Text(reliabilityExplanation(row)),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }
}
