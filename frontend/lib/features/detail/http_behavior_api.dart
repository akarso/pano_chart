import 'dart:convert';
import 'package:http/http.dart' as http;
import 'behavior_data.dart';

/// Fetches retail behavior scores for a symbol.
abstract class BehaviorApi {
  Future<BehaviorData> fetch({required String symbol, String timeframe});
}

class HttpBehaviorApi implements BehaviorApi {
  final http.Client client;
  final String baseUrl;

  HttpBehaviorApi({required this.client, required this.baseUrl});

  @override
  Future<BehaviorData> fetch({
    required String symbol,
    String timeframe = '4h',
  }) async {
    final uri = Uri.parse(
        '$baseUrl/api/token/$symbol/behavior?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpBehaviorApiException(
        'Behavior API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return BehaviorData.fromJson(json);
  }
}

class HttpBehaviorApiException implements Exception {
  final String message;
  HttpBehaviorApiException(this.message);

  @override
  String toString() => message;
}
