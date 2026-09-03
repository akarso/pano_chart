import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';

void main() {
  group('EventImpact', () {
    test('fromString maps known values', () {
      expect(EventImpact.fromString('high'), EventImpact.high);
      expect(EventImpact.fromString('HIGH'), EventImpact.high);
      expect(EventImpact.fromString('major'), EventImpact.high);
      expect(EventImpact.fromString('Major'), EventImpact.high);
      expect(EventImpact.fromString('medium'), EventImpact.medium);
      expect(EventImpact.fromString('Medium'), EventImpact.medium);
      expect(EventImpact.fromString('low'), EventImpact.low);
      expect(EventImpact.fromString('LOW'), EventImpact.low);
    });

    test('fromString defaults unknown to medium', () {
      expect(EventImpact.fromString('unknown'), EventImpact.medium);
      expect(EventImpact.fromString(''), EventImpact.medium);
    });
  });

  group('Event', () {
    test('fromJson parses correctly', () {
      final json = {
        'id': 'abc123',
        'country': 'US',
        'title': 'FOMC Meeting',
        'impact': 'high',
        'timestamp': '2025-03-03T14:45:00Z',
      };
      final event = Event.fromJson(json);
      expect(event.id, 'abc123');
      expect(event.country, 'US');
      expect(event.title, 'FOMC Meeting');
      expect(event.impact, EventImpact.high);
      expect(event.timestamp.isUtc, isTrue);
      expect(event.timestamp, DateTime.utc(2025, 3, 3, 14, 45));
    });

    test('toJson roundtrips', () {
      final event = Event(
        id: 'x1',
        country: 'EU',
        title: 'ECB Rate',
        impact: EventImpact.medium,
        timestamp: DateTime.utc(2025, 6, 1, 10, 0),
      );
      final json = event.toJson();
      final restored = Event.fromJson(json);
      expect(restored, event);
    });

    test('equality by id', () {
      final a = Event(
        id: 'same',
        country: 'US',
        title: 'Title A',
        impact: EventImpact.high,
        timestamp: DateTime.utc(2025, 1, 1),
      );
      final b = Event(
        id: 'same',
        country: 'EU',
        title: 'Title B',
        impact: EventImpact.low,
        timestamp: DateTime.utc(2025, 2, 2),
      );
      expect(a, b);
      expect(a.hashCode, b.hashCode);
    });

    test('different ids are not equal', () {
      final a = Event(
        id: 'a',
        country: 'US',
        title: 'T',
        impact: EventImpact.high,
        timestamp: DateTime.utc(2025, 1, 1),
      );
      final b = Event(
        id: 'b',
        country: 'US',
        title: 'T',
        impact: EventImpact.high,
        timestamp: DateTime.utc(2025, 1, 1),
      );
      expect(a, isNot(b));
    });
  });
}
