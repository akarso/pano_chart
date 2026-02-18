import 'dart:convert';

import 'package:http/http.dart' as http;

import 'dto/rankings_response_dto.dart';
import 'rankings_api.dart';

/// HTTP adapter implementing [RankingsApi] against the backend
/// `GET /api/rankings` endpoint.
class HttpRankingsApi implements RankingsApi {
  final http.Client client;
  final String baseUrl;

  HttpRankingsApi({
    required this.client,
    required this.baseUrl,
  });

  @override
  Future<RankingsResponseDto> fetchRankings({
    required String timeframe,
    required String sort,
    required int page,
    required int pageSize,
    String sidewaysAlgo = 'v2',
  }) async {
    final uri = Uri.parse(
      '$baseUrl/api/rankings?timeframe=$timeframe&sort=$sort&page=$page&pageSize=$pageSize&sidewaysAlgo=$sidewaysAlgo',
    );

    final response = await client.get(uri);

    if (response.statusCode != 200) {
      throw HttpRankingsApiException(
        statusCode: response.statusCode,
        message: 'Rankings API error: ${response.statusCode}',
      );
    }

    final jsonMap = jsonDecode(response.body) as Map<String, dynamic>;
    return RankingsResponseDto.fromJson(jsonMap);
  }
}

/// Exception thrown by [HttpRankingsApi] on non-200 responses.
class HttpRankingsApiException implements Exception {
  final int statusCode;
  final String message;

  const HttpRankingsApiException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'HttpRankingsApiException($statusCode): $message';
}
