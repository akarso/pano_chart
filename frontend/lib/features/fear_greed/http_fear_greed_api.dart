import 'dart:convert';
import 'package:http/http.dart' as http;
import 'fear_greed_data.dart';

/// Fetches the Fear & Greed Index from the backend.
abstract class FearGreedApi {
  Future<FearGreedData> fetch();
}

class HttpFearGreedApi implements FearGreedApi {
  final http.Client client;
  final String baseUrl;

  HttpFearGreedApi({required this.client, required this.baseUrl});

  @override
  Future<FearGreedData> fetch() async {
    final uri = Uri.parse('$baseUrl/api/v1/fear-greed');
    final response = await client.get(uri);
    if (response.statusCode != 200) {
      throw HttpFearGreedApiException(
        'Fear & Greed API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return FearGreedData.fromJson(json);
  }
}

class HttpFearGreedApiException implements Exception {
  final String message;
  HttpFearGreedApiException(this.message);

  @override
  String toString() => message;
}
