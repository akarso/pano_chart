import 'package:flutter/material.dart';

import 'behavior_oscillator_painter.dart';
import 'chart_config.dart';

/// Shows a bottom sheet letting the user toggle and configure
/// chart indicators. Returns the updated config on dismiss.
Future<ChartIndicatorConfig?> showIndicatorPanel(
  BuildContext context,
  ChartIndicatorConfig current,
) {
  return showModalBottomSheet<ChartIndicatorConfig>(
    context: context,
    backgroundColor: const Color(0xFF1A1A2E),
    isScrollControlled: true,
    shape: const RoundedRectangleBorder(
      borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
    ),
    builder: (_) => _IndicatorPanelBody(initial: current),
  );
}

class _IndicatorPanelBody extends StatefulWidget {
  final ChartIndicatorConfig initial;
  const _IndicatorPanelBody({required this.initial});

  @override
  State<_IndicatorPanelBody> createState() => _IndicatorPanelBodyState();
}

class _IndicatorPanelBodyState extends State<_IndicatorPanelBody> {
  late ChartIndicatorConfig _cfg;

  @override
  void initState() {
    super.initState();
    _cfg = widget.initial;
  }

  void _update(ChartIndicatorConfig c) => setState(() => _cfg = c);

  @override
  Widget build(BuildContext context) {
    // Cap height at 85% of screen so the sheet never hides behind the keyboard
    // or overflows on smaller devices.
    final maxH = MediaQuery.sizeOf(context).height * 0.85;
    return ConstrainedBox(
      constraints: BoxConstraints(maxHeight: maxH),
      child: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
            // drag handle
            Container(
              width: 36,
              height: 4,
              margin: const EdgeInsets.only(bottom: 10),
              decoration: BoxDecoration(
                color: Colors.white24,
                borderRadius: BorderRadius.circular(2),
              ),
            ),

            const Align(
              alignment: Alignment.centerLeft,
              child: Text(
                'Indicators',
                style: TextStyle(
                  color: Colors.white,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            const SizedBox(height: 8),

            // ── EMA Fast ──
            _IndicatorRow(
              label: 'EMA Fast',
              color: const Color(0xFF42A5F5),
              enabled: _cfg.showEmaFast,
              period: _cfg.emaFastPeriod,
              minPeriod: 5,
              maxPeriod: 50,
              onToggle: (v) => _update(_cfg.copyWith(showEmaFast: v)),
              onPeriod: (v) => _update(_cfg.copyWith(emaFastPeriod: v)),
            ),

            // ── EMA Slow ──
            _IndicatorRow(
              label: 'EMA Slow',
              color: const Color(0xFFFFA726),
              enabled: _cfg.showEmaSlow,
              period: _cfg.emaSlowPeriod,
              minPeriod: 10,
              maxPeriod: 200,
              onToggle: (v) => _update(_cfg.copyWith(showEmaSlow: v)),
              onPeriod: (v) => _update(_cfg.copyWith(emaSlowPeriod: v)),
            ),

            // ── RSI ──
            _IndicatorRow(
              label: 'RSI',
              color: const Color(0xFFAB47BC),
              enabled: _cfg.showRsi,
              period: _cfg.rsiPeriod,
              minPeriod: 5,
              maxPeriod: 30,
              onToggle: (v) => _update(_cfg.copyWith(showRsi: v)),
              onPeriod: (v) => _update(_cfg.copyWith(rsiPeriod: v)),
            ),

            // ── ATR ──
            _IndicatorRow(
              label: 'ATR',
              color: const Color(0xFF26A69A),
              enabled: _cfg.showAtr,
              period: _cfg.atrPeriod,
              minPeriod: 5,
              maxPeriod: 30,
              onToggle: (v) => _update(_cfg.copyWith(showAtr: v)),
              onPeriod: (v) => _update(_cfg.copyWith(atrPeriod: v)),
            ),

            const SizedBox(height: 10),
            const Divider(color: Colors.white12, height: 1),
            const SizedBox(height: 8),

            // ── Behavioral Indicators section ──
            Row(
              children: [
                const Expanded(
                  child: Text(
                    'Behavioral Indicators',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
                SizedBox(
                  width: 40,
                  child: Switch(
                    value: _cfg.showBehaviorPanel,
                    onChanged: (v) =>
                        _update(_cfg.copyWith(showBehaviorPanel: v)),
                    activeColor: const Color(0xFF4DD0E1),
                    materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),

            // ── Greed ──
            _IndicatorRow(
              label: 'Greed',
              color: BehaviorOscillatorPainter.greedColor,
              enabled: _cfg.showBehaviorPanel && _cfg.showGreed,
              period: _cfg.behaviorWindow,
              minPeriod: 5,
              maxPeriod: 50,
              onToggle: (v) => _update(_cfg.copyWith(showGreed: v)),
              onPeriod: (v) => _update(_cfg.copyWith(behaviorWindow: v)),
            ),

            // ── Fear ──
            _IndicatorRow(
              label: 'Fear',
              color: BehaviorOscillatorPainter.fearColor,
              enabled: _cfg.showBehaviorPanel && _cfg.showFear,
              period: _cfg.behaviorWindow,
              minPeriod: 5,
              maxPeriod: 50,
              onToggle: (v) => _update(_cfg.copyWith(showFear: v)),
              onPeriod: (v) => _update(_cfg.copyWith(behaviorWindow: v)),
            ),

            // ── Patience ──
            _IndicatorRow(
              label: 'Patience',
              color: BehaviorOscillatorPainter.patienceColor,
              enabled: _cfg.showBehaviorPanel && _cfg.showPatience,
              period: _cfg.behaviorWindow,
              minPeriod: 5,
              maxPeriod: 50,
              onToggle: (v) => _update(_cfg.copyWith(showPatience: v)),
              onPeriod: (v) => _update(_cfg.copyWith(behaviorWindow: v)),
            ),

            // ── Panic ──
            _IndicatorRow(
              label: 'Panic',
              color: BehaviorOscillatorPainter.panicColor,
              enabled: _cfg.showBehaviorPanel && _cfg.showPanic,
              period: _cfg.behaviorWindow,
              minPeriod: 5,
              maxPeriod: 50,
              onToggle: (v) => _update(_cfg.copyWith(showPanic: v)),
              onPeriod: (v) => _update(_cfg.copyWith(behaviorWindow: v)),
            ),

            const SizedBox(height: 10),
            const Divider(color: Colors.white12, height: 1),
            const SizedBox(height: 8),

            // ── Intraday Activity ──
            _IndicatorRow(
              label: 'Activity',
              color: const Color(0xFF66BB6A),
              enabled: _cfg.showVolatility,
              period: 0,
              minPeriod: 0,
              maxPeriod: 0,
              onToggle: (v) => _update(_cfg.copyWith(showVolatility: v)),
              onPeriod: (_) {},
            ),

            const SizedBox(height: 10),

            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFF00E5FF),
                  foregroundColor: Colors.black,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(8),
                  ),
                ),
                onPressed: () => Navigator.pop(context, _cfg),
                child: const Text('Apply'),
              ),
            ),
          ],
        ),
      ),
      ),
    );
  }
}

class _IndicatorRow extends StatelessWidget {
  final String label;
  final Color color;
  final bool enabled;
  final int period;
  final int minPeriod;
  final int maxPeriod;
  final ValueChanged<bool> onToggle;
  final ValueChanged<int> onPeriod;

  const _IndicatorRow({
    required this.label,
    required this.color,
    required this.enabled,
    required this.period,
    required this.minPeriod,
    required this.maxPeriod,
    required this.onToggle,
    required this.onPeriod,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        children: [
          // Toggle
          SizedBox(
            width: 40,
            child: Switch(
              value: enabled,
              onChanged: onToggle,
              activeColor: color,
              materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
          ),
          const SizedBox(width: 8),
          // Colour dot + label
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
          const SizedBox(width: 6),
          SizedBox(
            width: 72,
            child: Text(
              label,
              style: TextStyle(
                color: enabled ? Colors.white : Colors.white38,
                fontSize: 13,
              ),
            ),
          ),
          // Period slider (hidden when min == max, e.g. Activity row)
          if (maxPeriod > minPeriod) ...[
            Expanded(
              child: Slider(
                value: period.toDouble(),
                min: minPeriod.toDouble(),
                max: maxPeriod.toDouble(),
                divisions: maxPeriod - minPeriod,
                label: '$period',
                activeColor: color.withOpacity(enabled ? 1.0 : 0.3),
                inactiveColor: Colors.white12,
                onChanged: enabled
                    ? (v) => onPeriod(v.round())
                    : null,
              ),
            ),
            // Period value
            SizedBox(
              width: 28,
              child: Text(
                '$period',
                textAlign: TextAlign.right,
                style: TextStyle(
                  color: enabled ? Colors.white70 : Colors.white24,
                  fontSize: 12,
                ),
              ),
            ),
          ],
          if (maxPeriod <= minPeriod) const Spacer(),
        ],
      ),
    );
  }
}
