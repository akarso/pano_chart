import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/volatility/volatility_model.dart';

void main() {
  group('VolatilityBucket', () {
    test('constructor stores values', () {
      const b = VolatilityBucket(minute: 120, normalized: 1.5, spikeProb: 0.3);
      expect(b.minute, 120);
      expect(b.normalized, 1.5);
      expect(b.spikeProb, 0.3);
    });

    test('fromJson parses correctly', () {
      final json = {'minute': 540, 'normalized': 0.8, 'spike_prob': 0.05};
      final b = VolatilityBucket.fromJson(json);
      expect(b.minute, 540);
      expect(b.normalized, 0.8);
      expect(b.spikeProb, 0.05);
    });

    test('fromJson handles integer values', () {
      final json = {'minute': 0, 'normalized': 1, 'spike_prob': 0};
      final b = VolatilityBucket.fromJson(json);
      expect(b.minute, 0);
      expect(b.normalized, 1.0);
      expect(b.spikeProb, 0.0);
    });
  });
}
