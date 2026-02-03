import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';

void main() {
  test('CandleDto_serializesFromJson', () {
    final json = {
      'timestamp': '2024-01-01T00:00:00Z',
      'open': 42000.0,
      'high': 42100.0,
      'low': 41950.0,
      'close': 42050.0,
      'volume': 123.45
    };

    final dto = CandleDto.fromJson(json);
    expect(dto.timestamp.toUtc().toIso8601String(), '2024-01-01T00:00:00.000Z');
    expect(dto.open, 42000.0);
    expect(dto.high, 42100.0);
    expect(dto.low, 41950.0);
    expect(dto.close, 42050.0);
    expect(dto.volume, 123.45);

    final out = dto.toJson();
    expect(out['timestamp'], '2024-01-01T00:00:00.000Z');
    expect(out['open'], 42000.0);
  });
}
