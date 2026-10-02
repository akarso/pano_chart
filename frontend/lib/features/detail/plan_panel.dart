import 'package:flutter/material.dart';

import '../../core/format_price.dart';
import 'plan_data.dart';

/// Account-risk bounds for the Plan panel (PR-111).
const double kPlanRiskMin = 0.01;
const double kPlanRiskMax = 1e7;

/// Formats [risk] for the Plan risk field. Whole numbers stay integer;
/// fractions use 2 decimal places — callers must size/persist this value.
String formatPlanRisk(double risk) {
  if (risk == risk.roundToDouble()) return risk.toStringAsFixed(0);
  return risk.toStringAsFixed(2);
}

/// Range trade plan panel (Long/Short, levels, risk → size).
class PlanPanel extends StatelessWidget {
  final PlanData data;
  final bool isLong;
  final double risk;
  final TextEditingController riskController;
  final ValueChanged<bool> onLongChanged;
  final ValueChanged<double> onRiskChanged;
  final VoidCallback? onRetry;
  final bool showRetry;

  const PlanPanel({
    Key? key,
    required this.data,
    required this.isLong,
    required this.risk,
    required this.riskController,
    required this.onLongChanged,
    required this.onRiskChanged,
    this.onRetry,
    this.showRetry = false,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final quality = data.rangeQuality.clamp(0.0, 1.0);
    // Color must not contradict a rejection reason — mute when invalid.
    final Color qualityColor;
    if (!data.valid) {
      qualityColor = Colors.white38;
    } else if (quality >= 0.75) {
      qualityColor = Colors.green;
    } else if (quality >= 0.5) {
      qualityColor = Colors.amber;
    } else {
      qualityColor = Colors.red;
    }

    final entry = isLong ? data.longEntry : data.shortEntry;
    final stop = isLong ? data.longStop : data.shortStop;
    final target = isLong ? data.longTarget : data.shortTarget;
    final size = data.valid ? PlanData.sizeFor(risk, entry, stop) : 0.0;

    final children = <Widget>[
      Row(
        children: [
          ChoiceChip(
            key: const Key('plan-long'),
            label: const Text('Long'),
            selected: isLong,
            onSelected: (selected) {
              if (selected) onLongChanged(true);
            },
            selectedColor: Colors.teal.shade700,
            labelStyle: TextStyle(
              color: isLong ? Colors.white : Colors.white70,
              fontSize: 12,
            ),
          ),
          const SizedBox(width: 8),
          ChoiceChip(
            key: const Key('plan-short'),
            label: const Text('Short'),
            selected: !isLong,
            onSelected: (selected) {
              if (selected) onLongChanged(false);
            },
            selectedColor: Colors.red.shade700,
            labelStyle: TextStyle(
              color: !isLong ? Colors.white : Colors.white70,
              fontSize: 12,
            ),
          ),
          const Spacer(),
          Container(
            key: const Key('plan-quality-dot'),
            width: 10,
            height: 10,
            decoration: BoxDecoration(
              color: qualityColor,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 6),
          Text(
            key: const Key('plan-quality-label'),
            data.valid
                ? 'Q ${(quality * 100).toStringAsFixed(0)}%'
                : 'Q —',
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ],
      ),
      const SizedBox(height: 10),
    ];

    if (!data.valid) {
      children.add(
        Text(
          key: const Key('plan-reason'),
          data.reason.isEmpty ? 'Plan not valid' : data.reason,
          style: const TextStyle(color: Colors.orangeAccent, fontSize: 13),
        ),
      );
    } else {
      children.addAll([
        _row('Entry', formatPrice(entry)),
        _row('Stop', formatPrice(stop)),
        _row('Target', formatPrice(target)),
        _row('R:R', data.riskReward.toStringAsFixed(2)),
        const SizedBox(height: 8),
        const Text(
          'Position in range',
          style: TextStyle(color: Colors.white54, fontSize: 11),
        ),
        const SizedBox(height: 4),
        ClipRRect(
          borderRadius: BorderRadius.circular(3),
          child: LinearProgressIndicator(
            key: const Key('plan-position-bar'),
            value: data.position.clamp(0.0, 1.0),
            minHeight: 8,
            backgroundColor: Colors.white12,
            valueColor: const AlwaysStoppedAnimation<Color>(Colors.tealAccent),
          ),
        ),
        const SizedBox(height: 10),
        Row(
          children: [
            const Text(
              'Risk (USDT)',
              style: TextStyle(color: Colors.white54, fontSize: 12),
            ),
            const SizedBox(width: 12),
            SizedBox(
              width: 88,
              height: 32,
              child: TextField(
                key: const Key('plan-risk-input'),
                controller: riskController,
                keyboardType:
                    const TextInputType.numberWithOptions(decimal: true),
                style: const TextStyle(color: Colors.white, fontSize: 13),
                decoration: const InputDecoration(
                  isDense: true,
                  contentPadding:
                      EdgeInsets.symmetric(horizontal: 8, vertical: 8),
                  border: OutlineInputBorder(),
                ),
                onSubmitted: _submitRisk,
              ),
            ),
            const Spacer(),
            Text(
              key: const Key('plan-size'),
              'Size ${size.toStringAsFixed(size >= 10 ? 2 : 4)}',
              style: const TextStyle(
                color: Colors.white70,
                fontWeight: FontWeight.w600,
                fontSize: 13,
              ),
            ),
          ],
        ),
      ]);
    }

    if (showRetry && onRetry != null) {
      children.add(const SizedBox(height: 8));
      children.add(
        TextButton(
          key: const Key('plan-retry'),
          onPressed: onRetry,
          child: const Text('Retry'),
        ),
      );
    }

    children.add(const SizedBox(height: 10));
    children.add(
      const Text(
        key: Key('plan-disclaimer'),
        "Levels derived from the last 110 bars' swing structure. "
        'Not financial advice.',
        style: TextStyle(color: Colors.white38, fontSize: 11),
      ),
    );

    return KeyedSubtree(
      key: const Key('plan-panel'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: children,
      ),
    );
  }

  void _submitRisk(String raw) {
    final parsed = double.tryParse(raw.trim());
    if (parsed == null || !parsed.isFinite || parsed < kPlanRiskMin) {
      riskController.text = formatPlanRisk(risk);
      return;
    }
    final clamped = parsed > kPlanRiskMax ? kPlanRiskMax : parsed;
    // Persist/size the same value the field shows (2 d.p. for fractions).
    final text = formatPlanRisk(clamped);
    final used = double.parse(text);
    riskController.text = text;
    onRiskChanged(used);
  }

  static Widget _row(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          SizedBox(
            width: 64,
            child: Text(
              label,
              style: const TextStyle(color: Colors.white54, fontSize: 12),
            ),
          ),
          Text(
            value,
            style: const TextStyle(color: Colors.white70, fontSize: 13),
          ),
        ],
      ),
    );
  }
}

/// Compact error row when the plan fetch failed (distinct from free-tier).
class PlanLoadError extends StatelessWidget {
  final VoidCallback onRetry;

  const PlanLoadError({Key? key, required this.onRetry}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return KeyedSubtree(
      key: const Key('plan-error'),
      child: Row(
        children: [
          const Expanded(
            child: Text(
              'Plan unavailable',
              style: TextStyle(color: Colors.white54, fontSize: 13),
            ),
          ),
          TextButton(
            key: const Key('plan-retry'),
            onPressed: onRetry,
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }
}
