import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/events/api/events_response_dto.dart';

void main() {
  group('EventsResponseDto', () {
    test('fromJson parses events list', () {
      final json = {
        'events': [
          {
            'id': 'e1',
            'country': 'US',
            'title': 'CPI',
            'impact': 'high',
            'timestamp': '2025-03-01T08:30:00Z',
          },
          {
            'id': 'e2',
            'country': 'EU',
            'title': 'PMI',
            'impact': 'medium',
            'timestamp': '2025-03-01T10:00:00Z',
          },
        ],
      };
      final dto = EventsResponseDto.fromJson(json);
      expect(dto.events.length, 2);
      expect(dto.events[0].id, 'e1');
      expect(dto.events[1].id, 'e2');
    });

    test('fromJson handles empty events', () {
      final dto = EventsResponseDto.fromJson({'events': []});
      expect(dto.events, isEmpty);
    });

    test('fromJson handles missing events key', () {
      final dto = EventsResponseDto.fromJson({});
      expect(dto.events, isEmpty);
    });
  });
}
