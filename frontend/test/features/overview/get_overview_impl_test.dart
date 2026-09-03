import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/overview/dto/overview_response_dto.dart';
import 'package:pano_chart_frontend/features/overview/get_overview_impl.dart';
import 'package:pano_chart_frontend/features/overview/overview_api.dart';

class _FakeOverviewApi implements OverviewApi {
  OverviewResponseDto? response;
  Exception? error;
  String? capturedTimeframe;
  int? capturedLimit;

  @override
  Future<OverviewResponseDto> fetchOverview({
    required String timeframe,
    required int limit,
  }) async {
    capturedTimeframe = timeframe;
    capturedLimit = limit;
    if (error != null) throw error!;
    return response!;
  }
}

void main() {
  test('GetOverviewImpl_delegatesToOverviewApi', () async {
    final fakeApi = _FakeOverviewApi();
    fakeApi.response = const OverviewResponseDto(
      timeframe: '1h',
      count: 1,
      precision: 30,
      results: [
        OverviewItemDto(
          symbol: 'BTCUSDT',
          totalScore: 2.75,
          sparkline: [42000.0, 42100.0],
        ),
      ],
    );

    final usecase = GetOverviewImpl(fakeApi);
    final result = await usecase.call(
      timeframe: '1h',
      page: 1,
      sort: 'total',
    );

    expect(fakeApi.capturedTimeframe, '1h');
    expect(fakeApi.capturedLimit, 30);
    expect(result.items.length, 1);
    expect(result.items[0].symbol, 'BTCUSDT');
    expect(result.items[0].totalScore, 2.75);
    expect(result.items[0].sparkline, [42000.0, 42100.0]);
    expect(result.hasMore, false);
    expect(result.snapshot, isNull);
  });

  test('GetOverviewImpl_mapsMultipleItems', () async {
    final fakeApi = _FakeOverviewApi();
    fakeApi.response = const OverviewResponseDto(
      timeframe: '4h',
      count: 2,
      precision: 30,
      results: [
        OverviewItemDto(
          symbol: 'BTCUSDT',
          totalScore: 2.75,
          sparkline: [42000.0],
        ),
        OverviewItemDto(
          symbol: 'ETHUSDT',
          totalScore: -1.5,
          sparkline: [3200.0, 3180.0],
        ),
      ],
    );

    final usecase = GetOverviewImpl(fakeApi);
    final result = await usecase.call(
      timeframe: '4h',
      page: 1,
      sort: 'gain',
    );

    expect(result.items.length, 2);
    expect(result.items[0].symbol, 'BTCUSDT');
    expect(result.items[1].symbol, 'ETHUSDT');
    expect(result.items[1].totalScore, -1.5);
  });

  test('GetOverviewImpl_propagatesApiError', () async {
    final fakeApi = _FakeOverviewApi();
    fakeApi.error = Exception('network failure');

    final usecase = GetOverviewImpl(fakeApi);

    expect(
      () => usecase.call(timeframe: '1h', page: 1, sort: 'total'),
      throwsA(isA<Exception>()),
    );
  });
}
