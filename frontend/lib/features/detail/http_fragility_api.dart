import 'dart:convert';
import 'package:http/http.dart' as http;
import 'fragility_data.dart';

/// Fetches fragility / position crowding scores for a symbol.
abstract class FragilityApi {
  Future<FragilityData> fetch({required String symbol, String timeframe});
}

class HttpFragilityApi implements FragilityApi {
  final http.Client client;
  final String baseUrl;

  HttpFragilityApi({required this.client, required this.baseUrl});

  @override
  Future<FragilityData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) async {
    final uri = Uri.parse(
        '$baseUrl/api/token/$symbol/fragility?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpFragilityApiException(
        'Fragility API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return FragilityData.fromJson(json);
  }
}

class HttpFragilityApiException implements Exception {
  final String message;
  HttpFragilityApiException(this.message);

  @override
  String toString() => message;
}
