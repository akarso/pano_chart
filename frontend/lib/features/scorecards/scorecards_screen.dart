import 'package:flutter/material.dart';

import 'http_scorecard_api.dart';
import 'reliability_chip.dart';
import 'scorecard_data.dart';

/// List of `(kind, label)` reliability rows for the last 30 days.
/// Tap a row to open its score-decile chart.
class ScorecardsScreen extends StatefulWidget {
  final ScorecardApi api;
  final String timeframe;

  const ScorecardsScreen({super.key, required this.api, this.timeframe = '1h'});

  @override
  State<ScorecardsScreen> createState() => _ScorecardsScreenState();
}

class _ScorecardsScreenState extends State<ScorecardsScreen> {
  ScorecardSummary? _summary;
  Object? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final summary = await widget.api.summary(
        timeframe: widget.timeframe,
        since: '30d',
      );
      if (!mounted) return;
      setState(() {
        _summary = summary;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(
        title: const Text('Reliability'),
        backgroundColor: const Color(0xFF1A1A1A),
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
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Reliability is unavailable.',
              style: TextStyle(color: Colors.white70),
            ),
            const SizedBox(height: 12),
            TextButton(onPressed: _load, child: const Text('Retry')),
          ],
        ),
      );
    }
    final summary = _summary;
    final items = summary?.items ?? const <ScorecardSummaryItem>[];
    final window = reliabilityWindowLabel(summary?.sinceRaw ?? '');
    if (items.isEmpty) {
      return Center(
        child: Text(
          'No graded calls in $window.',
          style: const TextStyle(color: Colors.white70),
        ),
      );
    }
    return ListView.separated(
      itemCount: items.length,
      separatorBuilder: (_, __) =>
          const Divider(height: 1, color: Colors.white12),
      itemBuilder: (context, index) {
        final item = items[index];
        final baseline = item.baseline == null
            ? '—'
            : '${(item.baseline! * 100).round()}%';
        return ListTile(
          title: Text(
            scorecardRowTitle(item.kind, item.label),
            style: const TextStyle(color: Colors.white),
          ),
          subtitle: Text(
            '${reliabilityChipLabel(item)} vs $baseline · $window',
            style: const TextStyle(color: Colors.white54),
          ),
          trailing: IgnorePointer(child: ReliabilityChip(item: item)),
          onTap: () {
            Navigator.of(context).push(
              MaterialPageRoute(
                builder: (_) => ScorecardDecilesScreen(
                  api: widget.api,
                  kind: item.kind,
                  label: item.label,
                  timeframe: widget.timeframe,
                ),
              ),
            );
          },
        );
      },
    );
  }
}

/// Decile hit-rate bars for one scorecard.
class ScorecardDecilesScreen extends StatefulWidget {
  final ScorecardApi api;
  final String kind;
  final String label;
  final String timeframe;

  const ScorecardDecilesScreen({
    super.key,
    required this.api,
    required this.kind,
    required this.label,
    required this.timeframe,
  });

  @override
  State<ScorecardDecilesScreen> createState() => _ScorecardDecilesScreenState();
}

class _ScorecardDecilesScreenState extends State<ScorecardDecilesScreen> {
  ScorecardDetail? _card;
  Object? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final card = await widget.api.get(
        kind: widget.kind,
        label: widget.label,
        timeframe: widget.timeframe,
        since: '30d',
      );
      if (!mounted) return;
      setState(() {
        _card = card;
        _error = null;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF121212),
      appBar: AppBar(
        title: Text(scorecardRowTitle(widget.kind, widget.label)),
        backgroundColor: const Color(0xFF1A1A1A),
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null || _card == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Could not load deciles.',
              style: TextStyle(color: Colors.white70),
            ),
            const SizedBox(height: 12),
            TextButton(onPressed: _load, child: const Text('Retry')),
          ],
        ),
      );
    }
    final card = _card!;
    final buckets = card.buckets;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(
          '${reliabilityChipLabel(ScorecardSummaryItem(kind: card.kind, label: card.label, hitRate: card.hitRate, baseline: card.baseline, n: card.total))} · ${reliabilityWindowLabel(card.sinceRaw)}',
          style: const TextStyle(color: Colors.white70),
        ),
        const SizedBox(height: 16),
        if (buckets.isEmpty)
          const Text(
            'No deciles in this window.',
            style: TextStyle(color: Colors.white54),
          )
        else
          for (final bucket in buckets) ...[
            _DecileBar(bucket: bucket),
            const SizedBox(height: 8),
          ],
      ],
    );
  }
}

class _DecileBar extends StatelessWidget {
  final ScorecardBucket bucket;

  const _DecileBar({required this.bucket});

  @override
  Widget build(BuildContext context) {
    final lo = (bucket.lo * 100).round();
    final hi = (bucket.hi * 100).round();
    final hit = (bucket.hitRate * 100).round();
    return Row(
      children: [
        SizedBox(
          width: 72,
          child: Text(
            '$lo–$hi',
            style: const TextStyle(color: Colors.white54, fontSize: 12),
          ),
        ),
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: bucket.hitRate.clamp(0.0, 1.0),
              minHeight: 8,
              backgroundColor: Colors.white10,
              valueColor: const AlwaysStoppedAnimation<Color>(
                Colors.tealAccent,
              ),
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 88,
          child: Text(
            '$hit% · n=${bucket.n}',
            textAlign: TextAlign.right,
            style: const TextStyle(color: Colors.white70, fontSize: 12),
          ),
        ),
      ],
    );
  }
}
