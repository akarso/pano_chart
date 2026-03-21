import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'composite_index_data.dart';
import 'http_composite_index_api.dart';
import 'http_market_state_api.dart';
import 'http_regime_api.dart';
import 'http_regime_history_api.dart';
import 'http_transition_api.dart';
import 'market_state_data.dart';
import 'regime_data.dart';
import 'regime_history_data.dart';
import 'transition_data.dart';

/// Full-page Market Pulse screen showing market state, breadth, and
/// composite index chart. Designed for extensibility with future stats.
class MarketPulseScreen extends StatefulWidget {
  final MarketStateApi marketStateApi;
  final CompositeIndexApi compositeIndexApi;
  final RegimeApi? regimeApi;
  final TransitionApi? transitionApi;
  final RegimeHistoryApi? regimeHistoryApi;

  const MarketPulseScreen({
    Key? key,
    required this.marketStateApi,
    required this.compositeIndexApi,
    this.regimeApi,
    this.transitionApi,
    this.regimeHistoryApi,
  }) : super(key: key);

  @override
  State<MarketPulseScreen> createState() => _MarketPulseScreenState();
}

const _supportedTimeframes = ['1m', '5m', '15m', '1h', '4h', '1d'];

class _MarketPulseScreenState extends State<MarketPulseScreen> {
  MarketStateData? _stateData;
  CompositeIndexData? _compositeData;
  RegimeData? _regimeData;
  TransitionData? _transitionData;
  RegimeHistoryData? _regimeHistoryData;
  String? _error;
  bool _loading = true;
  String _timeframe = '4h';

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
        widget.marketStateApi.fetch(timeframe: _timeframe),
        widget.compositeIndexApi.fetch(timeframe: _timeframe, limit: 200),
        if (widget.regimeApi != null)
          widget.regimeApi!.fetch(timeframe: _timeframe),
        if (widget.transitionApi != null)
          widget.transitionApi!.fetch(timeframe: _timeframe),
        if (widget.regimeHistoryApi != null)
          widget.regimeHistoryApi!.fetch(timeframe: _timeframe),
      ];
      final results = await Future.wait(futures);
      if (!mounted) return;
      int idx = 2;
      RegimeData? regime;
      TransitionData? trans;
      RegimeHistoryData? history;
      if (widget.regimeApi != null) {
        regime = results[idx] as RegimeData;
        idx++;
      }
      if (widget.transitionApi != null) {
        trans = results[idx] as TransitionData;
        idx++;
      }
      if (widget.regimeHistoryApi != null) {
        history = results[idx] as RegimeHistoryData;
      }
      setState(() {
        _stateData = results[0] as MarketStateData;
        _compositeData = results[1] as CompositeIndexData;
        _regimeData = regime;
        _transitionData = trans;
        _regimeHistoryData = history;
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
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 8),
            child: DropdownButtonHideUnderline(
              child: DropdownButton<String>(
                value: _timeframe,
                isDense: true,
                icon: const Icon(Icons.expand_more, color: Colors.white70, size: 18),
                dropdownColor: const Color(0xFF1A1A1A),
                style: const TextStyle(color: Colors.white, fontSize: 14),
                items: _supportedTimeframes
                    .map((tf) => DropdownMenuItem(
                          value: tf,
                          child: Text(tf),
                        ))
                    .toList(),
                onChanged: (v) {
                  if (v != null && v != _timeframe) {
                    setState(() => _timeframe = v);
                    _loadAll();
                  }
                },
              ),
            ),
          ),
        ],
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
          if (_regimeHistoryData != null)
            _buildRegimeHistoryCard(_regimeHistoryData!),
          if (_regimeHistoryData != null) const SizedBox(height: 16),
          if (_stateData != null) _buildBreadthCard(_stateData!),
          const SizedBox(height: 32),
        ],
      ),
    );
  }

  // ---------- Regime Card ----------

  Widget _buildRegimeCard(RegimeData data) {
    final color = _regimeColor(data.regime);
    final pct = (data.prevalence * 100).toStringAsFixed(0);

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
            '$pct% prevalence  •  ${data.timeframe}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 16),
          _regimeScoreBar('Trend', data.scores.trend, Colors.tealAccent),
          const SizedBox(height: 6),
          _regimeScoreBar('Sideways', data.scores.sideways, Colors.blueGrey),
          const SizedBox(height: 6),
          _regimeScoreBar('Compression', data.scores.compression, Colors.amber),
          const SizedBox(height: 6),
          _regimeScoreBar('Expansion', data.scores.expansion, Colors.redAccent),
        ],
      ),
    );
  }

  Widget _regimeScoreBar(String label, double value, Color color) {
    final pct = (value * 100).toStringAsFixed(0);
    return Row(
      children: [
        SizedBox(
          width: 90,
          child: Text(label,
              style: const TextStyle(fontSize: 12, color: Colors.white54)),
        ),
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: value,
              backgroundColor: Colors.white10,
              valueColor: AlwaysStoppedAnimation<Color>(color.withAlpha(180)),
              minHeight: 6,
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 32,
          child: Text(
            '$pct%',
            style: const TextStyle(fontSize: 11, color: Colors.white38),
            textAlign: TextAlign.right,
          ),
        ),
      ],
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
          Row(
            children: [
              const Text(
                'Market Metrics',
                style: TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 4),
              GestureDetector(
                onTap: () => _showInfoDialog(
                  title: 'Market Metrics',
                  body: 'Volatility — short-term ATR / long-term ATR ratio.\n'
                      '  • < 0.8 low  •  0.8–1.3 normal  •  > 1.3 high\n\n'
                      'Dispersion — how differently assets move from each other.\n'
                      '  • < 2% low  •  2–5% moderate  •  > 5% high\n\n'
                      'Trend Breadth — average directional strength across all tokens (0–100%).\n\n'
                      'Compression Breadth — average range-contraction signal across all tokens (0–100%).',
                ),
                child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
              ),
            ],
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
            'Sideways Breadth',
            '${(m.sidewaysBreadth * 100).toStringAsFixed(1)}%',
            Colors.blueGrey,
          ),
          const SizedBox(height: 8),
          _metricRow(
            'Breakout Breadth',
            '${(m.breakoutBreadth * 100).toStringAsFixed(1)}%',
            Colors.redAccent,
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
              Row(
                children: [
                  const Text(
                    'Transition Probabilities',
                    style: TextStyle(
                      color: Colors.white70,
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const SizedBox(width: 4),
                  GestureDetector(
                    onTap: () => _showInfoDialog(
                      title: 'Transition Probabilities',
                      body: 'Estimated likelihood of the market transitioning '
                          'to each regime given the current conditions.\n\n'
                          'Based on compression breadth, volatility slope, '
                          'and regime age (older regimes build more '
                          'breakout pressure).\n\n'
                          'Values are 0–100% and sum to ~100%.\n\n'
                          'Regime Age shows how long the current regime '
                          'has persisted, in both candles and real time.',
                    ),
                    child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
                  ),
                ],
              ),
              Text(
                'Age: ${data.horizon}',
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

  // ---------- Regime History Card ----------

  Widget _buildRegimeHistoryCard(RegimeHistoryData data) {
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
              Row(
                children: [
                  const Text(
                    'Regime History',
                    style: TextStyle(
                      color: Colors.white70,
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const SizedBox(width: 4),
                  GestureDetector(
                    onTap: () => _showInfoDialog(
                      title: 'Regime History',
                      body: 'Timeline of detected market regimes.\n\n'
                          'Age — how many candle periods the current regime '
                          'has been active.\n\n'
                          'The coloured bar shows the most recent regime '
                          'periods (up to 20) with duration proportional to '
                          'candle count.',
                    ),
                    child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
                  ),
                ],
              ),
              Text(
                'Age: ${data.currentAge} candles',
                style: const TextStyle(
                  color: Colors.amberAccent,
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          if (data.history.isEmpty)
            const Text(
              'No history yet',
              style: TextStyle(color: Colors.grey, fontSize: 12),
            )
          else
            _buildTimeline(data.history),
        ],
      ),
    );
  }

  Widget _buildTimeline(List<RegimePeriodData> periods) {
    // Show a horizontal regime timeline bar.
    final totalCandles =
        periods.fold<int>(0, (sum, p) => sum + p.durationCandles);
    if (totalCandles == 0) {
      return const SizedBox.shrink();
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(4),
          child: SizedBox(
            height: 12,
            child: Row(
              children: periods.map((p) {
                final flex = math.max(1, p.durationCandles);
                return Expanded(
                  flex: flex,
                  child: Container(color: _regimeColor(p.regime)),
                );
              }).toList(),
            ),
          ),
        ),
        const SizedBox(height: 8),
        ...periods.reversed.take(5).map((p) {
          final open = p.end == null;
          return Padding(
            padding: const EdgeInsets.only(bottom: 4),
            child: Row(
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: _regimeColor(p.regime),
                    shape: BoxShape.circle,
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    '${p.regime.toUpperCase()} — ${p.durationCandles} candles${open ? '  (active)' : ''}',
                    style: TextStyle(
                      color: open ? Colors.white : Colors.white54,
                      fontSize: 12,
                    ),
                  ),
                ),
              ],
            ),
          );
        }),
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
            '${data.symbolCount} symbols  •  base 100${_timeRangeLabel(data.points)}',
            style: const TextStyle(color: Colors.grey, fontSize: 11),
          ),
          const SizedBox(height: 12),
          SizedBox(
            height: 200,
            child: hasPoints
                ? Column(
                    children: [
                      Expanded(
                        child: CustomPaint(
                          size: Size.infinite,
                          painter: _CompositeChartPainter(
                            points: data.points,
                            lineColor: changeColor,
                          ),
                        ),
                      ),
                      const SizedBox(height: 4),
                      _buildTimeLabels(data.points),
                    ],
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

  /// Builds evenly-spaced time labels beneath the chart.
  Widget _buildTimeLabels(List<IndexPoint> points) {
    if (points.length < 2) return const SizedBox.shrink();
    // Pick ~4 label positions: first, 1/3, 2/3, last.
    final indices = [
      0,
      points.length ~/ 3,
      (points.length * 2) ~/ 3,
      points.length - 1,
    ];
    const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    String fmt(int ts) {
      final dt = DateTime.fromMillisecondsSinceEpoch(ts * 1000, isUtc: true).toLocal();
      final m = months[dt.month - 1];
      final d = dt.day;
      final h = dt.hour.toString().padLeft(2, '0');
      final min = dt.minute.toString().padLeft(2, '0');
      return '$m $d $h:$min';
    }
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: indices.map((i) {
        return Text(
          fmt(points[i].timestamp),
          style: const TextStyle(color: Colors.white38, fontSize: 9),
        );
      }).toList(),
    );
  }

  /// Produces a human-readable time range suffix like "  •  ~33 days".
  String _timeRangeLabel(List<IndexPoint> points) {
    if (points.length < 2) return '';
    final spanSec = points.last.timestamp - points.first.timestamp;
    if (spanSec <= 0) return '';
    final hours = spanSec / 3600;
    if (hours < 24) return '  \u2022  ~${hours.round()}h';
    final days = hours / 24;
    if (days < 2) return '  \u2022  ~${hours.round()}h';
    return '  \u2022  ~${days.round()}d';
  }

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
          Row(
            children: [
              const Text(
                'Market Breadth',
                style: TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 4),
              GestureDetector(
                onTap: () => _showInfoDialog(
                  title: 'Market Breadth',
                  body: 'Proportionally-weighted distribution of regime '
                      'character across the full token universe.\n\n'
                      'Each bar shows the average score weight for that '
                      'regime (0–100%). All four bars sum to ~100%.\n\n'
                      '• Sideways — range-bound, low conviction\n'
                      '• Compression — narrowing ranges\n'
                      '• Breakout — price escaping a range\n'
                      '• Trend — strong directional move',
                ),
                child: const Icon(Icons.help_outline, size: 13, color: Colors.white30),
              ),
            ],
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

  void _showInfoDialog({required String title, required String body}) {
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: Text(body),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

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
