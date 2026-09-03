/// DTO for a single item in the overview response.
class OverviewItemDto {
  final String symbol;
  final double totalScore;
  final List<double> sparkline;

  const OverviewItemDto({
    required this.symbol,
    required this.totalScore,
    required this.sparkline,
  });

  factory OverviewItemDto.fromJson(Map<String, dynamic> json) {
    return OverviewItemDto(
      symbol: json['symbol'] as String,
      totalScore: (json['totalScore'] as num).toDouble(),
      sparkline: (json['sparkline'] as List)
          .map((e) => (e as num).toDouble())
          .toList(),
    );
  }
}

/// DTO for the /api/overview response.
class OverviewResponseDto {
  final String timeframe;
  final int count;
  final int precision;
  final List<OverviewItemDto> results;

  const OverviewResponseDto({
    required this.timeframe,
    required this.count,
    required this.precision,
    required this.results,
  });

  factory OverviewResponseDto.fromJson(Map<String, dynamic> json) {
    return OverviewResponseDto(
      timeframe: json['timeframe'] as String,
      count: json['count'] as int,
      precision: json['precision'] as int,
      results: (json['results'] as List)
          .map((e) => OverviewItemDto.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}
