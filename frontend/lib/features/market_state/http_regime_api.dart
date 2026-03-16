import 'dart:convert';
import 'package:http/http.dart' as http;
import 'regime_data.dart';

/// Fetches the market regime summary from the backend.
abstract class RegimeApi {
  Future<RegimeData> fetch({String timeframe});
}

class HttpRegimeApi implements RegimeApi {
  final http.Client client;
  final String baseUrl;

  HttpRegimeApi({required this.client, required this.baseUrl});

  @override
  Future<RegimeData> fetch({String timeframe = '4h'}) async {
    final uri = Uri.parse('$baseUrl/api/market/regime?timeframe=$timeframe');
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpRegimeApiException(
        'Regime API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return RegimeData.fromJson(json);
  }
}

class HttpRegimeApiException implements Exception {
  final String message;
  HttpRegimeApiException(this.message);

  @override
  String toString() => message;
}
