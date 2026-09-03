import 'dart:convert';
import 'package:http/http.dart' as http;
import 'composite_index_data.dart';

/// Fetches the composite market index from the backend.
abstract class CompositeIndexApi {
  Future<CompositeIndexData> fetch({String timeframe, int limit});
}

class HttpCompositeIndexApi implements CompositeIndexApi {
  final http.Client client;
  final String baseUrl;

  HttpCompositeIndexApi({required this.client, required this.baseUrl});

  @override
  Future<CompositeIndexData> fetch({
    String timeframe = '4h',
    int limit = 200,
  }) async {
    final uri = Uri.parse(
      '$baseUrl/api/market/composite?timeframe=$timeframe&limit=$limit',
    );
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpCompositeIndexApiException(
        'Composite index API error: ${response.statusCode}',
      );
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return CompositeIndexData.fromJson(json);
  }
}

class HttpCompositeIndexApiException implements Exception {
  final String message;
  HttpCompositeIndexApiException(this.message);

  @override
  String toString() => message;
}
