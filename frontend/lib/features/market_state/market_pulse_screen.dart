import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'composite_index_data.dart';
import 'http_composite_index_api.dart';
import 'http_market_state_api.dart';
import 'http_regime_api.dart';
import 'http_transition_api.dart';
import 'market_state_data.dart';
import 'regime_data.dart';
import 'transition_data.dart';

/// Full-page Market Pulse screen showing market state, breadth, and
/// composite index chart. Designed for extensibility with future stats.
class MarketPulseScreen extends StatefulWidget {
  final MarketStateApi marketStateApi;
  final CompositeIndexApi compositeIndexApi;
  final RegimeApi? regimeApi;
  final TransitionApi? transitionApi;

  const MarketPulseScreen({
    Key? key,
    required this.marketStateApi,
    required this.compositeIndexApi,
    this.regimeApi,
    this.transitionApi,
  }) : super(key: key);

  @override
  State<MarketPulseScreen> createState() => _MarketPulseScreenState();
}

class _MarketPulseScreenState extends State<MarketPulseScreen> {
  MarketStateData? _stateData;
  CompositeIndexData? _compositeData;
  RegimeData? _regimeData;
  TransitionData? _transitionData;
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
      final futures = <Future>[
        widget.marketStateApi.fetch(timeframe: '4h'),
        widget.compositeIndexApi.fetch(timeframe: '4h', limit: 200),
        if (widget.regimeApi != null)
          widget.regimeApi!.fetch(timeframe: '4h'),
        if (widget.transitionApi != null)
          widget.transitionApi!.fetch(timeframe: '4h'),
      ];
      final results = await Future.wait(futures);
      if (!mounted) return;
      int idx = 2;
      RegimeData? regime;
      TransitionData? trans;
      if (widget.regimeApi != null) {
        regime = results[idx] as RegimeData;
        idx++;
      }
      if (widget.transitionApi != null) {
        trans = results[idx] as TransitionData;
      }
      setState(() {
        _stateData = results[0] as MarketStateData;
        _compositeData = results[1] as CompositeIndexData;
        _regimeData = regime;
        _transitionData = trans;
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
          if (_regimeData != null) _buildRegimeCard(_regimeData!)
          else if (_stateData != null) _buildStateCard(_stateData!),
          const SizedBox(height: 16),
          if (_compositeData != null) _buildCompositeCard(_compositeData!),
          const SizedBox(height: 16),
          if (_regimeData != null) _buildMetricsCard(_regimeData!),
          if (_regimeData != null) const SizedBox(height: 16),
          if (_transitionData != null) _buildTransitionCard(_transitionData!),
          if (_transitionData != null) const SizedBox(height: 16),
          if (_stateData != null) _buildBreadthCard(_stateData!),
          const SizedBox(height: 32),
        ],
      ),
    );
  }

  // ---------- Regime Card ----------

  Widget _buildRegimeCard(RegimeData data) {
    final color = _regimeColor(data.regime);
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
              Icon(_regimeIcon(data.regime), color: color, size: 28),
              const SizedBox(width: 8),
              Text(
                data.regime.toUpperCase(),
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
            '$pct% confidence  •  ${data.timeframe}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
        ],
      ),
    );
  }

  // ---------- Metrics Card ----------

  Widget _buildMetricsCard(RegimeData data) {
    final m = data.metrics;
    final volLabel = _volatilityLabel(m.volatilityExpansion);
    final dispLabel = _dispersionLabel(m.dispersion);

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
            'Market Metrics',
            style: TextStyle(
              color: Colors.white70,
              fontSize: 14,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 12),
          _metricRow('Volatility', volLabel,
              _volatilityColor(m.volatilityExpansion)),
          const SizedBox(height: 8),
          _metricRow('Dispersion', dispLabel,
              _dispersionColor(m.dispersion)),
          const SizedBox(height: 8),
          _metricRow(
            'Trend Breadth',
            '${(m.trendBreadth * 100).toStringAsFixed(1)}%',
            Colors.tealAccent,
          ),
          const SizedBox(height: 8),
          _metricRow(
            'Compression Breadth',
            '${(m.compressionBreadth * 100).toStringAsFixed(1)}%',
            Colors.amber,
          ),
        ],
      ),
    );
  }

  Widget _metricRow(String label, String value, Color color) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(label,
            style: const TextStyle(fontSize: 13, color: Colors.white70)),
        Text(value, style: TextStyle(fontSize: 13, color: color)),
      ],
    );
  }

  String _volatilityLabel(double v) {
    if (v > 1.3) return 'high';
    if (v < 0.8) return 'low';
    return 'normal';
  }

  Color _volatilityColor(double v) {
    if (v > 1.3) return Colors.redAccent;
    if (v < 0.8) return Colors.blueGrey;
    return Colors.grey;
  }

  String _dispersionLabel(double d) {
    if (d > 0.05) return 'high';
    if (d < 0.02) return 'low';
    return 'moderate';
  }

  Color _dispersionColor(double d) {
    if (d > 0.05) return Colors.orangeAccent;
    if (d < 0.02) return Colors.blueGrey;
    return Colors.grey;
  }

  // ---------- Transition Probability Card ----------

  Widget _buildTransitionCard(TransitionData data) {
    final p = data.probabilities;
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
                'Transition Probabilities',
                style: TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              Text(
                data.horizon,
                style: const TextStyle(color: Colors.grey, fontSize: 11),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _probabilityBar('Trend', p.trend, Colors.tealAccent),
          const SizedBox(height: 8),
          _probabilityBar('Sideways', p.sideways, Colors.blueGrey),
          const SizedBox(height: 8),
          _probabilityBar('Expansion', p.expansion, Colors.redAccent),
        ],
      ),
    );
  }

  Widget _probabilityBar(String label, double value, Color color) {
    final pct = (value * 100).toStringAsFixed(0);
    return Row(
      children: [
        SizedBox(
          width: 80,
          child: Text(label,
              style: const TextStyle(fontSize: 13, color: Colors.white70)),
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
          width: 36,
          child: Text(
            '$pct%',
            style: const TextStyle(fontSize: 12, color: Colors.grey),
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }

  // ---------- Market State Card (fallback when no regime API) ----------

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

  Color _regimeColor(String regime) {
    switch (regime) {
      case 'compression':
        return Colors.amber;
      case 'sideways':
        return Colors.blueGrey;
      case 'trend':
        return Colors.tealAccent;
      case 'expansion':
        return Colors.redAccent;
      default:
        return Colors.grey;
    }
  }

  IconData _regimeIcon(String regime) {
    switch (regime) {
      case 'compression':
        return Icons.compress;
      case 'sideways':
        return Icons.swap_horiz;
      case 'trend':
        return Icons.trending_up;
      case 'expansion':
        return Icons.open_in_full;
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
