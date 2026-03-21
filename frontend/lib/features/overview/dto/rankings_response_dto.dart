/// DTO for the scores breakdown in a ranking item.
class RankingScoresDto {
  final double trend;
  final double sideways;
  final double gain;
  final double compression;
  final double breakoutUp;
  final double breakoutDown;

  const RankingScoresDto({
    required this.trend,
    required this.sideways,
    required this.gain,
    this.compression = 0.0,
    this.breakoutUp = 0.0,
    this.breakoutDown = 0.0,
  });

  factory RankingScoresDto.fromJson(Map<String, dynamic> json) {
    return RankingScoresDto(
      trend: (json['Trend Predictability'] as num?)?.toDouble() ?? 0.0,
      sideways: (json['Sideways Consistency'] as num?)?.toDouble() ?? 0.0,
      gain: (json['Gain/Loss'] as num?)?.toDouble() ?? 0.0,
      compression: (json['Compression'] as num?)?.toDouble() ?? 0.0,
      breakoutUp: (json['Breakout Up'] as num?)?.toDouble() ?? 0.0,
      breakoutDown: (json['Breakout Down'] as num?)?.toDouble() ?? 0.0,
    );
  }
}

/// DTO for a single item in the rankings response.
class RankingItemDto {
  final String symbol;
  final double totalScore;
  final RankingScoresDto scores;
  final double volume;
  final List<double> sparkline;
  final String badgeComponent;
  final double sidewaysPercentile;

  const RankingItemDto({
    required this.symbol,
    required this.totalScore,
    required this.scores,
    required this.volume,
    required this.sparkline,
    required this.badgeComponent,
    required this.sidewaysPercentile,
  });

  factory RankingItemDto.fromJson(Map<String, dynamic> json) {
    return RankingItemDto(
      symbol: json['symbol'] as String,
      totalScore: (json['totalScore'] as num).toDouble(),
      scores: RankingScoresDto.fromJson(
          json['scores'] as Map<String, dynamic>? ?? {}),
      volume: (json['volume'] as num?)?.toDouble() ?? 0.0,
      sparkline: (json['sparkline'] as List?)
              ?.map((e) => (e as num).toDouble())
              .toList() ??
          const [],
      badgeComponent: json['badgeComponent'] as String? ?? '',
      sidewaysPercentile: (json['sidewaysPercentile'] as num?)?.toDouble() ?? 0.0,
    );
  }
}

/// DTO for the GET /api/rankings response.
class RankingsResponseDto {
  final String timeframe;
  final String sort;
  final int page;
  final int pageSize;
  final int totalItems;
  final int totalPages;
  final int precision;
  final List<RankingItemDto> results;

  const RankingsResponseDto({
    required this.timeframe,
    required this.sort,
    required this.page,
    required this.pageSize,
    required this.totalItems,
    required this.totalPages,
    required this.precision,
    required this.results,
  });

  factory RankingsResponseDto.fromJson(Map<String, dynamic> json) {
    return RankingsResponseDto(
      timeframe: json['timeframe'] as String,
      sort: json['sort'] as String,
      page: json['page'] as int,
      pageSize: json['pageSize'] as int,
      totalItems: json['totalItems'] as int,
      totalPages: json['totalPages'] as int,
      precision: json['precision'] as int? ?? 0,
      results: (json['results'] as List)
          .map((e) => RankingItemDto.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}
