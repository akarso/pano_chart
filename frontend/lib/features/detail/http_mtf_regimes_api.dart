import 'dart:convert';
import 'package:http/http.dart' as http;
import 'mtf_regimes_data.dart';

/// Fetches a symbol's multi-timeframe regime stack from the backend.
abstract class MtfRegimesApi {
  Future<MtfRegimesData> fetch({required String symbol});
}

class HttpMtfRegimesApi implements MtfRegimesApi {
  final http.Client client;
  final String baseUrl;

  HttpMtfRegimesApi({required this.client, required this.baseUrl});

  @override
  Future<MtfRegimesData> fetch({required String symbol}) async {
    final uri = Uri.parse('$baseUrl/api/symbol/$symbol/regimes');
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpMtfRegimesApiException(
        'MTF regimes API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return MtfRegimesData.fromJson(json);
  }
}

class HttpMtfRegimesApiException implements Exception {
  final String message;
  HttpMtfRegimesApiException(this.message);

  @override
  String toString() => message;
}
