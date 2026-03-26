import 'dart:convert';
import 'package:http/http.dart' as http;
import '../api/subscription_api.dart';

/// HTTP implementation of [SubscriptionApi] that talks to the backend.
class HttpSubscriptionApi implements SubscriptionApi {
  final http.Client client;
  final String baseUrl;

  HttpSubscriptionApi({required this.client, required this.baseUrl});

  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {
    final uri = Uri.parse('$baseUrl/api/payments/verify');
    final response = await client.post(
      uri,
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({
        'provider': provider,
        'purchaseToken': purchaseToken,
        'userId': userId,
      }),
    );
    if (response.statusCode != 200) {
      throw Exception(
        'Purchase verification failed (${response.statusCode}): ${response.body}',
      );
    }
  }

  @override
  Future<SubscriptionStatus> getStatus(String userId) async {
    final uri =
        Uri.parse('$baseUrl/api/subscription/status?userId=$userId');
    final response = await client.get(uri);
    if (response.statusCode != 200) {
      throw Exception(
        'Failed to get subscription status (${response.statusCode})',
      );
    }
    final data = jsonDecode(response.body) as Map<String, dynamic>;
    final active = data['active'] as bool? ?? false;
    final expiresAtStr = data['expires_at'] as String?;
    final expiresAt =
        expiresAtStr != null ? DateTime.tryParse(expiresAtStr) : null;
    return SubscriptionStatus(active: active, expiresAt: expiresAt);
  }
}
