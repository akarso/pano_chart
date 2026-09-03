import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import 'fear_greed_data.dart';
import 'http_fear_greed_api.dart';

/// Shows a dialog with the current Fear & Greed Index.
///
/// Fetches from [api], shows a loading spinner while waiting, then displays
/// the value, classification, and UTC timestamp.
Future<void> showFearGreedDialog(BuildContext context, FearGreedApi api) async {
  showDialog(
    context: context,
    barrierDismissible: false,
    builder: (_) => const Center(child: CircularProgressIndicator()),
  );

  try {
    final data = await api.fetch();
    if (!context.mounted) return;
    Navigator.of(context).pop(); // dismiss spinner

    showDialog(
      context: context,
      builder: (_) => _FearGreedDialog(data: data),
    );
  } catch (e) {
    if (!context.mounted) return;
    Navigator.of(context).pop(); // dismiss spinner
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Fear & Greed Index'),
        content: Text('Failed to load: $e'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }
}

class _FearGreedDialog extends StatelessWidget {
  final FearGreedData data;
  const _FearGreedDialog({required this.data});

  Color _gaugeColor(int value) {
    if (value <= 25) return Colors.red;
    if (value <= 45) return Colors.orange;
    if (value <= 55) return Colors.grey;
    if (value <= 75) return Colors.lightGreen;
    return Colors.green;
  }

  @override
  Widget build(BuildContext context) {
    final color = _gaugeColor(data.value);
    final ts = data.timestampUtc;
    final dateStr =
        '${ts.year}-${ts.month.toString().padLeft(2, '0')}-${ts.day.toString().padLeft(2, '0')} '
        '${ts.hour.toString().padLeft(2, '0')}:${ts.minute.toString().padLeft(2, '0')} UTC';

    return AlertDialog(
      title: const Text('Fear & Greed Index'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            '${data.value}',
            style: TextStyle(
              fontSize: 48,
              fontWeight: FontWeight.bold,
              color: color,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            data.classification,
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w600,
              color: color,
            ),
          ),
          const SizedBox(height: 16),
          ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: data.value / 100,
              backgroundColor: Colors.white12,
              valueColor: AlwaysStoppedAnimation<Color>(color),
              minHeight: 10,
            ),
          ),
          const SizedBox(height: 16),
          Text(
            dateStr,
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 12),
          GestureDetector(
            onTap: () => launchUrl(
              Uri.parse('https://alternative.me/crypto/fear-and-greed-index/'),
              mode: LaunchMode.externalApplication,
            ),
            child: const Text(
              'Source: alternative.me',
              style: TextStyle(
                color: Colors.grey,
                fontSize: 10,
                decoration: TextDecoration.underline,
              ),
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('OK'),
        ),
      ],
    );
  }
}
