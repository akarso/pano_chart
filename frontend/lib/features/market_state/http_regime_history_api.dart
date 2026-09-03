import 'dart:convert';
import 'package:http/http.dart' as http;
import 'regime_history_data.dart';

/// Fetches regime history from the backend.
abstract class RegimeHistoryApi {
  Future<RegimeHistoryData> fetch({String timeframe});
}

class HttpRegimeHistoryApi implements RegimeHistoryApi {
  final http.Client client;
  final String baseUrl;

  HttpRegimeHistoryApi({required this.client, required this.baseUrl});

  @override
  Future<RegimeHistoryData> fetch({String timeframe = '4h'}) async {
    final uri = Uri.parse(
        '$baseUrl/api/market/regime/history?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpRegimeHistoryApiException(
        'Regime history API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return RegimeHistoryData.fromJson(json);
  }
}

class HttpRegimeHistoryApiException implements Exception {
  final String message;
  HttpRegimeHistoryApiException(this.message);

  @override
  String toString() => message;
}
