import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import 'trade_links.dart';

/// Action buttons for "Open in TradingView" and "Trade in App".
///
/// Pure deep-linking — no permissions, no API keys, no wallet access.
class TradeActionButtons extends StatelessWidget {
  final String symbol;
  final String timeframe;
  final Exchange exchange;
  final ValueChanged<Exchange>? onExchangeChanged;

  const TradeActionButtons({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.exchange,
    this.onExchangeChanged,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        // ── TradingView button ──
        Expanded(
          child: _ActionButton(
            icon: Icons.show_chart,
            label: 'TradingView',
            color: const Color(0xFF2962FF),
            onTap: () => _openTradingView(context),
          ),
        ),
        const SizedBox(width: 10),
        // ── Trade in App button ──
        Expanded(
          child: GestureDetector(
            onLongPress: () => _pickExchange(context),
            child: _ActionButton(
              icon: Icons.swap_horiz,
              label: exchange.label,
              color: const Color(0xFF00E5FF),
              onTap: () => _openExchange(context),
            ),
          ),
        ),
      ],
    );
  }

  Future<void> _openTradingView(BuildContext context) async {
    final uri = TradeLinkBuilder.tradingView(symbol, timeframe);
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Could not open TradingView')),
        );
      }
    }
  }

  Future<void> _openExchange(BuildContext context) async {
    final uri = TradeLinkBuilder.exchange(symbol, exchange);
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              content:
                  Text('Symbol not supported on ${exchange.label}')),
        );
      }
    }
  }

  Future<void> _pickExchange(BuildContext context) async {
    final selected = await showExchangePicker(context, exchange);
    if (selected != null && selected != exchange) {
      onExchangeChanged?.call(selected);
    }
  }
}

/// Compact action button with icon + label.
class _ActionButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final Color color;
  final VoidCallback onTap;

  const _ActionButton({
    required this.icon,
    required this.label,
    required this.color,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(
            border: Border.all(color: color.withOpacity(0.5), width: 1),
            borderRadius: BorderRadius.circular(8),
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(icon, size: 16, color: color),
              const SizedBox(width: 6),
              Text(
                label,
                style: TextStyle(
                  color: color,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Bottom sheet for selecting preferred exchange.
Future<Exchange?> showExchangePicker(
    BuildContext context, Exchange current) {
  return showModalBottomSheet<Exchange>(
    context: context,
    backgroundColor: const Color(0xFF1A1A2E),
    shape: const RoundedRectangleBorder(
      borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
    ),
    builder: (_) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 36,
              height: 4,
              margin: const EdgeInsets.only(bottom: 16),
              decoration: BoxDecoration(
                color: Colors.white24,
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const Align(
              alignment: Alignment.centerLeft,
              child: Text(
                'Preferred Exchange',
                style: TextStyle(
                    color: Colors.white,
                    fontSize: 16,
                    fontWeight: FontWeight.w600),
              ),
            ),
            const SizedBox(height: 12),
            for (final ex in Exchange.values)
              ListTile(
                leading: Icon(
                  ex == current ? Icons.radio_button_checked : Icons.radio_button_off,
                  color: ex == current
                      ? const Color(0xFF00E5FF)
                      : Colors.white38,
                  size: 20,
                ),
                title: Text(
                  ex.label,
                  style: TextStyle(
                    color: ex == current ? Colors.white : Colors.white70,
                    fontSize: 14,
                  ),
                ),
                onTap: () => Navigator.pop(context, ex),
                dense: true,
                contentPadding: EdgeInsets.zero,
              ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    ),
  );
}
