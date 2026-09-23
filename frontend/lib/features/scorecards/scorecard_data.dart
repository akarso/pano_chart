/// Minimum graded sample before a reliability chip is shown.
const int reliabilityMinN = 30;

/// Hit-rate gap, in basis points, that paints a chip green or red.
const int reliabilityEdgeBasisPoints = 500;

/// Minimum confidence at which the backend logs a setup signal.
const double setupSignalConfidence = 0.5;

/// Chip color from `hitRate − baseline`.
enum ReliabilityTone { green, grey, red }

/// One `(kind, label)` line from `GET /api/scorecards/summary`.
class ScorecardSummaryItem {
  final String kind;
  final String label;
  final double hitRate;
  final double? baseline;
  final int n;

  const ScorecardSummaryItem({
    required this.kind,
    required this.label,
    required this.hitRate,
    required this.baseline,
    required this.n,
  });

  factory ScorecardSummaryItem.fromJson(Map<String, dynamic> json) {
    return ScorecardSummaryItem(
      kind: json['kind'] as String? ?? '',
      label: json['label'] as String? ?? '',
      hitRate: (json['hitRate'] as num?)?.toDouble() ?? 0,
      baseline: _nullableDouble(json['baseline']),
      n: (json['n'] as num?)?.toInt() ?? 0,
    );
  }
}

/// Summary payload. `since` is the window frozen in the cached response.
class ScorecardSummary {
  final String timeframe;
  final String since;
  final String sinceRaw;
  final List<ScorecardSummaryItem> items;

  const ScorecardSummary({
    required this.timeframe,
    required this.since,
    required this.sinceRaw,
    required this.items,
  });

  factory ScorecardSummary.fromJson(Map<String, dynamic> json) {
    return ScorecardSummary(
      timeframe: json['timeframe'] as String? ?? '',
      since: json['since'] as String? ?? '',
      sinceRaw: json['sinceRaw'] as String? ?? '',
      items: _summaryItems(json['items']),
    );
  }
}

/// One score decile from `GET /api/scorecards`.
class ScorecardBucket {
  final double lo;
  final double hi;
  final int n;
  final int hits;
  final double hitRate;
  final double avgReturn;

  const ScorecardBucket({
    required this.lo,
    required this.hi,
    required this.n,
    required this.hits,
    required this.hitRate,
    required this.avgReturn,
  });

  factory ScorecardBucket.fromJson(Map<String, dynamic> json) {
    return ScorecardBucket(
      lo: (json['lo'] as num?)?.toDouble() ?? 0,
      hi: (json['hi'] as num?)?.toDouble() ?? 0,
      n: (json['n'] as num?)?.toInt() ?? 0,
      hits: (json['hits'] as num?)?.toInt() ?? 0,
      hitRate: (json['hitRate'] as num?)?.toDouble() ?? 0,
      avgReturn: (json['avgReturn'] as num?)?.toDouble() ?? 0,
    );
  }
}

/// Full scorecard, including deciles.
class ScorecardDetail {
  final String kind;
  final String label;
  final String timeframe;
  final String since;
  final String sinceRaw;
  final int total;
  final int hits;
  final double hitRate;
  final double? baseline;
  final List<ScorecardBucket> buckets;

  const ScorecardDetail({
    required this.kind,
    required this.label,
    required this.timeframe,
    required this.since,
    required this.sinceRaw,
    required this.total,
    required this.hits,
    required this.hitRate,
    required this.baseline,
    required this.buckets,
  });

  factory ScorecardDetail.fromJson(Map<String, dynamic> json) {
    return ScorecardDetail(
      kind: json['kind'] as String? ?? '',
      label: json['label'] as String? ?? '',
      timeframe: json['timeframe'] as String? ?? '',
      since: json['since'] as String? ?? '',
      sinceRaw: json['sinceRaw'] as String? ?? '',
      total: (json['total'] as num?)?.toInt() ?? 0,
      hits: (json['hits'] as num?)?.toInt() ?? 0,
      hitRate: (json['hitRate'] as num?)?.toDouble() ?? 0,
      baseline: _nullableDouble(json['baseline']),
      buckets: _detailBuckets(json['buckets']),
    );
  }
}

/// Drops a non-object or unreadable entry so one bad element does not
/// throw away the rest of the summary.
List<ScorecardSummaryItem> _summaryItems(Object? raw) {
  if (raw is! List) return const [];
  final items = <ScorecardSummaryItem>[];
  for (final entry in raw) {
    if (entry is! Map) continue;
    try {
      items.add(
        ScorecardSummaryItem.fromJson(Map<String, dynamic>.from(entry)),
      );
    } catch (_) {
      continue;
    }
  }
  return items;
}

/// Drops a non-object or unreadable bucket so one bad element does not
/// throw away the rest of the deciles.
List<ScorecardBucket> _detailBuckets(Object? raw) {
  if (raw is! List) return const [];
  final buckets = <ScorecardBucket>[];
  for (final entry in raw) {
    if (entry is! Map) continue;
    try {
      buckets.add(ScorecardBucket.fromJson(Map<String, dynamic>.from(entry)));
    } catch (_) {
      continue;
    }
  }
  return buckets;
}

double? _nullableDouble(Object? raw) {
  if (raw == null) return null;
  if (raw is num) return raw.toDouble();
  return null;
}

/// Index summary rows by `kind|label`.
Map<String, ScorecardSummaryItem> indexScorecards(
  List<ScorecardSummaryItem> items,
) {
  return {for (final item in items) scorecardKey(item.kind, item.label): item};
}

String scorecardKey(String kind, String label) => '$kind|$label';

/// Null when the chip must stay hidden (`n < 30`).
/// Green when hit rate beats baseline by at least 5 points, red when it
/// trails by more than 5 points, grey inside that band or when baseline is
/// null. The gap is compared in integer basis points so values that are
/// 5 points apart in decimal do not fall on the wrong side of a raw double cut.
ReliabilityTone? reliabilityTone(ScorecardSummaryItem item) {
  if (item.n < reliabilityMinN) return null;
  final baseline = item.baseline;
  if (baseline == null) return ReliabilityTone.grey;
  final delta =
      reliabilityBasisPoints(item.hitRate) - reliabilityBasisPoints(baseline);
  if (delta >= reliabilityEdgeBasisPoints) return ReliabilityTone.green;
  if (delta < -reliabilityEdgeBasisPoints) return ReliabilityTone.red;
  return ReliabilityTone.grey;
}

/// Hundredths of a percentage point. 500 is five percentage points.
int reliabilityBasisPoints(double rate) => (rate * 10000).round();

/// Pill text: `58% · n=412`.
String reliabilityChipLabel(ScorecardSummaryItem item) {
  final pct = (item.hitRate * 100).round();
  return '$pct% · n=${item.n}';
}

/// Dialog copy for a chip tap.
/// Regime rows are change events, not times the headline stayed on screen.
String reliabilityExplanation(ScorecardSummaryItem item) {
  final hit = (item.hitRate * 100).round();
  final chance = item.baseline == null
      ? '—'
      : '${(item.baseline! * 100).round()}%';
  if (item.kind == 'regime') {
    return 'Of the last ${item.n} regime changes, $hit% worked out. '
        'Chance: $chance.';
  }
  return 'Of the last ${item.n} times the app showed this, $hit% worked out. '
      'Chance: $chance.';
}

/// `Regime · trend` for `regime:trend`. A label that already starts with the
/// kind is not prefixed again.
String scorecardRowTitle(String kind, String label) {
  var rest = label;
  final prefix = '$kind:';
  if (rest.startsWith(prefix)) {
    rest = rest.substring(prefix.length);
  }
  final pretty = rest.replaceAll('_', ' ');
  if (kind.isEmpty) return pretty;
  if (pretty.isEmpty) return _capitalize(kind);
  return '${_capitalize(kind)} · $pretty';
}

String reliabilityWindowLabel(String sinceRaw) {
  final raw = sinceRaw.trim();
  if (raw.isEmpty) return 'last 30d';
  return 'last $raw';
}

/// Setup chip sample. Null when this card was not logged (confidence < 0.5).
ScorecardSummaryItem? setupReliabilityItem({
  required double confidence,
  required String label,
  required Map<String, ScorecardSummaryItem> items,
}) {
  if (confidence < setupSignalConfidence) return null;
  return items[scorecardKey('setup', label)];
}

String _capitalize(String value) {
  if (value.isEmpty) return value;
  return value[0].toUpperCase() + value.substring(1);
}

/// Badge label logged by the backend (`BadgeLabel`).
String badgeScorecardLabel(String component, List<double> sparkline) {
  switch (component) {
    case 'trend':
      if (sparkline.length >= 2 && sparkline.last < sparkline.first) {
        return 'trend_down';
      }
      return 'trend_up';
    case 'sideways':
      return 'sideways';
    case 'gain':
      return 'gain';
    default:
      return component;
  }
}

/// Setup label logged by the backend (`SetupLabel`).
String setupScorecardLabel({
  required String bestSetup,
  required String regime,
  required double breakoutUp,
  required double breakoutDown,
}) {
  switch (bestSetup) {
    case 'compression_breakout':
      if (breakoutUp >= 0.5 && breakoutUp >= breakoutDown) {
        return 'breakout_up';
      }
      if (breakoutDown >= 0.5 && breakoutDown > breakoutUp) {
        return 'breakout_down';
      }
      return 'compression';
    case 'range_reversion':
      return 'range';
    case 'trend_continuation':
      switch (regime) {
        case 'downtrend':
          return 'trend_down';
        case 'uptrend':
          return 'trend_up';
        default:
          if (breakoutDown > breakoutUp) return 'trend_down';
          return 'trend_up';
      }
    default:
      return bestSetup;
  }
}

/// Regime label logged by the backend (`regime:<name>`).
String regimeScorecardLabel(String regime) => 'regime:$regime';
