import 'dart:convert';
import 'package:http/http.dart' as http;
import 'transition_data.dart';

/// Fetches market transition probabilities from the backend.
abstract class TransitionApi {
  Future<TransitionData> fetch({String timeframe});
}

class HttpTransitionApi implements TransitionApi {
  final http.Client client;
  final String baseUrl;

  HttpTransitionApi({required this.client, required this.baseUrl});

  @override
  Future<TransitionData> fetch({String timeframe = '4h'}) async {
    final uri =
        Uri.parse('$baseUrl/api/market/transition?timeframe=$timeframe');
    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpTransitionApiException(
        'Transition API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return TransitionData.fromJson(json);
  }
}

class HttpTransitionApiException implements Exception {
  final String message;
  HttpTransitionApiException(this.message);

  @override
  String toString() => message;
}
