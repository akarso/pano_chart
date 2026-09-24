/// Count-based market participation (PR-115 / PR-116).
class ParticipationCounts {
  final int up;
  final int down;
  final int ranging;
  final int total;

  const ParticipationCounts({
    required this.up,
    required this.down,
    required this.ranging,
    required this.total,
  });

  static const empty = ParticipationCounts(
    up: 0,
    down: 0,
    ranging: 0,
    total: 0,
  );

  /// Tolerant parse: wrong JSON types yield [empty]; counts are clamped ≥ 0.
  /// When `total` disagrees with `up+down+ranging`, total is renormalized to
  /// the parts sum so shares never exceed 100%.
  factory ParticipationCounts.fromJson(dynamic json) {
    if (json is! Map) return empty;
    final map = Map<String, dynamic>.from(json);
    int read(String key) {
      final v = map[key];
      if (v is! num) return 0;
      final n = v.toInt();
      return n < 0 ? 0 : n;
    }

    final up = read('up');
    final down = read('down');
    final ranging = read('ranging');
    final parts = up + down + ranging;
    if (parts == 0) return empty;
    final reported = read('total');
    final total = reported == parts ? reported : parts;
    return ParticipationCounts(
      up: up,
      down: down,
      ranging: ranging,
      total: total,
    );
  }

  /// True when there is something to draw.
  bool get hasData => (up + down + ranging) > 0;

  int get _parts => up + down + ranging;

  double get upShare => _parts == 0 ? 0 : up / _parts;
  double get downShare => _parts == 0 ? 0 : down / _parts;
  double get rangingShare => _parts == 0 ? 0 : ranging / _parts;
}

/// Rounded Up / Ranging / Down percents that sum to 100 (largest remainder).
(int upPct, int rangingPct, int downPct) participationPercents(
  ParticipationCounts p,
) {
  if (!p.hasData) return (0, 0, 0);
  final parts = p.up + p.down + p.ranging;
  final raw = [
    (p.up / parts) * 100,
    (p.ranging / parts) * 100,
    (p.down / parts) * 100,
  ];
  final floors = raw.map((x) => x.floor()).toList();
  var rem = 100 - floors.reduce((a, b) => a + b);
  final order = List.generate(3, (i) => i)
    ..sort((a, b) => (raw[b] - floors[b]).compareTo(raw[a] - floors[a]));
  for (var i = 0; i < rem; i++) {
    floors[order[i]]++;
  }
  return (floors[0], floors[1], floors[2]);
}

/// One-line reading under the participation bar (PR-116).
///
/// Thresholds use the same rounded percents as the bar label. [bias] /
/// [regime] are the headline tape fields — "with the tape" only when the
/// lead side matches a trend bias. An up/down tie never claims agreement
/// or disagreement with the tape.
String participationReading(
  ParticipationCounts p, {
  String bias = '',
  String regime = '',
}) {
  if (!p.hasData) return '';
  final (upPct, _, downPct) = participationPercents(p);
  if (upPct >= 30 && downPct >= 30) {
    return 'Split market';
  }
  final tied = upPct == downPct;
  final leadIsUp = upPct > downPct;
  final leadIsDown = downPct > upPct;
  final lead = leadIsUp ? upPct : downPct;
  final tapeIsTrend = regime == 'trend';
  final matchesTape = !tied &&
      tapeIsTrend &&
      ((leadIsUp && bias == 'up') || (leadIsDown && bias == 'down'));
  final againstTape = !tied &&
      tapeIsTrend &&
      ((leadIsUp && bias == 'down') || (leadIsDown && bias == 'up'));

  if (lead >= 50) {
    if (matchesTape) {
      return 'Broad move — most tokens trend with the tape';
    }
    if (againstTape) {
      return 'Broad move against the tape';
    }
    return leadIsUp
        ? 'Broad up — most tokens are trending up'
        : 'Broad down — most tokens are trending down';
  }
  if (lead >= 30) {
    if (againstTape) {
      return 'Mixed — tokens lean against the tape';
    }
    if (matchesTape) {
      return 'Mixed — tape led by part of the market';
    }
    return 'Mixed — part of the market is trending';
  }
  if (againstTape) {
    return 'Narrow — a few tokens lean against the tape';
  }
  if (matchesTape) {
    return 'Narrow — tape led by a few heavyweights';
  }
  if (tied) {
    return 'Narrow — up and down are tied';
  }
  return 'Narrow — move led by a few tokens';
}
