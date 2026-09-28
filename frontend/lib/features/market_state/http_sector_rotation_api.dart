import 'dart:convert';
import 'package:http/http.dart' as http;
import 'sector_rotation_data.dart';

/// Default fetch limit — matches [compositeChartLimit] so a sector's own
/// series covers the same context length as the composite chart it overlays.
const sectorRotationLimit = 200;

/// Fetches sector rotation (relative strength) data from the backend.
///
/// The route is absent (HTTP 404) when the sector catalog isn't configured
/// server-side (COMMON.md "Sector composites (PR-098)"); callers must treat
/// a null result as feature-disabled, not as an error.
abstract class SectorRotationApi {
  Future<SectorRotationData?> fetch({String timeframe, int limit});
}

class HttpSectorRotationApi implements SectorRotationApi {
  final http.Client client;
  final String baseUrl;

  HttpSectorRotationApi({required this.client, required this.baseUrl});

  @override
  Future<SectorRotationData?> fetch({
    String timeframe = '4h',
    int limit = sectorRotationLimit,
  }) async {
    final uri = Uri.parse(
      '$baseUrl/api/market/sectors?timeframe=$timeframe&limit=$limit',
    );
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode == 404) return null;
    if (response.statusCode != 200) {
      throw HttpSectorRotationApiException(
        'Sector rotation API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return SectorRotationData.fromJson(json);
  }
}

class HttpSectorRotationApiException implements Exception {
  final String message;
  HttpSectorRotationApiException(this.message);

  @override
  String toString() => message;
}
