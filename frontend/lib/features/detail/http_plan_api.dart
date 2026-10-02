import 'dart:convert';
import 'package:http/http.dart' as http;
import 'plan_data.dart';

/// Fetches a range trade plan from the backend (PR-110).
abstract class PlanApi {
  Future<PlanData> fetch({
    required String symbol,
    required String timeframe,
    double risk = 100,
  });
}

class HttpPlanApi implements PlanApi {
  final http.Client client;
  final String baseUrl;

  HttpPlanApi({required this.client, required this.baseUrl});

  @override
  Future<PlanData> fetch({
    required String symbol,
    required String timeframe,
    double risk = 100,
  }) async {
    final uri = Uri.parse('$baseUrl/api/symbol/$symbol/plan').replace(
      queryParameters: {
        'timeframe': timeframe,
        'risk': risk == risk.roundToDouble()
            ? risk.toStringAsFixed(0)
            : risk.toString(),
      },
    );
    final response = await client.get(uri).timeout(const Duration(seconds: 15));
    if (response.statusCode != 200) {
      throw HttpPlanApiException('Plan API error: ${response.statusCode}');
    }
    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return PlanData.fromJson(json);
  }
}

class HttpPlanApiException implements Exception {
  final String message;
  HttpPlanApiException(this.message);

  @override
  String toString() => message;
}
