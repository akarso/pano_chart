import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/fear_greed/fear_greed_data.dart';

void main() {
  group('FearGreedData', () {
    test('fromJson parses correctly', () {
      final json = {
        'value': 14,
        'valueClassification': 'Extreme Fear',
        'timestampUtc': '2026-03-01T00:00:00Z',
      };
      final data = FearGreedData.fromJson(json);

      expect(data.value, 14);
      expect(data.classification, 'Extreme Fear');
      expect(data.timestampUtc, DateTime.utc(2026, 3, 1));
    });

    test('fromJson handles high values', () {
      final json = {
        'value': 92,
        'valueClassification': 'Extreme Greed',
        'timestampUtc': '2026-06-15T12:30:00Z',
      };
      final data = FearGreedData.fromJson(json);

      expect(data.value, 92);
      expect(data.classification, 'Extreme Greed');
      expect(data.timestampUtc.hour, 12);
      expect(data.timestampUtc.minute, 30);
    });
  });
}
