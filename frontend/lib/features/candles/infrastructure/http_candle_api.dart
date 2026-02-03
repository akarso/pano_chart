import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/candle_api.dart';
import '../api/candle_request.dart';
import '../api/candle_response.dart';

class HttpCandleApiException implements Exception {
  final int? statusCode;
  final String message;

  HttpCandleApiException(this.message, {this.statusCode});

  @override
  String toString() =>
      'HttpCandleApiException(statusCode: $statusCode, message: $message)';
}

/// HTTP adapter implementing [CandleApi].
class HttpCandleApi implements CandleApi {
  final String baseUrl;
  final http.Client _client;

  HttpCandleApi({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  @override
  Future<CandleSeriesResponse> fetchCandles(CandleRequest request) async {
    final uri =
        Uri.parse(baseUrl).replace(path: '/api/v1/candles', queryParameters: {
      'symbol': request.symbol,
      'timeframe': request.timeframe,
      'from': request.from.toUtc().toIso8601String(),
      'to': request.to.toUtc().toIso8601String(),
    });

    final res = await _client.get(uri);

    if (res.statusCode == 200) {
      final body = jsonDecode(res.body) as Map<String, dynamic>;
      return CandleSeriesResponse.fromJson(body);
    }

    if (res.statusCode == 400) {
      throw HttpCandleApiException('Bad request', statusCode: 400);
    }

    if (res.statusCode == 500) {
      throw HttpCandleApiException('Server error', statusCode: 500);
    }

    throw HttpCandleApiException('Unexpected status: ${res.statusCode}',
        statusCode: res.statusCode);
  }
}
