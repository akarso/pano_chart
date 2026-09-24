import 'dart:convert';
import 'package:http/http.dart' as http;
import 'composite_index_data.dart';

/// Candle window used by backend `CalculateTape` (`candleMetricsWindow`).
const tapeMetricsWindow = 110;

/// Composite chart fetch limit (ROADMAP historical context). Longer than
/// [tapeMetricsWindow]; the last N of a longer rebase is not the scored tape.
const compositeChartLimit = 200;

/// Known composite `regimeSource` values that pair with a chart series.
bool isKnownCompositeSource(String src) =>
    src == 'composite_volume_weighted' || src == 'composite_median';

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
    int limit = compositeChartLimit,
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
