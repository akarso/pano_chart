import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/api/events_api.dart';
import 'package:pano_chart_frontend/features/events/api/events_response_dto.dart';
import 'package:pano_chart_frontend/features/events/application/get_events.dart';

class _FakeEventsApi implements EventsApi {
  List<Event> result = [];
  int callCount = 0;
  String? lastDateFrom;
  String? lastDateTo;
  String? lastImpact;
  String? lastCountry;

  @override
  Future<EventsResponseDto> fetchEvents({
    required String dateFrom,
    required String dateTo,
    String? impact,
    String? country,
  }) async {
    callCount++;
    lastDateFrom = dateFrom;
    lastDateTo = dateTo;
    lastImpact = impact;
    lastCountry = country;
    return EventsResponseDto(events: result);
  }
}

void main() {
  group('GetEventsImpl', () {
    test('delegates to api and returns events', () async {
      final fakeApi = _FakeEventsApi()
        ..result = [
          Event(
            id: 'e1',
            country: 'US',
            title: 'CPI',
            impact: EventImpact.high,
            timestamp: DateTime.utc(2025, 3, 3),
          ),
        ];
      final useCase = GetEventsImpl(fakeApi);
      final result = await useCase.execute(
        const GetEventsInput(dateFrom: '2025-03-01', dateTo: '2025-03-07'),
      );
      expect(result.length, 1);
      expect(result[0].title, 'CPI');
      expect(fakeApi.callCount, 1);
      expect(fakeApi.lastDateFrom, '2025-03-01');
      expect(fakeApi.lastDateTo, '2025-03-07');
    });

    test('passes impact and country to api', () async {
      final fakeApi = _FakeEventsApi();
      final useCase = GetEventsImpl(fakeApi);
      await useCase.execute(
        const GetEventsInput(
          dateFrom: '2025-03-01',
          dateTo: '2025-03-07',
          impact: 'high',
          country: 'US',
        ),
      );
      expect(fakeApi.lastImpact, 'high');
      expect(fakeApi.lastCountry, 'US');
    });

    test('returns empty list when api returns none', () async {
      final fakeApi = _FakeEventsApi();
      final useCase = GetEventsImpl(fakeApi);
      final result = await useCase.execute(
        const GetEventsInput(dateFrom: '2025-03-01', dateTo: '2025-03-07'),
      );
      expect(result, isEmpty);
    });
  });
}
