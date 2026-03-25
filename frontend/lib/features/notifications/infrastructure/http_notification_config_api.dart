import 'dart:convert';

import 'package:http/http.dart' as http;

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

  HttpNotificationConfigApi({required this.client, required this.baseUrl});

  @override
  Future<NotificationSettings> fetch(String userId) async {
    final uri = Uri.parse(baseUrl).replace(
      path: '/api/notification/config',
      queryParameters: {'user_id': userId},
    );

    final response =
        await client.get(uri).timeout(const Duration(seconds: 15));

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

    final response = await client
        .put(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpNotificationConfigException(
        statusCode: response.statusCode,
        message: 'Notification config save error: ${response.statusCode}',
      );
    }
  }
}
