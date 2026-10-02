import 'package:flutter/material.dart';

import 'replay_asof.dart';
import 'replay_controller.dart';

/// Persistent slim banner while replay is active (PR-112b).
class ReplayBanner extends StatelessWidget {
  final DateTime asOf;
  final VoidCallback onExit;
  final String? footnote;

  const ReplayBanner({
    super.key,
    required this.asOf,
    required this.onExit,
    this.footnote,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      key: const Key('replay-banner'),
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      color: const Color(0xFF1A3A5C).withAlpha(220),
      child: Row(
        children: [
          const Icon(Icons.history, color: Colors.white70, size: 14),
          const SizedBox(width: 6),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  formatReplayBanner(asOf),
                  style: const TextStyle(
                    color: Colors.white70,
                    fontSize: 11,
                    fontWeight: FontWeight.w500,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
                if (footnote != null)
                  Text(
                    footnote!,
                    key: const Key('replay-banner-footnote'),
                    style: const TextStyle(
                      color: Colors.white54,
                      fontSize: 10,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
              ],
            ),
          ),
          TextButton(
            key: const Key('replay-exit'),
            onPressed: onExit,
            style: TextButton.styleFrom(
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(horizontal: 8),
              minimumSize: const Size(0, 28),
              tapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
            child: const Text('Exit', style: TextStyle(fontSize: 11)),
          ),
        ],
      ),
    );
  }
}

/// Bottom scrubber: step bars/days + UTC date picker (PR-112b).
class ReplayScrubber extends StatelessWidget {
  final ReplayController controller;
  final String timeframe;

  const ReplayScrubber({
    super.key,
    required this.controller,
    required this.timeframe,
  });

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final asOf = controller.asOfFor(timeframe);
        if (asOf == null) return const SizedBox.shrink();

        return Material(
          key: const Key('replay-scrubber'),
          color: const Color(0xFF121212),
          elevation: 8,
          child: SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(8, 6, 8, 6),
              child: Row(
                children: [
                  _step(context, key: 'replay-step-bar-minus', label: '−1b',
                      onTap: () => controller.stepBars(timeframe, -1)),
                  _step(context, key: 'replay-step-day-minus', label: '−1d',
                      onTap: () => controller.stepDays(timeframe, -1)),
                  Expanded(
                    child: InkWell(
                      key: const Key('replay-date-picker'),
                      onTap: () => _pickDate(context, asOf),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(vertical: 8),
                        child: Text(
                          formatReplayBanner(asOf).replaceFirst('Replay: ', ''),
                          textAlign: TextAlign.center,
                          style: const TextStyle(
                            color: Colors.white70,
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    ),
                  ),
                  _step(context, key: 'replay-step-day-plus', label: '+1d',
                      onTap: () => controller.stepDays(timeframe, 1)),
                  _step(context, key: 'replay-step-bar-plus', label: '+1b',
                      onTap: () => controller.stepBars(timeframe, 1)),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  Widget _step(
    BuildContext context, {
    required String key,
    required String label,
    required VoidCallback onTap,
  }) {
    return TextButton(
      key: Key(key),
      onPressed: onTap,
      style: TextButton.styleFrom(
        foregroundColor: Colors.white70,
        padding: const EdgeInsets.symmetric(horizontal: 6),
        minimumSize: const Size(36, 32),
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
      child: Text(label, style: const TextStyle(fontSize: 11)),
    );
  }

  /// Date picker uses **UTC calendar days** so the chosen day matches the
  /// banner's UTC label (not the device's local midnight).
  Future<void> _pickDate(BuildContext context, DateTime asOf) async {
    final utc = asOf.toUtc();
    final initial = DateTime(utc.year, utc.month, utc.day);
    final first = DateTime(2018);
    final lastUtc = DateTime.now().toUtc();
    final last = DateTime(lastUtc.year, lastUtc.month, lastUtc.day);
    final picked = await showDatePicker(
      context: context,
      initialDate: initial.isBefore(first)
          ? first
          : (initial.isAfter(last) ? last : initial),
      firstDate: first,
      lastDate: last,
      helpText: 'Select UTC date',
    );
    if (picked == null) return;
    final withTime = DateTime.utc(
      picked.year,
      picked.month,
      picked.day,
      utc.hour,
      utc.minute,
    );
    controller.setAsOf(timeframe, withTime);
  }
}
