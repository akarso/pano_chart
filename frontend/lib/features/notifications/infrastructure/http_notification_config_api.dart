import 'dart:convert';

import 'package:http/http.dart' as http;

import '../../auth/auth_headers.dart';
import '../api/notification_config_api.dart';
import '../notification_settings_model.dart';

/// Exception thrown by [HttpNotificationConfigApi] on non-2xx responses.
class HttpNotificationConfigException implements Exception {
  final int statusCode;
  final String message;

  const HttpNotificationConfigException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() =>
      'HttpNotificationConfigException($statusCode): $message';
}

/// HTTP adapter implementing [NotificationConfigApi].
class HttpNotificationConfigApi implements NotificationConfigApi {
  final http.Client client;
  final String baseUrl;
  final String? Function()? getAuthSecret;
  final Future<void> Function()? onUnauthorized;

  HttpNotificationConfigApi({
    required this.client,
    required this.baseUrl,
    this.getAuthSecret,
    this.onUnauthorized,
  });

  @override
  Future<NotificationSettings> fetch(String userId) async {
    // user_id is intentionally NOT sent — the backend derives the caller's
    // identity from the Authorization header (device secret), never from a
    // client-supplied value. The parameter stays for interface stability
    // across the call sites that still pass it.
    final uri = Uri.parse(baseUrl).replace(path: '/api/notification/config');

    final response = await sendAuthenticated(
      (headers) => client.get(uri, headers: headers).timeout(const Duration(seconds: 15)),
      getAuthSecret,
      onUnauthorized,
    );

    if (response.statusCode != 200) {
      throw HttpNotificationConfigException(
        statusCode: response.statusCode,
        message: 'Notification config fetch error: ${response.statusCode}',
      );
    }

    final json = jsonDecode(response.body) as Map<String, dynamic>;
    final settings = NotificationSettings.defaults();
    settings.applyFromJson(json);
    return settings;
  }

  @override
  Future<void> save(String userId, NotificationSettings settings) async {
    final uri =
        Uri.parse(baseUrl).replace(path: '/api/notification/config');
    final body = jsonEncode(settings.toJson(userId));

    final response = await sendAuthenticated(
      (headers) => client
          .put(uri, body: body, headers: headers)
          .timeout(const Duration(seconds: 15)),
      getAuthSecret,
      onUnauthorized,
      {'Content-Type': 'application/json'},
    );

    if (response.statusCode != 200) {
      throw HttpNotificationConfigException(
        statusCode: response.statusCode,
        message: 'Notification config save error: ${response.statusCode}',
      );
    }
  }
}
