import 'package:flutter/material.dart';

/// Bottom-right tile chip: `+3.2% vs mkt`.
///
/// Hidden unless [rsAvailable] and [rs] are both set (real `0` still shows).
/// Paint stays compact; a small translucent pad expands the tap target without
/// covering half of a 3-column tile.
class RelativeStrengthChip extends StatelessWidget {
  final bool rsAvailable;
  final double? rs;
  final double? beta;
  final bool dense;

  const RelativeStrengthChip({
    super.key,
    required this.rsAvailable,
    required this.rs,
    this.beta,
    this.dense = true,
  });

  /// Formats log excess return as a percent label (`rs × 100`, one decimal).
  static String labelFor(double rs) {
    final pct = rs * 100;
    final rounded = pct.toStringAsFixed(1);
    if (rounded == '0.0' || rounded == '-0.0') {
      return '0.0% vs mkt';
    }
    final sign = pct > 0 ? '+' : '';
    return '$sign$rounded% vs mkt';
  }

  /// Chip color aligned with [labelFor] (grey when the label is neutral zero).
  static Color colorFor(double rs) {
    final label = labelFor(rs);
    if (label.startsWith('0.0%')) return Colors.grey;
    if (rs > 0) return Colors.green;
    return Colors.red;
  }

  @override
  Widget build(BuildContext context) {
    final value = rs;
    if (!rsAvailable || value == null) return const SizedBox.shrink();
    final fontSize = dense ? 8.0 : 11.0;
    final label = labelFor(value);
    return Semantics(
      button: true,
      label: 'Relative strength $label',
      child: GestureDetector(
        key: const ValueKey('relative-strength-chip'),
        behavior: HitTestBehavior.translucent,
        onTap: () => _showExplanation(context, value, beta),
        child: Padding(
          // Hit slop only — does not force a large painted box.
          padding: const EdgeInsets.all(6),
          child: Container(
            padding: EdgeInsets.symmetric(
              horizontal: dense ? 4 : 6,
              vertical: dense ? 2 : 3,
            ),
            decoration: BoxDecoration(
              color: colorFor(value).withAlpha((0.8 * 255).round()),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              label,
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

  void _showExplanation(
    BuildContext context,
    double rsValue,
    double? betaValue,
  ) {
    final betaLine = betaValue == null
        ? ''
        : '\n\nBeta for this symbol: ${betaValue.toStringAsFixed(2)}.';
    showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Relative strength'),
        content: Text(
          'Relative strength: this symbol\'s return minus the market '
          'composite\'s return over the same window. Beta: how much it '
          'moves per 1% market move.$betaLine',
        ),
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
