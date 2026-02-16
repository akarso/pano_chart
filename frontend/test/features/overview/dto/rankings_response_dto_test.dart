import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/dto/rankings_response_dto.dart';

void main() {
  test('RankingsResponseDto_parsesValidJson', () {
    final json = {
      'timeframe': '1h',
      'sort': 'total',
      'page': 1,
      'pageSize': 30,
      'totalItems': 2,
      'totalPages': 1,
      'precision': 30,
      'results': [
        {
          'symbol': 'BTCUSDT',
          'totalScore': 2.75,
          'scores': {'trend': 1.0, 'sideways': 0.5, 'gain': 1.25},
          'volume': 5000000.0,
          'sparkline': [42000.0, 42100.0, 41900.0],
        },
        {
          'symbol': 'ETHUSDT',
          'totalScore': -1.5,
          'scores': {'trend': -0.5, 'sideways': 0.2, 'gain': -1.2},
          'volume': 2000000.0,
          'sparkline': [3200.0, 3180.0],
        },
      ],
    };

    final dto = RankingsResponseDto.fromJson(json);

    expect(dto.timeframe, '1h');
    expect(dto.sort, 'total');
    expect(dto.page, 1);
    expect(dto.pageSize, 30);
    expect(dto.totalItems, 2);
    expect(dto.totalPages, 1);
    expect(dto.precision, 30);
    expect(dto.results.length, 2);

    expect(dto.results[0].symbol, 'BTCUSDT');
    expect(dto.results[0].totalScore, 2.75);
    expect(dto.results[0].scores.trend, 1.0);
    expect(dto.results[0].scores.sideways, 0.5);
    expect(dto.results[0].scores.gain, 1.25);
    expect(dto.results[0].volume, 5000000.0);
    expect(dto.results[0].sparkline, [42000.0, 42100.0, 41900.0]);

    expect(dto.results[1].symbol, 'ETHUSDT');
    expect(dto.results[1].totalScore, -1.5);
    expect(dto.results[1].scores.trend, -0.5);
    expect(dto.results[1].sparkline, [3200.0, 3180.0]);
  });

  test('RankingItemDto_handlesMissingScoresAndSparkline', () {
    final json = {
      'symbol': 'SOLUSDT',
      'totalScore': 0,
      'volume': 100,
    };

    final dto = RankingItemDto.fromJson(json);

    expect(dto.symbol, 'SOLUSDT');
    expect(dto.totalScore, 0.0);
    expect(dto.scores.trend, 0.0);
    expect(dto.scores.sideways, 0.0);
    expect(dto.scores.gain, 0.0);
    expect(dto.volume, 100.0);
    expect(dto.sparkline, isEmpty);
  });

  test('RankingsResponseDto_handlesEmptyResults', () {
    final json = {
      'timeframe': '4h',
      'sort': 'gain',
      'page': 1,
      'pageSize': 30,
      'totalItems': 0,
      'totalPages': 0,
      'precision': 0,
      'results': [],
    };

    final dto = RankingsResponseDto.fromJson(json);

    expect(dto.results, isEmpty);
    expect(dto.totalItems, 0);
    expect(dto.totalPages, 0);
  });

  test('RankingsResponseDto_handlesMissingPrecision', () {
    final json = {
      'timeframe': '1h',
      'sort': 'total',
      'page': 1,
      'pageSize': 30,
      'totalItems': 0,
      'totalPages': 0,
      'results': [],
    };

    final dto = RankingsResponseDto.fromJson(json);
    expect(dto.precision, 0);
  });
}
