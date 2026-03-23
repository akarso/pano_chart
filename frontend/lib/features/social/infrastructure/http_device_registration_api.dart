import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/device_registration_api.dart';

/// Exception thrown by [HttpDeviceRegistrationApi] on non-2xx responses.
class HttpDeviceRegistrationException implements Exception {
  final int statusCode;
  final String message;

  const HttpDeviceRegistrationException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() =>
      'HttpDeviceRegistrationException($statusCode): $message';
}

/// HTTP adapter implementing [DeviceRegistrationApi].
class HttpDeviceRegistrationApi implements DeviceRegistrationApi {
  final http.Client client;
  final String baseUrl;

  HttpDeviceRegistrationApi({required this.client, required this.baseUrl});

  @override
  Future<void> register({
    required String userId,
    required String deviceId,
    required String fcmToken,
    required String platform,
  }) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/device/register');
    final body = jsonEncode({
      'user_id': userId,
      'device_id': deviceId,
      'fcm_token': fcmToken,
      'platform': platform,
    });

    final response = await client
        .post(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 201) {
      throw HttpDeviceRegistrationException(
        statusCode: response.statusCode,
        message: 'Device register error: ${response.statusCode}',
      );
    }
  }

  @override
  Future<void> unregister({required String deviceId}) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/device/unregister');
    final body = jsonEncode({'device_id': deviceId});

    final response = await client
        .post(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpDeviceRegistrationException(
        statusCode: response.statusCode,
        message: 'Device unregister error: ${response.statusCode}',
      );
    }
  }
}
