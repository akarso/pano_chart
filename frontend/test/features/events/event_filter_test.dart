import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';

void main() {
  group('EventFilterLevel', () {
    test('highOnly allows only high', () {
      const f = EventFilterLevel.highOnly;
      expect(f.allows(EventImpact.high), isTrue);
      expect(f.allows(EventImpact.medium), isFalse);
      expect(f.allows(EventImpact.low), isFalse);
    });

    test('highAndMedium allows high and medium', () {
      const f = EventFilterLevel.highAndMedium;
      expect(f.allows(EventImpact.high), isTrue);
      expect(f.allows(EventImpact.medium), isTrue);
      expect(f.allows(EventImpact.low), isFalse);
    });

    test('all allows everything', () {
      const f = EventFilterLevel.all;
      expect(f.allows(EventImpact.high), isTrue);
      expect(f.allows(EventImpact.medium), isTrue);
      expect(f.allows(EventImpact.low), isTrue);
    });

    test('fromString parses correctly', () {
      expect(EventFilterLevel.fromString('highOnly'), EventFilterLevel.highOnly);
      expect(EventFilterLevel.fromString('highAndMedium'), EventFilterLevel.highAndMedium);
      expect(EventFilterLevel.fromString('all'), EventFilterLevel.all);
      expect(EventFilterLevel.fromString('unknown'), EventFilterLevel.highAndMedium);
    });

    test('label returns human-readable text', () {
      expect(EventFilterLevel.highOnly.label, 'High Only');
      expect(EventFilterLevel.highAndMedium.label, 'High + Medium');
      expect(EventFilterLevel.all.label, 'All');
    });
  });
}
