import 'dart:convert';
import 'package:http/http.dart' as http;
import 'volatility_model.dart';

/// Fetches intraday volatility profiles from the backend.
abstract class VolatilityApi {
  Future<List<VolatilityBucket>> fetch({String timeframe});
}

class HttpVolatilityApi implements VolatilityApi {
  final http.Client client;
  final String baseUrl;

  HttpVolatilityApi({required this.client, required this.baseUrl});

  @override
  Future<List<VolatilityBucket>> fetch({String timeframe = '1m'}) async {
    final uri =
        Uri.parse('$baseUrl/api/volatility?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpVolatilityApiException(
        'Volatility API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    final buckets = json['buckets'] as List<dynamic>;
    return buckets
        .map((b) => VolatilityBucket.fromJson(b as Map<String, dynamic>))
        .toList();
  }
}

class HttpVolatilityApiException implements Exception {
  final String message;
  HttpVolatilityApiException(this.message);

  @override
  String toString() => message;
}
