import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import 'exchange_config.dart';
import 'trade_links.dart';

/// Represents the user's custom exchange entry.
class CustomExchange {
  final String name;
  final String urlTemplate;

  const CustomExchange({required this.name, required this.urlTemplate});

  /// Build URL by replacing 'BTC' in the template with [baseSymbol].
  Uri buildUrl(String baseSymbol) {
    final url = urlTemplate.replaceAll('BTC', baseSymbol.toUpperCase());
    return Uri.parse(url);
  }
}

/// Action buttons for "Open in TradingView" and "Trade in App".
///
/// Shows the preferred exchange button with an "…or choose another" link
/// that opens a full exchange picker bottom sheet.
class TradeActionButtons extends StatelessWidget {
  final String symbol;
  final String timeframe;
  final String preferredExchangeId;
  final List<ExchangeConfig> exchanges;
  final CustomExchange? customExchange;
  final ValueChanged<String>? onExchangeChanged;
  final VoidCallback? onAddCustom;
  final VoidCallback? onEditCustom;

  const TradeActionButtons({
    Key? key,
    required this.symbol,
    required this.timeframe,
    required this.preferredExchangeId,
    required this.exchanges,
    this.customExchange,
    this.onExchangeChanged,
    this.onAddCustom,
    this.onEditCustom,
  }) : super(key: key);

  ExchangeConfig? get _preferred =>
      exchanges.where((e) => e.id == preferredExchangeId).isNotEmpty
          ? exchanges.firstWhere((e) => e.id == preferredExchangeId)
          : exchanges.isNotEmpty
              ? exchanges.first
              : null;

  @override
  Widget build(BuildContext context) {
    final pref = _preferred;
    final base = extractBaseSymbol(symbol);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
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
            // ── Preferred exchange button ──
            Expanded(
              child: _ActionButton(
                icon: Icons.swap_horiz,
                label: pref?.name ?? 'Exchange',
                color: const Color(0xFF00E5FF),
                onTap: () {
                  if (pref != null) {
                    _openUrl(context, pref.buildUrl(base), pref.name);
                  }
                },
              ),
            ),
          ],
        ),
        // "…or choose another" link
        const SizedBox(height: 4),
        Align(
          alignment: Alignment.centerRight,
          child: GestureDetector(
            onTap: () => _showExchangeSheet(context),
            child: const Padding(
              padding: EdgeInsets.symmetric(vertical: 4),
              child: Text(
                '…or choose another',
                style: TextStyle(
                  color: Colors.white38,
                  fontSize: 11,
                  decoration: TextDecoration.underline,
                  decorationColor: Colors.white38,
                ),
              ),
            ),
          ),
        ),
        // Custom exchange button (if defined)
        if (customExchange != null) ...[
          const SizedBox(height: 4),
          _ActionButton(
            icon: Icons.open_in_new,
            label: customExchange!.name,
            color: const Color(0xFF00E6C0),
            onTap: () {
              _openUrl(
                  context, customExchange!.buildUrl(base), customExchange!.name);
            },
          ),
          Align(
            alignment: Alignment.centerRight,
            child: GestureDetector(
              onTap: onEditCustom,
              child: const Padding(
                padding: EdgeInsets.symmetric(vertical: 4),
                child: Text(
                  'edit',
                  style: TextStyle(
                    color: Colors.white38,
                    fontSize: 11,
                    decoration: TextDecoration.underline,
                    decorationColor: Colors.white38,
                  ),
                ),
              ),
            ),
          ),
        ],
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

  Future<void> _openUrl(BuildContext context, Uri uri, String name) async {
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Could not open $name')),
        );
      }
    }
  }

  void _showExchangeSheet(BuildContext context) {
    final base = extractBaseSymbol(symbol);
    showModalBottomSheet(
      context: context,
      backgroundColor: const Color(0xFF1A1A2E),
      isScrollControlled: true,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (_) => _ExchangePickerSheet(
        exchanges: exchanges,
        currentId: preferredExchangeId,
        baseSymbol: base,
        customExchange: customExchange,
        onSelected: (id) {
          Navigator.pop(context);
          onExchangeChanged?.call(id);
        },
        onOpenExchange: (uri, name) {
          Navigator.pop(context);
          _openUrl(context, uri, name);
        },
        onAddCustom: () {
          Navigator.pop(context);
          onAddCustom?.call();
        },
      ),
    );
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

/// Bottom sheet with all exchanges, preferred marker, and "Add your own".
class _ExchangePickerSheet extends StatelessWidget {
  final List<ExchangeConfig> exchanges;
  final String currentId;
  final String baseSymbol;
  final CustomExchange? customExchange;
  final ValueChanged<String> onSelected;
  final void Function(Uri uri, String name) onOpenExchange;
  final VoidCallback onAddCustom;

  const _ExchangePickerSheet({
    required this.exchanges,
    required this.currentId,
    required this.baseSymbol,
    this.customExchange,
    required this.onSelected,
    required this.onOpenExchange,
    required this.onAddCustom,
  });

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Drag handle
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
                'Choose Exchange',
                style: TextStyle(
                    color: Colors.white,
                    fontSize: 16,
                    fontWeight: FontWeight.w600),
              ),
            ),
            const SizedBox(height: 12),
            // Built-in exchanges
            for (final ex in exchanges)
              ListTile(
                leading: Icon(
                  ex.id == currentId
                      ? Icons.radio_button_checked
                      : Icons.radio_button_off,
                  color: ex.id == currentId
                      ? const Color(0xFF00E5FF)
                      : Colors.white38,
                  size: 20,
                ),
                title: Text(
                  ex.name,
                  style: TextStyle(
                    color: ex.id == currentId ? Colors.white : Colors.white70,
                    fontSize: 14,
                  ),
                ),
                trailing: GestureDetector(
                  onTap: () =>
                      onOpenExchange(ex.buildUrl(baseSymbol), ex.name),
                  child: const Icon(Icons.open_in_new,
                      size: 16, color: Colors.white38),
                ),
                onTap: () => onSelected(ex.id),
                dense: true,
                contentPadding: EdgeInsets.zero,
              ),
            // Custom exchange entry if present
            if (customExchange != null) ...[
              Divider(color: Colors.white.withAlpha(25)),
              ListTile(
                leading: const Icon(Icons.person, size: 20, color: Color(0xFF00E6C0)),
                title: Text(
                  customExchange!.name,
                  style: const TextStyle(color: Colors.white, fontSize: 14),
                ),
                trailing: GestureDetector(
                  onTap: () => onOpenExchange(
                      customExchange!.buildUrl(baseSymbol), customExchange!.name),
                  child: const Icon(Icons.open_in_new,
                      size: 16, color: Colors.white38),
                ),
                onTap: () => onOpenExchange(
                    customExchange!.buildUrl(baseSymbol), customExchange!.name),
                dense: true,
                contentPadding: EdgeInsets.zero,
              ),
            ],
            Divider(color: Colors.white.withAlpha(25)),
            // "Add your own" button
            ListTile(
              leading: const Icon(Icons.add_circle_outline,
                  size: 20, color: Colors.white54),
              title: Text(
                customExchange != null
                    ? 'Edit your custom exchange'
                    : 'Add your own (url redirect)',
                style: const TextStyle(color: Colors.white54, fontSize: 14),
              ),
              onTap: onAddCustom,
              dense: true,
              contentPadding: EdgeInsets.zero,
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}

/// Full-screen dialog for adding/editing a custom exchange.
///
/// Returns a [CustomExchange] if the user saves, or null if cancelled.
Future<CustomExchange?> showCustomExchangeEditor(
  BuildContext context, {
  CustomExchange? existing,
}) {
  return showDialog<CustomExchange>(
    context: context,
    builder: (_) => _CustomExchangeDialog(existing: existing),
  );
}

class _CustomExchangeDialog extends StatefulWidget {
  final CustomExchange? existing;
  const _CustomExchangeDialog({this.existing});

  @override
  State<_CustomExchangeDialog> createState() => _CustomExchangeDialogState();
}

class _CustomExchangeDialogState extends State<_CustomExchangeDialog> {
  late final TextEditingController _nameCtrl;
  late final TextEditingController _urlCtrl;

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController(text: widget.existing?.name ?? '');
    _urlCtrl = TextEditingController(
      text: widget.existing?.urlTemplate ??
          'https://your-own.dex.com/trade/BTC_USDT',
    );
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _urlCtrl.dispose();
    super.dispose();
  }

  void _save() {
    final name = _nameCtrl.text.trim();
    final url = _urlCtrl.text.trim();
    if (name.isEmpty || url.isEmpty) return;
    Navigator.of(context).pop(CustomExchange(name: name, urlTemplate: url));
  }

  @override
  Widget build(BuildContext context) {
    final isEditing = widget.existing != null;
    return AlertDialog(
      backgroundColor: const Color(0xFF1A1A2E),
      title: Text(
        isEditing ? 'Edit Custom Exchange' : 'Add Custom Exchange',
        style: const TextStyle(color: Colors.white, fontSize: 16),
      ),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: _nameCtrl,
              style: const TextStyle(color: Colors.white, fontSize: 14),
              decoration: const InputDecoration(
                labelText: 'Name',
                labelStyle: TextStyle(color: Colors.white54),
                enabledBorder: UnderlineInputBorder(
                  borderSide: BorderSide(color: Colors.white24),
                ),
                focusedBorder: UnderlineInputBorder(
                  borderSide: BorderSide(color: Color(0xFF00E6C0)),
                ),
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _urlCtrl,
              style: const TextStyle(color: Colors.white, fontSize: 14),
              decoration: const InputDecoration(
                labelText: 'URL',
                labelStyle: TextStyle(color: Colors.white54),
                enabledBorder: UnderlineInputBorder(
                  borderSide: BorderSide(color: Colors.white24),
                ),
                focusedBorder: UnderlineInputBorder(
                  borderSide: BorderSide(color: Color(0xFF00E6C0)),
                ),
              ),
              keyboardType: TextInputType.url,
            ),
            const SizedBox(height: 12),
            const Text(
              'Provide the URL of the exchange you know and would like '
              'to trade on. The BTC_USDT symbol is a placeholder. '
              'Keep the BTC part, edit the rest of the URL to fit your needs.',
              style: TextStyle(color: Colors.white38, fontSize: 11),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(null),
          child: const Text('Cancel',
              style: TextStyle(color: Colors.white54)),
        ),
        TextButton(
          onPressed: _save,
          child: const Text('Save',
              style: TextStyle(color: Color(0xFF00E6C0))),
        ),
      ],
    );
  }
}
