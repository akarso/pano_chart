import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/app_lifecycle_manager.dart';
import '../../core/auto_refresh_timer.dart';
import '../../core/polling_config.dart';
import 'composite_chart_painter.dart';
import 'composite_index_data.dart';
import 'http_composite_index_api.dart';
import 'http_market_state_api.dart';
import 'http_regime_api.dart';
import 'http_regime_history_api.dart';
import 'http_transition_api.dart';
import 'market_state_data.dart';
import 'participation_counts.dart';
import 'regime_data.dart';
import 'regime_history_data.dart';
import 'transition_data.dart';
import '../scorecards/http_scorecard_api.dart';
import '../scorecards/reliability_chip.dart';
import '../scorecards/scorecard_catalog.dart';
import '../scorecards/scorecard_data.dart';
import '../scorecards/scorecards_screen.dart';

/// Full-page Market Pulse screen showing market state, participation, and
/// composite index chart. Designed for extensibility with future stats.
class MarketPulseScreen extends StatefulWidget {
  final MarketStateApi marketStateApi;
  final CompositeIndexApi compositeIndexApi;
  final RegimeApi? regimeApi;
  final TransitionApi? transitionApi;
  final RegimeHistoryApi? regimeHistoryApi;
  final ScorecardApi? scorecardApi;
  final String? initialTimeframe;
  final bool isProUser;

  const MarketPulseScreen({
    Key? key,
    required this.marketStateApi,
    required this.compositeIndexApi,
    this.regimeApi,
    this.transitionApi,
    this.regimeHistoryApi,
    this.scorecardApi,
    this.initialTimeframe,
    this.isProUser = false,
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
  final ScorecardCatalog _scorecards = ScorecardCatalog();
  String? _error;
  bool _loading = true;
  String _timeframe = '4h';

  /// When true and volume-weighted series exists, chart that path (PR-084).
  bool _useVolumeWeighted = true;

  /// Last `regimeSource` that drove `_useVolumeWeighted`. Chip taps must
  /// survive refresh; we only re-lock when the source string itself changes.
  String? _syncedRegimeSource;

  AutoRefreshTimer? _autoRefreshTimer;
  Pausable? _pausable;
  AppLifecycleManager? _lifecycleManager;

  static const _prefKeyTimeframe = 'marketPulse.timeframe';

  @override
  void initState() {
    super.initState();
    final initial = widget.initialTimeframe;
    if (initial != null && _supportedTimeframes.contains(initial)) {
      _timeframe = initial;
    }
    _initializeData();
  }

  Future<void> _initializeData() async {
    await _loadPersistedTimeframe();
    if (!mounted) return;
    _loadAll();
    _startAutoRefresh();
  }

  Future<void> _loadPersistedTimeframe() async {
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString(_prefKeyTimeframe);
    if (saved != null &&
        _supportedTimeframes.contains(saved) &&
        widget.initialTimeframe == null) {
      if (mounted) {
        setState(() => _timeframe = saved);
      }
    }
  }

  Future<void> _persistTimeframe(String tf) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_prefKeyTimeframe, tf);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_pausable == null) {
      _lifecycleManager = AppLifecycleScope.of(context);
      if (_lifecycleManager != null) {
        _pausable = Pausable(
          onPause: () => _autoRefreshTimer?.stop(),
          onResume: () => _autoRefreshTimer?.start(),
        );
        _lifecycleManager!.addPausable(_pausable!);
      }
    }
  }

  @override
  void dispose() {
    if (_pausable != null) _lifecycleManager?.removePausable(_pausable!);
    _autoRefreshTimer?.dispose();
    super.dispose();
  }

  void _startAutoRefresh() {
    _autoRefreshTimer?.dispose();
    _autoRefreshTimer = null;
    if (!widget.isProUser) return;
    final interval = kChartRefreshIntervals[_timeframe];
    if (interval == null) return;
    _autoRefreshTimer = AutoRefreshTimer(
      interval: interval,
      onTick: _autoRefreshData,
    );
    _autoRefreshTimer!.start();
  }

  Future<void> _autoRefreshData() async {
    if (!mounted) return;
    try {
      final futures = <Future>[
        widget.marketStateApi.fetch(timeframe: _timeframe),
        widget.compositeIndexApi.fetch(
          timeframe: _timeframe,
          limit: tapeMetricsWindow,
        ),
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
        _syncSeriesToRegimeSourceIfChanged();
      });
    } catch (_) {
      // Silently ignore — next tick will retry.
    }
  }

  Future<void> _loadAll() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    _loadScorecards();
    try {
      final futures = <Future>[
        widget.marketStateApi.fetch(timeframe: _timeframe),
        widget.compositeIndexApi.fetch(
          timeframe: _timeframe,
          limit: tapeMetricsWindow,
        ),
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
        _syncSeriesToRegimeSourceIfChanged();
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  /// Lock the chart series to the tape only when [regimeSource] changes.
  /// Chip taps survive auto-refresh.
  void _syncSeriesToRegimeSourceIfChanged() {
    final src = _regimeSource();
    if (src == _syncedRegimeSource) return;
    _syncedRegimeSource = src;
    if (src == 'composite_volume_weighted') {
      _useVolumeWeighted = true;
    } else if (src == 'composite_median') {
      _useVolumeWeighted = false;
    }
  }

  String _regimeSource() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      return _regimeData!.regimeSource;
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _stateData!.regimeSource;
    }
    return '';
  }

  ParticipationCounts? _participationCounts() {
    if (_regimeData != null &&
        !_regimeData!.isDataUnavailable &&
        _regimeData!.participation.hasData) {
      return _regimeData!.participation;
    }
    if (_stateData != null &&
        !_stateData!.isDataUnavailable &&
        _stateData!.participation.hasData) {
      return _stateData!.participation;
    }
    return null;
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
                icon: const Icon(
                  Icons.expand_more,
                  color: Colors.white70,
                  size: 18,
                ),
                dropdownColor: const Color(0xFF1A1A1A),
                style: const TextStyle(color: Colors.white, fontSize: 14),
                items: _supportedTimeframes
                    .map((tf) => DropdownMenuItem(value: tf, child: Text(tf)))
                    .toList(),
                onChanged: (v) {
                  if (v != null && v != _timeframe) {
                    setState(() => _timeframe = v);
                    _persistTimeframe(v);
                    _loadAll();
                    _startAutoRefresh();
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
              const Icon(
                Icons.error_outline,
                color: Colors.redAccent,
                size: 48,
              ),
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
              TextButton(onPressed: _loadAll, child: const Text('Retry')),
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
          KeyedSubtree(
            key: const Key('mp-headline'),
            child: _buildHeadlineCard(),
          ),
          const SizedBox(height: 16),
          if (_compositeData != null)
            KeyedSubtree(
              key: const Key('mp-composite'),
              child: _buildCompositeCard(_compositeData!),
            ),
          if (_compositeData != null) const SizedBox(height: 16),
          Builder(
            builder: (context) {
              final participation = _participationCounts();
              if (participation == null) return const SizedBox.shrink();
              return Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  KeyedSubtree(
                    key: const Key('mp-participation'),
                    child: _buildParticipationCard(participation),
                  ),
                  const SizedBox(height: 16),
                ],
              );
            },
          ),
          if (_regimeData != null && !_regimeData!.isDataUnavailable)
            _buildMetricsCard(_regimeData!),
          if (_regimeData != null && !_regimeData!.isDataUnavailable)
            const SizedBox(height: 16),
          if (_transitionData != null) _buildTransitionCard(_transitionData!),
          if (_transitionData != null) const SizedBox(height: 16),
          if (_regimeHistoryData != null)
            _buildRegimeHistoryCard(_regimeHistoryData!),
          if (_regimeHistoryData != null) const SizedBox(height: 16),
          const SizedBox(height: 16),
        ],
      ),
    );
  }

  // ---------- Headline selection ----------

  /// Picks which headline card to show. Previously this always preferred
  /// `_regimeData` whenever it was non-null, even if it was unavailable and
  /// `_stateData` was perfectly healthy — showing a "Data unavailable"
  /// banner while `_buildBreadthCard()` below it fell back to state and
  /// rendered real numbers, a contradictory screen (PR-074 CR follow-up).
  /// Prefer whichever source is actually healthy; only fall through to a
  /// banner when both are unavailable (or only one source exists at all).
  Widget _buildHeadlineCard() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      return _buildRegimeCard(_regimeData!);
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _buildStateCard(_stateData!);
    }
    if (_regimeData != null) {
      return _buildRegimeCard(_regimeData!); // renders the banner itself
    }
    if (_stateData != null) {
      return _buildStateCard(_stateData!); // renders the banner itself
    }
    return const SizedBox.shrink();
  }

  // ---------- Regime Card ----------

  Widget _buildRegimeCard(RegimeData data) {
    if (data.isDataUnavailable) {
      return const _DataUnavailableBanner();
    }
    final color = data.regime == 'trend'
        ? _trendBiasColor(data.bias)
        : _regimeColor(data.regime);
    final icon = data.regime == 'trend'
        ? _trendBiasIcon(data.bias)
        : _regimeIcon(data.regime);
    final pct = (data.prevalence * 100).toStringAsFixed(0);
    final hasLabel = data.label.isNotEmpty;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF1A1A1A),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        children: [
          Wrap(
            alignment: WrapAlignment.center,
            crossAxisAlignment: WrapCrossAlignment.center,
            spacing: 6,
            children: [
              Icon(icon, color: color, size: 28),
              Text(
                _regimeLabel(data.regime, data.bias),
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                  color: color,
                ),
              ),
              ReliabilityChip(
                item:
                    _scorecards.items[scorecardKey(
                      'regime',
                      regimeScorecardLabel(data.regime),
                    )],
              ),
              GestureDetector(
                key: const Key('mp-regime-help'),
                onTap: () => _showRegimeInfo(data),
                child: const Padding(
                  padding: EdgeInsets.all(4),
                  child: Icon(
                    Icons.help_outline,
                    size: 16,
                    color: Colors.white30,
                  ),
                ),
              ),
              if (hasLabel) ...[
                const Text(
                  '·',
                  style: TextStyle(fontSize: 24, color: Colors.grey),
                ),
                Icon(
                  _healthIcon(data.label),
                  color: _healthColor(data.label),
                  size: 20,
                ),
                Text(
                  _healthSuffix(data.label),
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                    color: _healthColor(data.label),
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '$pct% ${regimeConfidenceLabel(data.regimeSource)}  •  ${data.timeframe}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 2),
          Text(
            _headlineSubline(
              data.regimeSource,
              data.windowBars,
              _chartPointCount(),
              matchesTape: _displayedSeriesMatchesTape(),
            ),
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ],
      ),
    );
  }

  int _chartPointCount() {
    final data = _compositeData;
    if (data == null) return 0;
    final pts = (_useVolumeWeighted && data.hasVolumeWeighted)
        ? data.volumeWeightedPoints
        : data.points;
    return pts.length;
  }

  /// Caption for the scored window. Uses min(windowBars, chart length) and a
  /// different sentence when the chart is shorter than the tape. Must not
  /// claim the chart is scored when the displayed series is not the tape.
  String _headlineSubline(
    String regimeSource,
    int windowBars,
    int chartLen, {
    required bool matchesTape,
  }) {
    if (!isKnownCompositeSource(regimeSource)) {
      return regimeSource == 'participation' || regimeSource.isEmpty
          ? 'From token participation (tape unavailable)'
          : 'From merged market tape (same structure as one chart)';
    }
    if (!matchesTape) {
      return 'Headline scored on the tape; chart shows another series';
    }
    if (windowBars <= 0) {
      return 'From merged market tape (same structure as one chart)';
    }
    if (chartLen <= 0) {
      return 'Scored on $windowBars bars';
    }
    final shown = math.min(windowBars, chartLen);
    if (windowBars > chartLen) {
      return 'Scored on $windowBars bars; chart shows $chartLen';
    }
    if (windowBars == chartLen) {
      return 'Scored on these $shown bars';
    }
    return 'Scored on the last $shown bars shown';
  }

  Widget _regimeScoreBar(String label, double value, Color color) {
    final pct = (value * 100).toStringAsFixed(0);
    return Row(
      key: Key('mp-structure-bar-$label'),
      children: [
        SizedBox(
          width: 90,
          child: Text(
            label,
            style: const TextStyle(fontSize: 12, color: Colors.white54),
          ),
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
                  body:
                      'Avg mix — average per-token score mix across the '
                      'universe (0–1 each). Not the count-based Market '
                      'participation card.\n\n'
                      'Volatility — short-term ATR / long-term ATR ratio.\n'
                      '  • < 0.8 low  •  0.8–1.3 normal  •  > 1.3 high\n\n'
                      'Dispersion — how differently assets move from each other.\n'
                      '  • < 2% low  •  2–5% moderate  •  > 5% high\n\n'
                      'Avg mix rows are not the headline. The headline '
                      'comes from the merged market tape.',
                ),
                child: const Icon(
                  Icons.help_outline,
                  size: 13,
                  color: Colors.white30,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _metricRow(
            'Volatility',
            volLabel,
            _volatilityColor(m.volatilityExpansion),
          ),
          const SizedBox(height: 8),
          _metricRow('Dispersion', dispLabel, _dispersionColor(m.dispersion)),
          const SizedBox(height: 8),
          _metricRow(
            'Avg trend mix',
            '${(m.trendBreadth * 100).toStringAsFixed(1)}%',
            Colors.tealAccent,
          ),
          const SizedBox(height: 8),
          _metricRow(
            'Avg sideways mix',
            '${(m.sidewaysBreadth * 100).toStringAsFixed(1)}%',
            Colors.blueGrey,
          ),
          const SizedBox(height: 8),
          _metricRow(
            'Avg compression mix',
            '${(m.compressionBreadth * 100).toStringAsFixed(1)}%',
            Colors.amber,
          ),
          const SizedBox(height: 8),
          _metricRow(
            'Avg expansion mix',
            '${(m.expansionBreadth * 100).toStringAsFixed(1)}%',
            Colors.redAccent,
          ),
        ],
      ),
    );
  }

  Widget _metricRow(String label, String value, Color color) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: const TextStyle(fontSize: 13, color: Colors.white70),
        ),
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
                      body:
                          'Regime — dominant structure of the tape: trend / '
                          'sideways / compression / expansion / indecisive / '
                          'silent.\n\n'
                          'Estimated likelihood of the market transitioning '
                          'to each regime given the current conditions.\n\n'
                          'Based on compression participation, volatility '
                          'slope, and regime age (older regimes build more '
                          'expansion pressure).\n\n'
                          'Values are 0–100% and sum to ~100%.\n\n'
                          'Regime Age shows how long the current regime '
                          'has persisted, in both candles and real time.',
                    ),
                    child: const Icon(
                      Icons.help_outline,
                      size: 13,
                      color: Colors.white30,
                    ),
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
          _probabilityBar('Compression', p.compression, Colors.amber),
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
          child: Text(
            label,
            style: const TextStyle(fontSize: 13, color: Colors.white70),
          ),
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
                      body:
                          'Regime — dominant structure of the tape: trend / '
                          'sideways / compression / expansion / indecisive / '
                          'silent.\n\n'
                          'Timeline of detected market regimes.\n\n'
                          'Age — how many candle periods the current regime '
                          'has been active.\n\n'
                          'The coloured bar shows the most recent regime '
                          'periods (up to 20) with duration proportional to '
                          'candle count.',
                    ),
                    child: const Icon(
                      Icons.help_outline,
                      size: 13,
                      color: Colors.white30,
                    ),
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
    final totalCandles = periods.fold<int>(
      0,
      (sum, p) => sum + p.durationCandles,
    );
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
                    '${p.regime.toUpperCase()} — ${p.durationCandles} candles${open ? '  (latest)' : ''}',
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
    if (data.isDataUnavailable) {
      return const _DataUnavailableBanner();
    }
    final color = _stateColor(data.state, data.bias);
    final pct = (data.confidence * 100).toStringAsFixed(1);
    final hasLabel = data.label.isNotEmpty;

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
              Icon(_stateIcon(data.state, data.bias), color: color, size: 28),
              const SizedBox(width: 8),
              Text(
                data.state.toUpperCase(),
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                  color: color,
                ),
              ),
              if (hasLabel) ...[
                const SizedBox(width: 8),
                const Text(
                  '·',
                  style: TextStyle(fontSize: 24, color: Colors.grey),
                ),
                const SizedBox(width: 8),
                Icon(
                  _healthIcon(data.label),
                  color: _healthColor(data.label),
                  size: 20,
                ),
                const SizedBox(width: 4),
                Text(
                  _healthSuffix(data.label),
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                    color: _healthColor(data.label),
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '$pct% ${data.confidenceLabel}  •  ${data.symbolCount} symbols  •  ${data.timeframe}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
          const SizedBox(height: 2),
          Text(
            _headlineSubline(
              data.regimeSource,
              data.windowBars,
              _chartPointCount(),
              matchesTape: _displayedSeriesMatchesTape(),
            ),
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ],
      ),
    );
  }

  // ---------- Composite Index Chart Card ----------

  Widget _buildCompositeCard(CompositeIndexData data) {
    final src = _regimeSource();
    final isCompositeTape = isKnownCompositeSource(src);
    final matchesTape = _displayedSeriesMatchesTape();
    final chartPoints = (_useVolumeWeighted && data.hasVolumeWeighted)
        ? data.volumeWeightedPoints
        : data.points;
    final hasPoints = chartPoints.isNotEmpty;
    final scoredWin = matchesTape ? _scoredWindowBars() : 0;
    final scoredStart = scoredWindowStart(chartPoints.length, scoredWin);
    final change = hasPoints && chartPoints.length > 1
        ? chartPoints.last.value - chartPoints[scoredStart].value
        : 0.0;
    final changeStr = change >= 0
        ? '+${change.toStringAsFixed(2)}'
        : change.toStringAsFixed(2);
    final changeColor = change >= 0 ? Colors.greenAccent : Colors.redAccent;
    final seriesLabel = (_useVolumeWeighted && data.hasVolumeWeighted)
        ? 'Volume weighted'
        : 'equal weight (median)';
    final showRegression = matchesTape && scoredWin >= 2;
    final changeScope = isCompositeTape && !matchesTape
        ? 'series change'
        : (matchesTape &&
                scoredWin > 0 &&
                scoredWin < chartPoints.length
            ? 'scored-window change'
            : 'window change');

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
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    '$changeStr%',
                    style: TextStyle(color: changeColor, fontSize: 14),
                  ),
                  Text(
                    changeScope,
                    style: const TextStyle(color: Colors.white38, fontSize: 10),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '${data.symbolCount} symbols  •  $seriesLabel  •  base 100${_timeRangeLabel(chartPoints)}',
            style: const TextStyle(color: Colors.grey, fontSize: 11),
          ),
          if (data.hasVolumeWeighted) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                _compositeSeriesChip(
                  label: 'Volume weighted',
                  selected: _useVolumeWeighted,
                  onTap: () => setState(() => _useVolumeWeighted = true),
                ),
                const SizedBox(width: 8),
                _compositeSeriesChip(
                  label: 'Median',
                  selected: !_useVolumeWeighted,
                  onTap: () => setState(() => _useVolumeWeighted = false),
                ),
              ],
            ),
            if (isCompositeTape && !matchesTape)
              const Padding(
                padding: EdgeInsets.only(top: 6),
                child: Text(
                  'Showing a different series than the headline tape — '
                  'regression hidden',
                  style: TextStyle(color: Colors.white38, fontSize: 10),
                ),
              ),
          ],
          const SizedBox(height: 12),
          SizedBox(
            height: 200,
            child: hasPoints
                ? Column(
                    children: [
                      Expanded(
                        child: CustomPaint(
                          key: const Key('mp-composite-paint'),
                          size: Size.infinite,
                          painter: CompositeChartPainter(
                            points: chartPoints,
                            lineColor: changeColor,
                            regressionColor: showRegression
                                ? _headlineChartColor()
                                : Colors.transparent,
                            windowBars: showRegression ? scoredWin : 0,
                            solidRegression:
                                showRegression && _isTrendHeadline(),
                          ),
                        ),
                      ),
                      const SizedBox(height: 4),
                      _buildTimeLabels(chartPoints),
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

  Widget _compositeSeriesChip({
    required String label,
    required bool selected,
    required VoidCallback onTap,
  }) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          color: selected ? Colors.white12 : Colors.transparent,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: selected ? Colors.white38 : Colors.white12),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 11,
            color: selected ? Colors.white70 : Colors.white38,
          ),
        ),
      ),
    );
  }

  int _scoredWindowBars() {
    if (_regimeData != null &&
        !_regimeData!.isDataUnavailable &&
        _regimeData!.windowBars > 0) {
      return _regimeData!.windowBars;
    }
    if (_stateData != null &&
        !_stateData!.isDataUnavailable &&
        _stateData!.windowBars > 0) {
      return _stateData!.windowBars;
    }
    return 0;
  }

  /// True when the chart series is the same path as `regimeSource`.
  /// Exact contract values only: `composite_volume_weighted` /
  /// `composite_median`.
  bool _displayedSeriesMatchesTape() {
    final src = _regimeSource();
    final data = _compositeData;
    if (data == null) return false;
    final showingVw = _useVolumeWeighted && data.hasVolumeWeighted;
    if (src == 'composite_volume_weighted') {
      return showingVw;
    }
    if (src == 'composite_median') {
      return !showingVw;
    }
    return false;
  }

  bool _isTrendHeadline() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      return _regimeData!.regime == 'trend';
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _stateData!.state == 'trend';
    }
    return false;
  }

  Color _headlineChartColor() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      final d = _regimeData!;
      return d.regime == 'trend'
          ? _trendBiasColor(d.bias)
          : _regimeColor(d.regime);
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _stateColor(_stateData!.state, _stateData!.bias);
    }
    return Colors.tealAccent;
  }

  // ---------- Participation Card ----------

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
    const months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    String fmt(int ts) {
      final dt = DateTime.fromMillisecondsSinceEpoch(
        ts * 1000,
        isUtc: true,
      ).toLocal();
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

  Widget _buildParticipationCard(ParticipationCounts counts) {
    final (upPct, rangingPct, downPct) = participationPercents(counts);
    final bias = _headlineBias();
    final regime = _headlineRegime();
    final reading = participationReading(
      counts,
      bias: bias,
      regime: regime,
    );

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
                'Market participation',
                style: TextStyle(
                  color: Colors.white70,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 4),
              GestureDetector(
                key: const Key('mp-participation-help'),
                onTap: () => _showInfoDialog(
                  title: 'Market participation',
                  body:
                      'Participation — share of tokens whose own chart is '
                      'currently in an uptrend, a downtrend, or ranging. It '
                      'can differ from the headline: averaging 150 noisy '
                      'charts removes noise, so the tape can trend cleanly '
                      'while many single tokens still look range-bound — or '
                      'a few heavyweights can pull the tape while most '
                      'tokens sit still.',
                ),
                child: const Icon(
                  Icons.help_outline,
                  size: 13,
                  color: Colors.white30,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: SizedBox(
              height: 14,
              child: Row(
                children: [
                  if (counts.up > 0)
                    Expanded(
                      flex: counts.up,
                      child: Container(color: Colors.tealAccent),
                    ),
                  if (counts.ranging > 0)
                    Expanded(
                      flex: counts.ranging,
                      child: Container(color: Colors.blueGrey),
                    ),
                  if (counts.down > 0)
                    Expanded(
                      flex: counts.down,
                      child: Container(color: Colors.redAccent),
                    ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 8),
          Text(
            'Up $upPct% · Ranging $rangingPct% · Down $downPct%',
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
          if (reading.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text(
              reading,
              style: const TextStyle(color: Colors.white38, fontSize: 12),
            ),
          ],
        ],
      ),
    );
  }

  String _headlineBias() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      return _regimeData!.bias;
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _stateData!.bias;
    }
    return '';
  }

  String _headlineRegime() {
    if (_regimeData != null && !_regimeData!.isDataUnavailable) {
      return _regimeData!.regime;
    }
    if (_stateData != null && !_stateData!.isDataUnavailable) {
      return _stateData!.state;
    }
    return '';
  }

  // ---------- Trend Health Helpers ----------

  /// Short suffix shown next to the regime name.
  String _healthSuffix(String label) {
    if (label.contains('Strong')) return 'Strong';
    if (label.contains('weakening')) return 'Weakening \u2193';
    if (label.contains('breaking')) return 'Breaking \u2193\u2193';
    if (label.contains('Mixed')) return 'Mixed';
    return label;
  }

  IconData _healthIcon(String label) {
    if (label.contains('Strong')) return Icons.trending_up;
    if (label.contains('weakening')) return Icons.trending_flat;
    if (label.contains('breaking')) return Icons.trending_down;
    return Icons.remove;
  }

  Color _healthColor(String label) {
    if (label.contains('Strong')) return Colors.greenAccent;
    if (label.contains('weakening')) return Colors.orangeAccent;
    if (label.contains('breaking')) return Colors.redAccent;
    return Colors.grey;
  }

  // ---------- Helpers ----------

  void _showRegimeInfo(RegimeData data) {
    final api = widget.scorecardApi;
    showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Regime'),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Regime — dominant structure of the tape: trend / '
                'sideways / compression / expansion / indecisive / silent.\n\n'
                'The headline is that structure on the merged market tape.',
              ),
              const SizedBox(height: 16),
              const Text(
                'Structure mix',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              _regimeScoreBar('Trend', data.scores.trend, Colors.tealAccent),
              const SizedBox(height: 6),
              _regimeScoreBar(
                'Sideways',
                data.scores.sideways,
                Colors.blueGrey,
              ),
              const SizedBox(height: 6),
              _regimeScoreBar(
                'Compression',
                data.scores.compression,
                Colors.amber,
              ),
              const SizedBox(height: 6),
              _regimeScoreBar(
                'Expansion',
                data.scores.expansion,
                Colors.redAccent,
              ),
              if (api != null) ...[
                const SizedBox(height: 8),
                TextButton(
                  onPressed: () {
                    Navigator.of(dialogContext).pop();
                    Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) =>
                            ScorecardsScreen(api: api, timeframe: _timeframe),
                      ),
                    );
                  },
                  child: const Text('Reliability'),
                ),
              ],
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

  Future<void> _loadScorecards() {
    return _scorecards.load(
      api: widget.scorecardApi,
      timeframe: _timeframe,
      notify: () {
        if (mounted) setState(() {});
      },
    );
  }

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

  Color _stateColor(String state, [String bias = 'neutral']) {
    switch (state) {
      case 'sideways':
        return Colors.blueGrey;
      case 'compression':
        return Colors.amber;
      case 'expansion':
        return Colors.redAccent;
      case 'trend':
        return bias == 'down' ? Colors.redAccent : Colors.tealAccent;
      case 'silent':
        return Colors.white;
      case 'indecisive':
        return const Color(0xFFB0C4DE);
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
      case 'expansion':
        return Icons.open_in_full;
      case 'trend':
        return bias == 'down' ? Icons.trending_down : Icons.trending_up;
      case 'silent':
        return Icons.horizontal_rule;
      case 'indecisive':
        return Icons.help_outline;
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
      case 'silent':
        return Colors.white;
      case 'indecisive':
        return const Color(0xFFB0C4DE);
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
      case 'silent':
        return Icons.horizontal_rule;
      case 'indecisive':
        return Icons.help_outline;
      default:
        return Icons.help_outline;
    }
  }

  Color _trendBiasColor(String bias) {
    switch (bias) {
      case 'up':
        return Colors.tealAccent;
      case 'down':
        return Colors.redAccent;
      default:
        return Colors.amber;
    }
  }

  IconData _trendBiasIcon(String bias) {
    switch (bias) {
      case 'up':
        return Icons.trending_up;
      case 'down':
        return Icons.trending_down;
      default:
        return Icons.show_chart;
    }
  }

  String _regimeLabel(String regime, String bias) {
    if (regime == 'trend') {
      switch (bias) {
        case 'up':
          return 'UPTREND';
        case 'down':
          return 'DOWNTREND';
        default:
          return 'TREND';
      }
    }
    return regime.toUpperCase();
  }
}

/// Shown instead of the regime/state card when DataQuality is
/// "unavailable" — a full evaluation-source outage, distinct from a
/// legitimate "Indecisive"/"Sideways, low confidence" reading, which
/// otherwise looks identical (see PR-074).
class _DataUnavailableBanner extends StatelessWidget {
  const _DataUnavailableBanner();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF1A1A1A),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.orangeAccent.withAlpha(120)),
      ),
      child: const Column(
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.cloud_off, color: Colors.orangeAccent, size: 28),
              SizedBox(width: 8),
              Text(
                'Data unavailable',
                style: TextStyle(
                  fontSize: 20,
                  fontWeight: FontWeight.bold,
                  color: Colors.orangeAccent,
                ),
              ),
            ],
          ),
          SizedBox(height: 6),
          Text(
            'Market data could not be read right now — this is not a quiet '
            'market, the read itself failed. Pull to refresh in a moment.',
            style: TextStyle(color: Colors.grey, fontSize: 12),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}
