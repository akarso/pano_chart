import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/events_api.dart';
import '../api/events_response_dto.dart';

/// Exception thrown by [HttpEventsApi] on non-200 responses.
class HttpEventsApiException implements Exception {
  final int statusCode;
  final String message;

  const HttpEventsApiException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'HttpEventsApiException($statusCode): $message';
}

/// HTTP adapter implementing [EventsApi] against the backend
/// `GET /api/v1/events` endpoint.
class HttpEventsApi implements EventsApi {
  final http.Client client;
  final String baseUrl;

  HttpEventsApi({required this.client, required this.baseUrl});

  @override
  Future<EventsResponseDto> fetchEvents({
    required String dateFrom,
    required String dateTo,
    String? impact,
    String? country,
  }) async {
    final queryParams = <String, String>{
      'date_from': dateFrom,
      'date_to': dateTo,
    };
    if (impact != null && impact.isNotEmpty) {
      queryParams['impact'] = impact;
    }
    if (country != null && country.isNotEmpty) {
      queryParams['country'] = country;
    }

    final uri = Uri.parse(baseUrl).replace(
      path: '/api/v1/events',
      queryParameters: queryParams,
    );

    final response = await client.get(uri).timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpEventsApiException(
        statusCode: response.statusCode,
        message: 'Events API error: ${response.statusCode}',
      );
    }

    final jsonMap = jsonDecode(response.body) as Map<String, dynamic>;
    return EventsResponseDto.fromJson(jsonMap);
  }
}
