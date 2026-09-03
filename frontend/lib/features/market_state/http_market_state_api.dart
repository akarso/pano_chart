import 'dart:convert';
import 'package:http/http.dart' as http;
import 'market_state_data.dart';

/// Fetches the market state summary from the backend.
abstract class MarketStateApi {
  Future<MarketStateData> fetch({String timeframe});
}

class HttpMarketStateApi implements MarketStateApi {
  final http.Client client;
  final String baseUrl;

  HttpMarketStateApi({required this.client, required this.baseUrl});

  @override
  Future<MarketStateData> fetch({String timeframe = '4h'}) async {
    final uri = Uri.parse('$baseUrl/api/market/state?timeframe=$timeframe');
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpMarketStateApiException(
        'Market state API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return MarketStateData.fromJson(json);
  }
}

class HttpMarketStateApiException implements Exception {
  final String message;
  HttpMarketStateApiException(this.message);

  @override
  String toString() => message;
}
