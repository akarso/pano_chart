import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../auth/auth_headers.dart';
import '../api/subscription_api.dart';

/// HTTP implementation of [SubscriptionApi] that talks to the backend.
class HttpSubscriptionApi implements SubscriptionApi {
  final http.Client client;
  final String baseUrl;
  final String? Function()? getAuthSecret;
  final Future<void> Function()? onUnauthorized;

  HttpSubscriptionApi({
    required this.client,
    required this.baseUrl,
    this.getAuthSecret,
    this.onUnauthorized,
  });

  @override
  Future<void> verifyPurchase({
    required String provider,
    required String purchaseToken,
    required String userId,
  }) async {
    // NOTE: /api/payments/verify does not check auth yet (backend PR-071) —
    // deliberately not attaching an Authorization header here so this
    // doesn't look already-secured. Add it back alongside the server-side
    // enforcement, not before.
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
    // userId is intentionally NOT sent — the backend derives the caller's
    // identity from the Authorization header (device secret), never from a
    // client-supplied value. The parameter stays for interface stability
    // across the call sites that still pass it.
    final uri = Uri.parse('$baseUrl/api/subscription/status');
    final response = await sendAuthenticated(
      (headers) => client.get(uri, headers: headers),
      getAuthSecret,
      onUnauthorized,
    );
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
