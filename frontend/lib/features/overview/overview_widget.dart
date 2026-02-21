import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter_svg/flutter_svg.dart';
import '../../domain/symbol.dart';
import '../../domain/timeframe.dart';
import '../../infrastructure/preferences_service.dart';
import '../candles/application/get_candle_series.dart';
import '../candles/application/get_candle_series_input.dart';
import '../detail/detail_screen.dart';
import '../detail/detail_context.dart';
import 'overview_state.dart';
import 'overview_view_model.dart';

/// Overview widget that displays a scrollable grid of market sparklines.
///
/// All data and loading state is owned by [OverviewViewModel].
/// Widget rebuilds via [OverviewViewModel.onChanged] callback.
class OverviewWidget extends StatefulWidget {
  final OverviewViewModel viewModel;
  final GetCandleSeries getCandleSeries;
  final PreferencesService? prefs;

  const OverviewWidget({
    Key? key,
    required this.viewModel,
    required this.getCandleSeries,
    this.prefs,
  }) : super(key: key);

  @override
  OverviewWidgetState createState() => OverviewWidgetState();
}

/// Which overlay is currently visible.
enum _OverlayKind { none, settings, menu }

class OverviewWidgetState extends State<OverviewWidget>
    with SingleTickerProviderStateMixin {
  late final OverviewViewModel vm;
  final ScrollController _scrollController = ScrollController();
  int _columns = 2;
  String _timeframe = '1h';
  String _sidewaysAlgo = 'v1';
  bool _normalizeSparklines = true;
  bool _showFavourites = false;
  Set<String> _favourites = {};

  /// Which overlay panel is open (none by default).
  _OverlayKind _overlay = _OverlayKind.none;

  /// Threshold in pixels from bottom to trigger loading more items.
  static const double _scrollThreshold = 200.0;

  PreferencesService? get _prefs => widget.prefs;

  @override
  void initState() {
    super.initState();
    vm = widget.viewModel;

    // Attach prefs to view model for offline cache
    vm.attachPrefs(_prefs);

    // Restore persisted settings.
    final p = _prefs;
    if (p != null) {
      _columns = p.columns;
      _timeframe = p.timeframe;
      _sidewaysAlgo = p.sidewaysAlgo;
      _normalizeSparklines = p.normalizeSparklines;
      _favourites = p.favourites;

      // Also sync sort + sidewaysAlgo into the view model state so the
      // first loadInitial uses the persisted values.
      if (p.sort != vm.state.sort) {
        vm.changeSortSilent(p.sort);
      }
      if (p.sidewaysAlgo != vm.state.sidewaysAlgo) {
        vm.changeSidewaysAlgoSilent(p.sidewaysAlgo);
      }
    }

    vm.onChanged = () {
      setState(() {});
      // After new items are rendered, check if we still need more to fill the viewport.
      SchedulerBinding.instance.addPostFrameCallback((_) {
        _checkAndLoadMore();
      });
    };
    _scrollController.addListener(_onScroll);
    vm.loadInitial(_timeframe);
  }

  @override
  void dispose() {
    vm.onChanged = null;
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    super.dispose();
  }

  // ---- scroll helpers ----

  void _onScroll() {
    _checkAndLoadMore();
  }

  void _checkAndLoadMore() {
    if (!_scrollController.hasClients) return;
    final pos = _scrollController.position;
    if (pos.pixels >= pos.maxScrollExtent - _scrollThreshold) {
      if (!vm.state.isLoading && vm.state.hasMore) {
        vm.loadNext(_timeframe);
      }
    }
  }

  // ---- overlay helpers ----

  void _toggleOverlay(_OverlayKind kind) {
    setState(() {
      _overlay = _overlay == kind ? _OverlayKind.none : kind;
    });
  }

  // ---- navigation helpers ----

  Duration _candleDuration(String tf) {
    switch (tf) {
      case '1m':
        return const Duration(minutes: 1);
      case '5m':
        return const Duration(minutes: 5);
      case '15m':
        return const Duration(minutes: 15);
      case '1h':
        return const Duration(hours: 1);
      case '4h':
        return const Duration(hours: 4);
      case '1d':
        return const Duration(days: 1);
      default:
        return const Duration(hours: 1);
    }
  }

  /// Number of candles the overview sparkline covers.
  static const int _sparklineCandles = 30;

  /// Standard intervals ordered by duration (ascending).
  static const List<String> _intervals = ['1m', '5m', '15m', '1h', '4h', '1d'];

  /// Returns a finer-grained timeframe for the detail chart that covers the
  /// same time window as the overview sparkline (30 × overview-TF) but with
  /// more candles.  Picks the closest standard interval whose duration is
  /// strictly less than the overview TF; falls back to the overview TF itself
  /// when already at the smallest interval.
  String _detailTimeframe(String overviewTf) {
    final idx = _intervals.indexOf(overviewTf);
    // Already at finest interval, or unknown → keep as-is.
    if (idx <= 0) return overviewTf;
    return _intervals[idx - 1];
  }

  Future<void> _onItemTapped(OverviewItem item) async {
    final now = DateTime.now().toUtc();
    // Time window = what the sparkline covers.
    final timespan = _candleDuration(_timeframe) * _sparklineCandles;
    final from = now.subtract(timespan);
    final detailTf = _detailTimeframe(_timeframe);
    final input = GetCandleSeriesInput(
      symbol: item.symbol,
      timeframe: detailTf,
      from: from,
      to: now,
    );

    // Compute rank = 1-based position in current list.
    final rankIndex = vm.state.items.indexOf(item);
    final rank = rankIndex >= 0 ? rankIndex + 1 : 0;

    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (_) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final series = await widget.getCandleSeries.execute(input);
      if (!mounted) return;
      Navigator.of(context).pop();
      final result = await Navigator.of(context).push<bool>(
        MaterialPageRoute(
          builder: (_) => DetailScreen(
            symbol: AppSymbol(item.symbol),
            timeframe: Timeframe(detailTf),
            series: series,
            isFavourite: _favourites.contains(item.symbol),
            detailContext: DetailContext(
              rank: rank,
              totalScore: item.totalScore,
              trendScore: item.trendScore,
              sidewaysScore: item.sidewaysScore,
              gainScore: item.gainScore,
              volume: item.volume,
            ),
          ),
        ),
      );
      // Update favourites from detail screen result.
      if (result != null && mounted) {
        setState(() {
          if (result) {
            _favourites.add(item.symbol);
          } else {
            _favourites.remove(item.symbol);
          }
          _prefs?.favourites = _favourites;
        });
      }
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load chart: $e')),
      );
    }
  }

  // ---- pull-to-refresh ----

  Future<void> _onRefresh() async {
    await vm.refresh(_timeframe);
  }

  // ---- sparkline helpers ----

  /// Compute the maximum absolute % change across all visible sparklines.
  /// This provides a shared y-axis scale for non-normalized mode.
  double _globalMaxPctChange(OverviewState state) {
    double maxPct = 0.0;
    for (final item in state.items) {
      if (item.sparkline.length < 2 || item.sparkline.first == 0) continue;
      final first = item.sparkline.first;
      for (final p in item.sparkline) {
        final pct = ((p - first) / first).abs();
        if (pct > maxPct) maxPct = pct;
      }
    }
    // Floor at 0.1% to avoid division by zero on flat data.
    return maxPct < 0.001 ? 0.001 : maxPct;
  }

  // ---- build ----

  @override
  Widget build(BuildContext context) {
    final state = vm.state;

    return SafeArea(
      bottom: false,
      child: Column(
        children: [
          // Nav bar + overlay unit — bottom border moves with rollout
          Container(
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: Colors.grey, width: 1)),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _buildNavBar(),
                _buildOverlayPanel(state),
              ],
            ),
          ),
          Expanded(child: _buildBody(state)),
        ],
      ),
    );
  }

  // ---- navigation bar ----

  Widget _buildNavBar() {
    const barHeight = 50.0;
    return Container(
      height: barHeight,
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Colors.black, Color(0xFF333333)],
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 12),
      child: Row(
        children: [
          // Logo + branding
          Padding(
            padding: const EdgeInsets.only(left: 0, right: 8),
            child: Row(
              children: [
                Container(
                  margin: const EdgeInsets.only(right: 15),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(3),
                    child: Image.asset(
                      'assets/icon.png',
                      width: 26,
                      height: 26,
                    ),
                  ),
                ),
                const Text(
                  'Pano Charts',
                  style: TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF00e6c0),
                    letterSpacing: 0.5,
                  ),
                ),
              ],
            ),
          ),
          const Spacer(),
          // Favourites toggle
          GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () => setState(() => _showFavourites = !_showFavourites),
            child: SizedBox(
              width: 44,
              height: 44,
              child: Center(
                child: Icon(
                  _showFavourites ? Icons.star : Icons.star_border,
                  color: _showFavourites ? Colors.amber : Colors.white,
                  size: 22,
                ),
              ),
            ),
          ),
          const SizedBox(width: 8),
          // Settings icon
          _NavBarIcon(
            isActive: _overlay == _OverlayKind.settings,
            svgAsset: 'assets/gear-setting-settings.svg',
            onTap: () => _toggleOverlay(_OverlayKind.settings),
          ),
          const SizedBox(width: 8),
          // Menu icon
          _NavBarIcon(
            isActive: _overlay == _OverlayKind.menu,
            svgAsset: 'assets/menu.svg',
            onTap: () => _toggleOverlay(_OverlayKind.menu),
          ),
          const SizedBox(width: 4),
        ],
      ),
    );
  }

  // ---- overlay panel ----

  Widget _buildOverlayPanel(OverviewState state) {
    return AnimatedSize(
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeInOut,
      alignment: Alignment.topCenter,
      clipBehavior: Clip.hardEdge,
      child: _overlay != _OverlayKind.none
          ? Container(
              key: ValueKey(_overlay),
              width: double.infinity,
              decoration: BoxDecoration(
                color: const Color(0xFF333333).withAlpha((0.9 * 255).round()),
              ),
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: _overlay == _OverlayKind.settings
                  ? _buildSettingsOverlay(state)
                  : _buildMenuOverlay(),
            )
          : const SizedBox.shrink(),
    );
  }

  Widget _buildSettingsOverlay(OverviewState state) {
    final screenWidth = MediaQuery.of(context).size.width;
    // Font size proportional to screen width (~3.5vw), floor 11, cap 16.
    final ctrlFontSize = (screenWidth * 0.035).clamp(11.0, 16.0);

    return DefaultTextStyle.merge(
      style: TextStyle(fontSize: ctrlFontSize),
      child: Theme(
        data: Theme.of(context).copyWith(
          dropdownMenuTheme: const DropdownMenuThemeData(),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                _controlRow('Columns', DropdownButton<int>(
                  value: _columns,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const [1, 2, 3]
                      .map((c) => DropdownMenuItem(value: c, child: Text('$c')))
                      .toList(),
                  onChanged: (v) {
                    setState(() => _columns = v ?? 2);
                    _prefs?.columns = _columns;
                    SchedulerBinding.instance.addPostFrameCallback((_) {
                      _checkAndLoadMore();
                    });
                  },
                ), ctrlFontSize),
                _controlRow('Timeframe', DropdownButton<String>(
                  value: _timeframe,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const ['1m', '5m', '15m', '1h', '4h', '1d']
                      .map((tf) => DropdownMenuItem(value: tf, child: Text(tf)))
                      .toList(),
                  onChanged: (v) {
                    setState(() => _timeframe = v ?? '1h');
                    _prefs?.timeframe = _timeframe;
                    vm.loadInitial(_timeframe);
                  },
                ), ctrlFontSize),
                _controlRow('Sort', DropdownButton<String>(
                  value: state.sort,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const [
                    DropdownMenuItem(value: 'total', child: Text('Total')),
                    DropdownMenuItem(value: 'gain', child: Text('Gain')),
                    DropdownMenuItem(value: 'trend', child: Text('Trend')),
                    DropdownMenuItem(value: 'sideways', child: Text('Sideways')),
                    DropdownMenuItem(value: 'volume', child: Text('Volume')),
                  ],
                  onChanged: (v) {
                    if (v != null) {
                      _prefs?.sort = v;
                      vm.changeSort(v, _timeframe);
                    }
                  },
                ), ctrlFontSize),
              ],
            ),
            const SizedBox(height: 20),
            CustomPaint(
              painter: _DottedLinePainter(color: const Color(0xFF666666)),
              size: const Size(double.infinity, 1),
            ),
            const SizedBox(height: 20),
            Row(
              children: [
                SizedBox(
                  height: 24,
                  width: 24,
                  child: Checkbox(
                    value: _normalizeSparklines,
                    onChanged: (v) {
                      setState(() => _normalizeSparklines = v ?? true);
                      _prefs?.normalizeSparklines = _normalizeSparklines;
                    },
                    materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    visualDensity: VisualDensity.compact,
                  ),
                ),
                const SizedBox(width: 6),
                GestureDetector(
                  onTap: () {
                    setState(() => _normalizeSparklines = !_normalizeSparklines);
                    _prefs?.normalizeSparklines = _normalizeSparklines;
                  },
                  child: Text(
                    'Normalize sparklines',
                    style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  ),
                ),
                const SizedBox(width: 16),
                _controlRow('Sideways', DropdownButton<String>(
                  value: _sidewaysAlgo,
                  isDense: true,
                  style: TextStyle(fontSize: ctrlFontSize, color: Colors.white),
                  items: const [
                    DropdownMenuItem(value: 'v1', child: Text('Algo 1')),
                    DropdownMenuItem(value: 'v2', child: Text('Algo 2')),
                    DropdownMenuItem(value: 'v3', child: Text('Algo 3')),
                  ],
                  onChanged: (v) {
                    if (v != null) {
                      setState(() => _sidewaysAlgo = v);
                      _prefs?.sidewaysAlgo = v;
                      vm.changeSidewaysAlgo(v, _timeframe);
                    }
                  },
                ), ctrlFontSize),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _controlRow(String label, Widget dropdown, double fontSize) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label, style: TextStyle(fontSize: fontSize)),
        const SizedBox(width: 4),
        dropdown,
      ],
    );
  }

  Widget _buildMenuOverlay() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        TextButton.icon(
          onPressed: () => _showInfoDialog(
            title: 'About',
            body: 'Simple market screener app showcasing a custom technical analysis algorithm. '
                'Built, because I was lacking exactly such a set of tools for my own trading decisions. ',
          ),
          icon: const Icon(Icons.info_outline, size: 18),
          label: const Text('About'),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: CustomPaint(
            size: const Size(double.infinity, 1),
            painter: _DottedLinePainter(color: const Color(0xFF666666)),
          ),
        ),
        TextButton.icon(
          onPressed: () => _showInfoDialog(
            title: 'Help',
            body:
                'Pull down to refresh data. Tap on any chart to see detailed view with score breakdown and more info. '
                'Scroll to load more items (max 150 tickers). Use settings to change sort, timeframe, and other options.',
          ),
          icon: const Icon(Icons.help_outline, size: 18),
          label: const Text('Help'),
        ),
      ],
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

  // ---- main body ----

  Widget _buildBody(OverviewState state) {
    if (state.isLoading && state.items.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }

    if (state.error != null && state.items.isEmpty) {
      return Center(child: Text(state.error!));
    }

    final allItems = state.items;
    final visibleItems = _showFavourites
        ? allItems.where((i) => _favourites.contains(i.symbol)).toList()
        : allItems;

    if (_showFavourites && visibleItems.isEmpty) {
      return const Center(
        child: Text(
          'No favourites yet.\nTap ★ on any detail screen to add.',
          textAlign: TextAlign.center,
          style: TextStyle(color: Colors.white38, fontSize: 14),
        ),
      );
    }

    final spacing = _columns == 3 ? 4.0 : 8.0;
    return RefreshIndicator(
      onRefresh: _onRefresh,
      child: GridView.builder(
        physics: const AlwaysScrollableScrollPhysics(),
        controller: _scrollController,
        padding: const EdgeInsets.all(8),
        gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
          crossAxisCount: _columns,
          crossAxisSpacing: spacing,
          mainAxisSpacing: spacing,
          childAspectRatio: 2.5,
        ),
        itemCount: visibleItems.length +
            (!_showFavourites && state.hasMore ? 1 : 0),
        itemBuilder: (context, index) {
          if (index >= visibleItems.length) {
            return const Center(child: CircularProgressIndicator());
          }
          final item = visibleItems[index];
          return GestureDetector(
            onTap: () => _onItemTapped(item),
            child: _OverviewGridItem(
              item: item,
              columns: _columns,
              normalize: _normalizeSparklines,
              globalMaxPct: _globalMaxPctChange(state),
              isFavourite: _favourites.contains(item.symbol),
            ),
          );
        },
      ),
    );
  }
}

// ---- nav bar icon widget ----

class _NavBarIcon extends StatelessWidget {
  final bool isActive;
  final String svgAsset;
  final VoidCallback onTap;

  const _NavBarIcon({
    required this.isActive,
    required this.svgAsset,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: SizedBox(
        width: 44,
        height: 44,
        child: Center(
          child: isActive
              ? const Icon(Icons.close, color: Colors.white, size: 22)
              : SvgPicture.asset(
                  svgAsset,
                  width: 20,
                  height: 20,
                  colorFilter: const ColorFilter.mode(
                    Colors.white,
                    BlendMode.srcIn,
                  ),
                ),
        ),
      ),
    );
  }
}

/// Dominant signal type derived from score breakdown.
enum SignalType { trend, sideways, gain }

/// Returns the dominant signal for an [OverviewItem].
SignalType dominantSignal(OverviewItem item) {
  final entries = {
    SignalType.trend: item.trendScore,
    SignalType.sideways: item.sidewaysScore,
    SignalType.gain: item.gainScore,
  };
  return entries.entries.reduce((a, b) => a.value >= b.value ? a : b).key;
}

/// Parses a backend badge component string into a [SignalType].
SignalType _parseSignalType(String component) {
  switch (component) {
    case 'trend':
      return SignalType.trend;
    case 'sideways':
      return SignalType.sideways;
    case 'gain':
      return SignalType.gain;
    default:
      return SignalType.trend;
  }
}

Color _signalColor(SignalType signal) {
  switch (signal) {
    case SignalType.trend:
      return Colors.blue;
    case SignalType.gain:
      return Colors.green;
    case SignalType.sideways:
      return Colors.orange;
  }
}

String _signalLabel(SignalType signal, {bool abbreviate = false}) {
  switch (signal) {
    case SignalType.trend:
      return abbreviate ? 'T' : 'TREND';
    case SignalType.gain:
      return abbreviate ? 'G' : 'GAIN';
    case SignalType.sideways:
      return abbreviate ? 'S' : 'SIDEWAYS';
  }
}

class _OverviewGridItem extends StatelessWidget {
  final OverviewItem item;
  final int columns;
  final bool normalize;
  final double globalMaxPct;
  final bool isFavourite;

  const _OverviewGridItem({
    required this.item,
    required this.columns,
    required this.normalize,
    required this.globalMaxPct,
    this.isFavourite = false,
  });

  @override
  Widget build(BuildContext context) {
    final borderRadius = columns == 3 ? 6.0 : 12.0;
    return Card(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(borderRadius),
      ),
      child: AspectRatio(
        aspectRatio: 2.5,
        child: LayoutBuilder(
          builder: (context, constraints) {
            // Scale font proportionally to card width.
            final fontSize = (constraints.maxWidth * 0.08).clamp(9.0, 18.0);
            final pad = (constraints.maxWidth * 0.03).clamp(4.0, 12.0);
            return Stack(
              children: [
                Padding(
                  padding: EdgeInsets.all(pad),
                  child: _buildSparkline(item.sparkline),
                ),
                Positioned(
                  left: pad + 4,
                  top: pad,
                  child: Text(
                    item.symbol.replaceAll('USDT', ''),
                    style: TextStyle(
                      fontSize: fontSize,
                      fontWeight: FontWeight.w600,
                      color: Colors.white.withAlpha(
                        ((columns == 1 ? 0.9 : columns == 2 ? 0.8 : 0.7) * 255).round(),
                      ),
                      backgroundColor:
                          Colors.black.withAlpha((0.25 * 255).round()),
                    ),
                  ),
                ),
                if (item.badgeComponent.isNotEmpty)
                  Positioned(
                    right: pad + 4,
                    top: pad,
                    child: _buildBadge(item, fontSize),
                  ),
                if (isFavourite)
                  Positioned(
                    left: pad + 4,
                    bottom: pad,
                    child: Icon(
                      Icons.star,
                      color: Colors.amber.withAlpha((0.8 * 255).round()),
                      size: (fontSize * 0.8).clamp(10.0, 16.0),
                    ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _buildSparkline(List<double> points) {
    if (points.isEmpty) return const Center(child: Text('No data'));
    return CustomPaint(
      painter: SparklineRenderer(
        points,
        normalize: normalize,
        globalMaxPct: globalMaxPct,
      ),
      size: Size.infinite,
    );
  }

  bool _hasScores(OverviewItem item) {
    return item.trendScore != 0 ||
        item.sidewaysScore != 0 ||
        item.gainScore != 0;
  }

  Widget _buildBadge(OverviewItem item, double fontSize) {
    final signal = _parseSignalType(item.badgeComponent);
    final scale = columns == 1 ? 1.0 : columns == 2 ? 0.9 : 0.8;
    final badgeFontSize = (fontSize * 0.7 * scale).clamp(7.0, 12.0);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
      decoration: BoxDecoration(
        color: _signalColor(signal).withAlpha((0.8 * 255).round()),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        _signalLabel(signal, abbreviate: columns > 1),
        style: TextStyle(
          fontSize: badgeFontSize,
          fontWeight: FontWeight.bold,
          color: Colors.white,
        ),
      ),
    );
  }
}

/// Draws a simple sparkline (line chart) from a list of values.
///
/// When [normalize] is true (default), the sparkline fills the full height
/// using min-max scaling. When false, values are converted to % change
/// from the first point and scaled relative to [globalMaxPct], so that
/// visually smaller moves appear smaller.
class SparklineRenderer extends CustomPainter {
  final List<double> points;
  final bool normalize;
  final double globalMaxPct;

  SparklineRenderer(
    this.points, {
    this.normalize = true,
    this.globalMaxPct = 0.05,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (points.length < 2) return;

    final paint = Paint()
      ..color = points.last >= points.first ? Colors.green : Colors.red
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final path = Path();

    if (normalize) {
      // Original min-max normalization — fills full height.
      final minVal = points.reduce((a, b) => a < b ? a : b);
      final maxVal = points.reduce((a, b) => a > b ? a : b);
      final range = (maxVal - minVal) == 0 ? 1.0 : (maxVal - minVal);

      for (var i = 0; i < points.length; i++) {
        final x = (i / (points.length - 1)) * size.width;
        final y = size.height - ((points[i] - minVal) / range) * size.height;
        if (i == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
    } else {
      // Percentage mode — height reflects actual % change relative to
      // the global maximum % change across all visible sparklines.
      final first = points.first;
      final maxPct = globalMaxPct;

      for (var i = 0; i < points.length; i++) {
        final x = (i / (points.length - 1)) * size.width;
        final pct = first == 0 ? 0.0 : (points[i] - first) / first;
        // Map [-maxPct, +maxPct] to [height, 0] (top = +maxPct).
        var ratio = (pct + maxPct) / (2 * maxPct);
        // Clamp to [0, 1] for outliers beyond ±globalMaxPct.
        if (ratio < 0) ratio = 0;
        if (ratio > 1) ratio = 1;
        final y = size.height - ratio * size.height;
        if (i == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
    }

    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant SparklineRenderer oldDelegate) {
    return !identical(points, oldDelegate.points) ||
        normalize != oldDelegate.normalize ||
        globalMaxPct != oldDelegate.globalMaxPct;
  }
}

/// Draws a horizontal dotted line for menu dividers.
class _DottedLinePainter extends CustomPainter {
  final Color color;

  _DottedLinePainter({required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;
    const dashWidth = 4.0;
    const dashGap = 3.0;
    double x = 0;
    while (x < size.width) {
      canvas.drawLine(Offset(x, 0), Offset(x + dashWidth, 0), paint);
      x += dashWidth + dashGap;
    }
  }

  @override
  bool shouldRepaint(covariant _DottedLinePainter old) => color != old.color;
}
