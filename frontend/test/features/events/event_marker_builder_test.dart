import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';
import 'package:pano_chart_frontend/features/events/event_marker_builder.dart';

CandleDto _candle(DateTime ts) => CandleDto(
      timestamp: ts,
      open: 100,
      high: 110,
      low: 90,
      close: 105,
      volume: 1000,
    );

Event _event(String id, DateTime ts, {EventImpact impact = EventImpact.high}) =>
    Event(id: id, country: 'US', title: id, impact: impact, timestamp: ts);

void main() {
  group('buildEventMarkers', () {
    test('empty events returns empty markers', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [_candle(DateTime.utc(2025, 3, 1))],
      );
      final markers = buildEventMarkers(
        series: series,
        events: [],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
      );
      expect(markers, isEmpty);
    });

    test('event outside range is excluded', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          _candle(DateTime.utc(2025, 3, 1, 0)),
          _candle(DateTime.utc(2025, 3, 1, 1)),
          _candle(DateTime.utc(2025, 3, 1, 2)),
        ],
      );
      final markers = buildEventMarkers(
        series: series,
        events: [_event('e1', DateTime.utc(2025, 3, 2))],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
      );
      expect(markers, isEmpty);
    });

    test('event aligned to nearest candle', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          _candle(DateTime.utc(2025, 3, 1, 0)),
          _candle(DateTime.utc(2025, 3, 1, 1)),
          _candle(DateTime.utc(2025, 3, 1, 2)),
        ],
      );
      // Event at 1:20 should snap to candle index 1 (timestamp 1:00)
      final markers = buildEventMarkers(
        series: series,
        events: [_event('e1', DateTime.utc(2025, 3, 1, 1, 20))],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
      );
      expect(markers.length, 1);
      // candle index 1 -> x = 1 * 14 + 7 = 21
      expect(markers[0].x, 21.0);
    });

    test('multiple events at same candle merge', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          _candle(DateTime.utc(2025, 3, 1, 0)),
          _candle(DateTime.utc(2025, 3, 1, 1)),
        ],
      );
      final markers = buildEventMarkers(
        series: series,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 0, 15)),
          _event('e2', DateTime.utc(2025, 3, 1, 0, 30)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
      );
      expect(markers.length, 1);
      expect(markers[0].events.length, 2);
    });

    test('filter level excludes low impact', () {
      final series = CandleSeriesResponse(
        symbol: 'BTC',
        timeframe: '1h',
        candles: [
          _candle(DateTime.utc(2025, 3, 1, 0)),
          _candle(DateTime.utc(2025, 3, 1, 1)),
        ],
      );
      final markers = buildEventMarkers(
        series: series,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 0, 30), impact: EventImpact.low),
        ],
        filterLevel: EventFilterLevel.highAndMedium,
        candleWidth: 14,
      );
      expect(markers, isEmpty);
    });
  });
}
