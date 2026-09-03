import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/dto/overview_response_dto.dart';

void main() {
  test('OverviewResponseDto_parsesValidJson', () {
    final json = {
      'timeframe': '1h',
      'count': 2,
      'precision': 30,
      'results': [
        {
          'symbol': 'BTCUSDT',
          'totalScore': 2.75,
          'sparkline': [42000.0, 42100.0, 41900.0],
        },
        {
          'symbol': 'ETHUSDT',
          'totalScore': -1.5,
          'sparkline': [3200.0, 3180.0],
        },
      ],
    };

    final dto = OverviewResponseDto.fromJson(json);

    expect(dto.timeframe, '1h');
    expect(dto.count, 2);
    expect(dto.precision, 30);
    expect(dto.results.length, 2);

    expect(dto.results[0].symbol, 'BTCUSDT');
    expect(dto.results[0].totalScore, 2.75);
    expect(dto.results[0].sparkline, [42000.0, 42100.0, 41900.0]);

    expect(dto.results[1].symbol, 'ETHUSDT');
    expect(dto.results[1].totalScore, -1.5);
    expect(dto.results[1].sparkline, [3200.0, 3180.0]);
  });

  test('OverviewItemDto_parsesIntegerSparklineValues', () {
    final json = {
      'symbol': 'SOLUSDT',
      'totalScore': 0,
      'sparkline': [100, 105, 110],
    };

    final dto = OverviewItemDto.fromJson(json);

    expect(dto.symbol, 'SOLUSDT');
    expect(dto.totalScore, 0.0);
    expect(dto.sparkline, [100.0, 105.0, 110.0]);
  });

  test('OverviewResponseDto_handlesEmptyResults', () {
    final json = {
      'timeframe': '4h',
      'count': 0,
      'precision': 30,
      'results': [],
    };

    final dto = OverviewResponseDto.fromJson(json);

    expect(dto.results, isEmpty);
    expect(dto.count, 0);
  });
}
