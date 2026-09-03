import 'dart:convert';
import 'package:http/http.dart' as http;
import 'setup_data.dart';

/// Fetches setup quality scores for a symbol.
abstract class SetupApi {
  Future<SetupData> fetch({required String symbol, String timeframe});
}

class HttpSetupApi implements SetupApi {
  final http.Client client;
  final String baseUrl;

  HttpSetupApi({required this.client, required this.baseUrl});

  @override
  Future<SetupData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) async {
    final uri = Uri.parse(
        '$baseUrl/api/token/$symbol/setup?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpSetupApiException(
        'Setup API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return SetupData.fromJson(json);
  }
}

class HttpSetupApiException implements Exception {
  final String message;
  HttpSetupApiException(this.message);

  @override
  String toString() => message;
}
