import 'package:flutter/material.dart';
import 'market_state_data.dart';
import 'http_market_state_api.dart';

/// Shows a dialog with the current market state and breadth.
Future<void> showMarketStateDialog(
  BuildContext context,
  MarketStateApi api, {
  String timeframe = '4h',
}) async {
  showDialog(
    context: context,
    barrierDismissible: false,
    builder: (_) => const Center(child: CircularProgressIndicator()),
  );

  try {
    final data = await api.fetch(timeframe: timeframe);
    if (!context.mounted) return;
    Navigator.of(context).pop(); // dismiss spinner

    showDialog(
      context: context,
      builder: (_) => _MarketStateDialog(data: data),
    );
  } catch (e) {
    if (!context.mounted) return;
    Navigator.of(context).pop(); // dismiss spinner
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Market State'),
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

class _MarketStateDialog extends StatelessWidget {
  final MarketStateData data;
  const _MarketStateDialog({required this.data});

  Color _stateColor(String state, [String bias = 'neutral']) {
    switch (state) {
      case 'sideways':
        return Colors.blueGrey;
      case 'compression':
        return Colors.amber;
      case 'breakout':
        return Colors.redAccent;
      case 'trend':
        return bias == 'down' ? Colors.redAccent : Colors.tealAccent;
      default:
        return Colors.grey;
    }
  }

  IconData _stateIcon(String state, [String bias = 'neutral']) {
    switch (state) {
      case 'sideways':
        return Icons.swap_horiz;
      case 'compression':
        return Icons.compress;
      case 'breakout':
        return Icons.open_in_full;
      case 'trend':
        return bias == 'down' ? Icons.trending_down : Icons.trending_up;
      default:
        return Icons.help_outline;
    }
  }

  @override
  Widget build(BuildContext context) {
    final color = _stateColor(data.state, data.bias);
    final pct = (data.confidence * 100).toStringAsFixed(1);

    return AlertDialog(
      title: Row(
        children: [
          Icon(_stateIcon(data.state, data.bias), color: color, size: 28),
          const SizedBox(width: 8),
          const Text('Market State'),
        ],
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            data.state.toUpperCase(),
            style: TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.bold,
              color: color,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            '$pct% confidence  •  ${data.symbolCount} symbols',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 4),
          Text(
            data.timeframe,
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 16),
          _BreadthBar(label: 'Sideways', value: data.breadth.sideways, color: Colors.blueGrey),
          const SizedBox(height: 6),
          _BreadthBar(label: 'Compression', value: data.breadth.compression, color: Colors.amber),
          const SizedBox(height: 6),
          _BreadthBar(label: 'Breakout', value: data.breadth.breakout, color: Colors.redAccent),
          const SizedBox(height: 6),
          _BreadthBar(label: 'Trend', value: data.breadth.trend, color: Colors.tealAccent),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('CLOSE'),
        ),
      ],
    );
  }
}

class _BreadthBar extends StatelessWidget {
  final String label;
  final double value;
  final Color color;

  const _BreadthBar({
    required this.label,
    required this.value,
    required this.color,
  });

  @override
  Widget build(BuildContext context) {
    final pct = (value * 100).toStringAsFixed(1);
    return Row(
      children: [
        SizedBox(
          width: 90,
          child: Text(label, style: const TextStyle(fontSize: 13)),
        ),
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: value,
              backgroundColor: Colors.white12,
              valueColor: AlwaysStoppedAnimation<Color>(color),
              minHeight: 8,
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 44,
          child: Text(
            '$pct%',
            style: const TextStyle(fontSize: 12, color: Colors.grey),
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }
}
