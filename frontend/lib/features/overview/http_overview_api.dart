import 'dart:convert';

import 'package:http/http.dart' as http;

import 'dto/overview_response_dto.dart';
import 'overview_api.dart';

/// HTTP adapter implementing [OverviewApi] against the backend
/// `GET /api/overview` endpoint.
class HttpOverviewApi implements OverviewApi {
  final http.Client client;
  final String baseUrl;

  HttpOverviewApi({
    required this.client,
    required this.baseUrl,
  });

  @override
  Future<OverviewResponseDto> fetchOverview({
    required String timeframe,
    required int limit,
  }) async {
    final uri = Uri.parse(
      '$baseUrl/api/overview?timeframe=$timeframe&limit=$limit',
    );

    final response = await client.get(uri);

    if (response.statusCode != 200) {
      throw HttpOverviewApiException(
        statusCode: response.statusCode,
        message: 'Overview API error: ${response.statusCode}',
      );
    }

    final jsonMap = jsonDecode(response.body) as Map<String, dynamic>;
    return OverviewResponseDto.fromJson(jsonMap);
  }
}

/// Exception thrown by [HttpOverviewApi] on non-200 responses.
class HttpOverviewApiException implements Exception {
  final int statusCode;
  final String message;

  const HttpOverviewApiException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'HttpOverviewApiException($statusCode): $message';
}
