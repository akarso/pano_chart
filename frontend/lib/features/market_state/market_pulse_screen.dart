import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'composite_index_data.dart';
import 'http_composite_index_api.dart';
import 'http_market_state_api.dart';
import 'market_state_data.dart';

/// Full-page Market Pulse screen showing market state, breadth, and
/// composite index chart. Designed for extensibility with future stats.
class MarketPulseScreen extends StatefulWidget {
  final MarketStateApi marketStateApi;
  final CompositeIndexApi compositeIndexApi;

  const MarketPulseScreen({
    Key? key,
    required this.marketStateApi,
    required this.compositeIndexApi,
  }) : super(key: key);

  @override
  State<MarketPulseScreen> createState() => _MarketPulseScreenState();
}

class _MarketPulseScreenState extends State<MarketPulseScreen> {
  MarketStateData? _stateData;
  CompositeIndexData? _compositeData;
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  Future<void> _loadAll() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        widget.marketStateApi.fetch(timeframe: '4h'),
        widget.compositeIndexApi.fetch(timeframe: '4h', limit: 200),
      ]);
      if (!mounted) return;
      setState(() {
        _stateData = results[0] as MarketStateData;
        _compositeData = results[1] as CompositeIndexData;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF0D0D0D),
      appBar: AppBar(
        backgroundColor: const Color(0xFF0D0D0D),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_ios_new, size: 20),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: const Text('Market Pulse'),
        centerTitle: true,
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, color: Colors.redAccent, size: 48),
              const SizedBox(height: 12),
              const Text(
                'Failed to load market data',
                style: TextStyle(color: Colors.white70, fontSize: 16),
              ),
              const SizedBox(height: 8),
              Text(
                _error!,
                style: const TextStyle(color: Colors.grey, fontSize: 12),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 16),
              TextButton(
                onPressed: _loadAll,
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: _loadAll,
      child: ListView(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        children: [
          if (_stateData != null) _buildStateCard(_stateData!),
          const SizedBox(height: 16),
          if (_compositeData != null) _buildCompositeCard(_compositeData!),
          const SizedBox(height: 16),
          if (_stateData != null) _buildBreadthCard(_stateData!),
          const SizedBox(height: 32),
        ],
      ),
    );
  }

  // ---------- Market State Card ----------

  Widget _buildStateCard(MarketStateData data) {
    final color = _stateColor(data.state);
    final pct = (data.confidence * 100).toStringAsFixed(1);

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF1A1A1A),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(_stateIcon(data.state), color: color, size: 28),
              const SizedBox(width: 8),
              Text(
                data.state.toUpperCase(),
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                  color: color,
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '$pct% confidence  •  ${data.symbolCount} symbols  •  ${data.timeframe}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
        ],
      ),
    );
  }

  // ---------- Composite Index Chart Card ----------

  Widget _buildCompositeCard(CompositeIndexData data) {
    final hasPoints = data.points.isNotEmpty;
    final change = hasPoints && data.points.length > 1
        ? data.points.last.value - data.points.first.value
        : 0.0;
    final changeStr = change >= 0
        ? '+${change.toStringAsFixed(2)}'
        : change.toStringAsFixed(2);
    final changeColor = change >= 0 ? Colors.greenAccent : Colors.redAccent;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF1A1A1A),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'Market Composite Index',
                style: TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              Text(
                '$changeStr%',
                style: TextStyle(color: changeColor, fontSize: 14),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '${data.symbolCount} symbols  •  base 100',
            style: const TextStyle(color: Colors.grey, fontSize: 11),
          ),
          const SizedBox(height: 12),
          SizedBox(
            height: 180,
            child: hasPoints
                ? CustomPaint(
                    size: Size.infinite,
                    painter: _CompositeChartPainter(
                      points: data.points,
                      lineColor: changeColor,
                    ),
                  )
                : const Center(
                    child: Text(
                      'No data',
                      style: TextStyle(color: Colors.grey),
                    ),
                  ),
          ),
        ],
      ),
    );
  }

  // ---------- Breadth Card ----------

  Widget _buildBreadthCard(MarketStateData data) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF1A1A1A),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Market Breadth',
            style: TextStyle(
              color: Colors.white70,
              fontSize: 14,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 12),
          _breadthRow('Sideways', data.breadth.sideways, Colors.blueGrey),
          const SizedBox(height: 6),
          _breadthRow('Compression', data.breadth.compression, Colors.amber),
          const SizedBox(height: 6),
          _breadthRow('Breakout', data.breadth.breakout, Colors.redAccent),
          const SizedBox(height: 6),
          _breadthRow('Trend', data.breadth.trend, Colors.tealAccent),
        ],
      ),
    );
  }

  Widget _breadthRow(String label, double value, Color color) {
    final pct = (value * 100).toStringAsFixed(1);
    return Row(
      children: [
        SizedBox(
          width: 90,
          child: Text(label, style: const TextStyle(fontSize: 13, color: Colors.white70)),
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

  // ---------- Helpers ----------

  Color _stateColor(String state) {
    switch (state) {
      case 'sideways':
        return Colors.blueGrey;
      case 'compression':
        return Colors.amber;
      case 'breakout':
        return Colors.redAccent;
      case 'trend':
        return Colors.tealAccent;
      default:
        return Colors.grey;
    }
  }

  IconData _stateIcon(String state) {
    switch (state) {
      case 'sideways':
        return Icons.swap_horiz;
      case 'compression':
        return Icons.compress;
      case 'breakout':
        return Icons.open_in_full;
      case 'trend':
        return Icons.trending_up;
      default:
        return Icons.help_outline;
    }
  }
}

// ---------- Chart Painter ----------

class _CompositeChartPainter extends CustomPainter {
  final List<IndexPoint> points;
  final Color lineColor;

  _CompositeChartPainter({required this.points, required this.lineColor});

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2) return;

    final values = points.map((p) => p.value).toList();
    final minV = values.reduce(math.min);
    final maxV = values.reduce(math.max);
    final range = maxV - minV;
    if (range == 0) return;

    // Draw baseline at 100
    final baseY = size.height - ((100 - minV) / range) * size.height;
    final basePaint = Paint()
      ..color = Colors.white24
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;

    if (baseY >= 0 && baseY <= size.height) {
      canvas.drawLine(Offset(0, baseY), Offset(size.width, baseY), basePaint);
    }

    // Draw line
    final linePaint = Paint()
      ..color = lineColor
      ..strokeWidth = 2
      ..style = PaintingStyle.stroke
      ..strokeJoin = StrokeJoin.round;

    final path = Path();
    for (var i = 0; i < points.length; i++) {
      final x = (i / (points.length - 1)) * size.width;
      final y = size.height - ((values[i] - minV) / range) * size.height;
      if (i == 0) {
        path.moveTo(x, y);
      } else {
        path.lineTo(x, y);
      }
    }
    canvas.drawPath(path, linePaint);

    // Draw gradient fill
    final fillPath = Path.from(path)
      ..lineTo(size.width, size.height)
      ..lineTo(0, size.height)
      ..close();

    final fillPaint = Paint()
      ..shader = LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [lineColor.withAlpha(60), lineColor.withAlpha(0)],
      ).createShader(Rect.fromLTWH(0, 0, size.width, size.height));

    canvas.drawPath(fillPath, fillPaint);
  }

  @override
  bool shouldRepaint(covariant _CompositeChartPainter other) {
    return other.points != points || other.lineColor != lineColor;
  }
}
