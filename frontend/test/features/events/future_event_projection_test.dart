import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/event.dart';
import 'package:pano_chart_frontend/features/candles/api/candle_response.dart';
import 'package:pano_chart_frontend/features/detail/chart_navigation.dart';
import 'package:pano_chart_frontend/features/events/chart_event_overlay_painter.dart';
import 'package:pano_chart_frontend/features/events/event_filter.dart';
import 'package:pano_chart_frontend/features/events/event_marker_builder.dart';
import 'package:pano_chart_frontend/features/detail/chart/crosshair_overlay.dart';

CandleDto _candle(DateTime ts) => CandleDto(
      timestamp: ts,
      open: 100,
      high: 110,
      low: 90,
      close: 105,
      volume: 1000,
    );

Event _event(String id, DateTime ts,
        {EventImpact impact = EventImpact.high}) =>
    Event(id: id, country: 'US', title: id, impact: impact, timestamp: ts);

void main() {
  // ─── maxProjectionWindow / maxProjectionSlots ───

  group('maxProjectionWindow', () {
    test('returns correct windows for all timeframes', () {
      expect(maxProjectionWindow('1m'), const Duration(hours: 4));
      expect(maxProjectionWindow('5m'), const Duration(hours: 4));
      expect(maxProjectionWindow('15m'), const Duration(hours: 6));
      expect(maxProjectionWindow('1h'), const Duration(hours: 24));
      expect(maxProjectionWindow('4h'), const Duration(hours: 48));
      expect(maxProjectionWindow('1d'), const Duration(days: 3));
    });

    test('defaults to 24h for unknown timeframe', () {
      expect(maxProjectionWindow('2h'), const Duration(hours: 24));
    });
  });

  group('maxProjectionSlots', () {
    test('1h → 24 slots', () {
      expect(maxProjectionSlots('1h'), 24);
    });

    test('1m → 240 slots (4h / 1m)', () {
      expect(maxProjectionSlots('1m'), 240);
    });

    test('5m → 48 slots (4h / 5m)', () {
      expect(maxProjectionSlots('5m'), 48);
    });

    test('15m → 24 slots (6h / 15m)', () {
      expect(maxProjectionSlots('15m'), 24);
    });

    test('4h → 12 slots (48h / 4h)', () {
      expect(maxProjectionSlots('4h'), 12);
    });

    test('1d → 3 slots (3d / 1d)', () {
      expect(maxProjectionSlots('1d'), 3);
    });
  });

  // ─── buildFutureEventMarkers ───

  group('buildFutureEventMarkers', () {
    test('returns empty when no events', () {
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: DateTime.utc(2025, 3, 1, 10),
        totalCandleCount: 10,
        events: [],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers, isEmpty);
    });

    test('excludes events before or at last candle', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 9)), // before
          _event('e2', lastTs), // at exact last candle
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers, isEmpty);
    });

    test('excludes events beyond max projection window', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          // 25 hours ahead, but maxSlots = 24 for 1h
          _event('e1', DateTime.utc(2025, 3, 2, 11)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers, isEmpty);
    });

    test('positions future event at correct slot index', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      // Event 3 hours ahead → slot 3, absolute index = 10 + 3 = 13
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 13)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers.length, 1);
      expect(markers[0].isFuture, isTrue);
      // absIndex 13, visStart 0: x = 13 * 14 + 7 = 189
      expect(markers[0].x, 189.0);
    });

    test('respects visibleStartIndex and scrollPixelOffset', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 13)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 5,
        scrollPixelOffset: 3.0,
      );
      expect(markers.length, 1);
      // absIndex 13, visStart 5: (13-5)*14 + 7 - 3 = 112 + 7 - 3 = 116
      expect(markers[0].x, 116.0);
    });

    test('marks all future markers with isFuture = true', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 11)),
          _event('e2', DateTime.utc(2025, 3, 1, 15)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers.length, 2);
      expect(markers.every((m) => m.isFuture), isTrue);
    });

    test('groups multiple events at same slot', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 11, 10)),
          _event('e2', DateTime.utc(2025, 3, 1, 11, 40)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers.length, 1);
      expect(markers[0].events.length, 2);
    });

    test('respects filter level', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 12),
              impact: EventImpact.low),
        ],
        filterLevel: EventFilterLevel.highOnly,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers, isEmpty);
    });

    test('projected timestamp reflects slot position', () {
      final lastTs = DateTime.utc(2025, 3, 1, 10);
      final markers = buildFutureEventMarkers(
        lastCandleTimestamp: lastTs,
        totalCandleCount: 10,
        events: [
          _event('e1', DateTime.utc(2025, 3, 1, 14, 30)),
        ],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        candleDuration: const Duration(hours: 1),
        maxSlots: 24,
        visibleStartIndex: 0,
        scrollPixelOffset: 0,
      );
      expect(markers.length, 1);
      // Event at 14:30, slot = floor(4.5h / 1h) = 4
      // Projected timestamp = 10:00 + 4h = 14:00
      expect(markers[0].timestamp, DateTime.utc(2025, 3, 1, 14));
    });
  });

  // ─── EventMarker.isFuture field ───

  group('EventMarker', () {
    test('isFuture defaults to false', () {
      final marker = EventMarker(
        x: 10,
        timestamp: DateTime.utc(2025, 3, 1),
        events: [],
      );
      expect(marker.isFuture, isFalse);
    });

    test('isFuture can be set to true', () {
      final marker = EventMarker(
        x: 10,
        timestamp: DateTime.utc(2025, 3, 1),
        events: [],
        isFuture: true,
      );
      expect(marker.isFuture, isTrue);
    });
  });

  // ─── buildEventMarkers scrollPixelOffset ───

  group('buildEventMarkers scrollPixelOffset', () {
    test('default offset is zero (backward compatible)', () {
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
        events: [_event('e1', DateTime.utc(2025, 3, 1, 1, 20))],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
      );
      expect(markers.length, 1);
      // candle index 1: x = 1*14 + 7 - 0 = 21
      expect(markers[0].x, 21.0);
    });

    test('scrollPixelOffset shifts marker x left', () {
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
        events: [_event('e1', DateTime.utc(2025, 3, 1, 1, 20))],
        filterLevel: EventFilterLevel.all,
        candleWidth: 14,
        scrollPixelOffset: 5.0,
      );
      expect(markers.length, 1);
      // candle index 1: x = 1*14 + 7 - 5 = 16
      expect(markers[0].x, 16.0);
    });
  });

  // ─── CrosshairState future zone ───

  group('CrosshairState future zone', () {
    final candle = CandleDto(
      timestamp: DateTime.utc(2025, 3, 1, 10),
      open: 100,
      high: 110,
      low: 90,
      close: 105,
      volume: 5000,
    );

    test('isFutureZone defaults to false', () {
      final state = CrosshairState(
        candleIndex: 5,
        x: 75.0,
        touchY: 100.0,
        candle: candle,
      );
      expect(state.isFutureZone, isFalse);
      expect(state.futureTimestamp, isNull);
    });

    test('can represent future zone with projected timestamp', () {
      final futureTs = DateTime.utc(2025, 3, 1, 15);
      final state = CrosshairState(
        candleIndex: 15,
        x: 200.0,
        touchY: 100.0,
        candle: candle,
        isFutureZone: true,
        futureTimestamp: futureTs,
      );
      expect(state.isFutureZone, isTrue);
      expect(state.futureTimestamp, futureTs);
      // Candle is the last real candle (placeholder)
      expect(state.candle, candle);
    });
  });

  // ─── CrosshairOverlay future zone rendering ───

  group('CrosshairOverlay future zone', () {
    Widget _wrap(Widget child) =>
        MaterialApp(home: Scaffold(body: child));

    testWidgets('hides OHLC and shows "Future zone" in future mode',
        (tester) async {
      final candle = CandleDto(
        timestamp: DateTime.utc(2025, 3, 1, 10),
        open: 63240,
        high: 63820,
        low: 62980,
        close: 63510,
        volume: 18200,
      );
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 400,
          height: 400,
          child: CrosshairOverlay(
            state: CrosshairState(
              candleIndex: 20,
              x: 200.0,
              touchY: 100.0,
              candle: candle,
              isFutureZone: true,
              futureTimestamp: DateTime.utc(2025, 3, 1, 15, 0),
            ),
            symbol: 'BTCUSDT',
            timeframe: '1h',
            priceHeight: 250,
            volumeHeight: 50,
            oscillatorHeight: 80,
            chartWidth: 360,
            priceLo: 62000,
            priceHi: 64000,
          ),
        ),
      ));

      // Should show "Future zone" and "No price data"
      expect(find.text('Future zone'), findsOneWidget);
      expect(find.text('No price data'), findsOneWidget);
      // Should NOT show OHLC
      expect(find.textContaining('O:'), findsNothing);
      expect(find.textContaining('H:'), findsNothing);
    });

    testWidgets('shows normal OHLC when not in future zone',
        (tester) async {
      final candle = CandleDto(
        timestamp: DateTime.utc(2025, 3, 1, 10),
        open: 63240,
        high: 63820,
        low: 62980,
        close: 63510,
        volume: 18200,
      );
      await tester.pumpWidget(_wrap(
        SizedBox(
          width: 400,
          height: 400,
          child: CrosshairOverlay(
            state: CrosshairState(
              candleIndex: 5,
              x: 100.0,
              touchY: 80.0,
              candle: candle,
            ),
            symbol: 'BTCUSDT',
            timeframe: '1h',
            priceHeight: 250,
            volumeHeight: 50,
            oscillatorHeight: 80,
            chartWidth: 360,
            priceLo: 62000,
            priceHi: 64000,
          ),
        ),
      ));

      // OHLC present
      expect(find.textContaining('O:'), findsOneWidget);
      // No future zone label
      expect(find.text('Future zone'), findsNothing);
    });
  });

  // ─── EventOverlayPainter future rendering ───

  group('EventOverlayPainter', () {
    test('shouldRepaint when markers change', () {
      final markers1 = [
        EventMarker(
          x: 50,
          timestamp: DateTime.utc(2025, 3, 1),
          events: [_event('e1', DateTime.utc(2025, 3, 1))],
        ),
      ];
      final markers2 = [
        EventMarker(
          x: 50,
          timestamp: DateTime.utc(2025, 3, 1),
          events: [_event('e1', DateTime.utc(2025, 3, 1))],
          isFuture: true,
        ),
      ];

      final p1 = EventOverlayPainter(markers: markers1);
      final p2 = EventOverlayPainter(markers: markers2);
      // Different list instances → should repaint
      expect(p2.shouldRepaint(p1), isTrue);
    });

    test('shouldRepaint returns false for identical markers', () {
      final markers = [
        EventMarker(
          x: 50,
          timestamp: DateTime.utc(2025, 3, 1),
          events: [_event('e1', DateTime.utc(2025, 3, 1))],
        ),
      ];
      final p1 = EventOverlayPainter(markers: markers);
      final p2 = EventOverlayPainter(markers: markers);
      expect(p2.shouldRepaint(p1), isFalse);
    });
  });
}
