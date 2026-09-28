import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/market_state/http_sector_rotation_api.dart';
import 'package:pano_chart_frontend/features/market_state/sector_rotation_data.dart';
import 'package:pano_chart_frontend/features/market_state/sector_rotation_presentation.dart';

void main() {
  // ---- Data Model Tests ----

  group('SectorRotationData', () {
    test('fromJson parses all fields', () {
      final json = {
        'timeframe': '4h',
        'marketSymbolCount': 143,
        'sectors': [
          {
            'id': 'l1',
            'name': 'Layer 1',
            'symbolCount': 8,
            'points': [
              {'t': 1000, 'v': 100.0},
              {'t': 2000, 'v': 102.1},
            ],
            'return': 0.021,
            'rs': 0.008,
            'rsAvailable': true,
          },
          {
            'id': 'defi',
            'name': 'DeFi',
            'symbolCount': 1,
            'points': <dynamic>[],
            'return': 0.0,
            'rsAvailable': false,
          },
        ],
      };

      final data = SectorRotationData.fromJson(json);
      expect(data.timeframe, '4h');
      expect(data.marketSymbolCount, 143);
      expect(data.sectors.length, 2);

      final l1 = data.sectors[0];
      expect(l1.id, 'l1');
      expect(l1.name, 'Layer 1');
      expect(l1.symbolCount, 8);
      expect(l1.points.length, 2);
      expect(l1.points[1].value, 102.1);
      expect(l1.returnValue, 0.021);
      expect(l1.rs, 0.008);
      expect(l1.rsAvailable, isTrue);

      final defi = data.sectors[1];
      expect(defi.rs, isNull);
      expect(defi.rsAvailable, isFalse);
      expect(defi.points, isEmpty);
    });

    test('fromJson handles missing sectors as empty list', () {
      final data = SectorRotationData.fromJson({'timeframe': '1h'});
      expect(data.sectors, isEmpty);
      expect(data.marketSymbolCount, 0);
    });
  });

  // ---- API Tests ----

  group('HttpSectorRotationApi', () {
    test('sends GET request with timeframe and limit', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'marketSymbolCount': 50,
            'sectors': <dynamic>[],
          }),
          200,
        );
      });

      final api = HttpSectorRotationApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final data = await api.fetch(timeframe: '4h', limit: 100);
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/sectors?timeframe=4h&limit=100',
      );
      expect(data!.marketSymbolCount, 50);
    });

    test('uses default params', () async {
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedUri = req.url;
        return http.Response(
          jsonEncode({
            'timeframe': '4h',
            'marketSymbolCount': 10,
            'sectors': <dynamic>[],
          }),
          200,
        );
      });

      final api = HttpSectorRotationApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      await api.fetch();
      expect(
        capturedUri.toString(),
        'http://localhost:8080/api/market/sectors?timeframe=4h&limit=200',
      );
    });

    test('404 (catalog not configured) returns null, not an error', () async {
      final client = MockClient((_) async => http.Response('', 404));
      final api = HttpSectorRotationApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final data = await api.fetch();
      expect(data, isNull);
    });

    test('throws on other non-200 responses', () async {
      final client = MockClient((_) async => http.Response('error', 500));
      final api = HttpSectorRotationApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch(),
        throwsA(isA<HttpSectorRotationApiException>()),
      );
    });
  });

  // ---- Presentation Tests ----

  group('buildSectorRotationRows', () {
    test('normalizes bar fraction against the widest RS bar', () {
      const data = SectorRotationData(
        timeframe: '4h',
        marketSymbolCount: 100,
        sectors: [
          SectorIndexData(
            id: 'strong',
            name: 'Strong',
            symbolCount: 5,
            points: [],
            returnValue: 0.04,
            rs: 0.04,
            rsAvailable: true,
          ),
          SectorIndexData(
            id: 'weak',
            name: 'Weak',
            symbolCount: 3,
            points: [],
            returnValue: -0.02,
            rs: -0.02,
            rsAvailable: true,
          ),
          SectorIndexData(
            id: 'unavailable',
            name: 'Unavailable',
            symbolCount: 1,
            points: [],
            returnValue: 0.0,
            rsAvailable: false,
          ),
        ],
      );

      final rows = buildSectorRotationRows(data);
      expect(rows.length, 3);
      expect(rows[0].barFraction, 1.0);
      expect(rows[1].barFraction, 0.5);
      expect(rows[2].barFraction, 0.0);
      // Backend order is preserved (RS-available first, desc; then unavailable).
      expect(rows.map((r) => r.id), ['strong', 'weak', 'unavailable']);
    });

    test('all-unavailable sectors get zero bar fraction, no divide-by-zero', () {
      const data = SectorRotationData(
        timeframe: '4h',
        marketSymbolCount: 10,
        sectors: [
          SectorIndexData(
            id: 'a',
            name: 'A',
            symbolCount: 1,
            points: [],
            returnValue: 0.0,
            rsAvailable: false,
          ),
        ],
      );

      final rows = buildSectorRotationRows(data);
      expect(rows[0].barFraction, 0.0);
    });
  });

  group('sectorRsLabel', () {
    test('formats positive, negative, and zero RS', () {
      const positive = SectorRotationRow(
        id: 'a',
        name: 'A',
        symbolCount: 1,
        rs: 0.032,
        rsAvailable: true,
        barFraction: 1.0,
      );
      const negative = SectorRotationRow(
        id: 'b',
        name: 'B',
        symbolCount: 1,
        rs: -0.011,
        rsAvailable: true,
        barFraction: 0.5,
      );
      const zero = SectorRotationRow(
        id: 'c',
        name: 'C',
        symbolCount: 1,
        rs: 0.0,
        rsAvailable: true,
        barFraction: 0.0,
      );
      expect(sectorRsLabel(positive), '+3.2%');
      expect(sectorRsLabel(negative), '-1.1%');
      expect(sectorRsLabel(zero), '0.0%');
    });

    test('shows an em dash when unavailable', () {
      const row = SectorRotationRow(
        id: 'a',
        name: 'A',
        symbolCount: 1,
        rs: null,
        rsAvailable: false,
        barFraction: 0.0,
      );
      expect(sectorRsLabel(row), '—');
    });
  });
}
