import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/device_auth_api.dart';

/// Exception thrown by [HttpDeviceAuthApi] on non-2xx responses.
class HttpDeviceAuthException implements Exception {
  final int statusCode;
  final String message;

  const HttpDeviceAuthException({required this.statusCode, required this.message});

  @override
  String toString() => 'HttpDeviceAuthException($statusCode): $message';
}

/// HTTP adapter implementing [DeviceAuthApi].
class HttpDeviceAuthApi implements DeviceAuthApi {
  final http.Client client;
  final String baseUrl;

  HttpDeviceAuthApi({required this.client, required this.baseUrl});

  @override
  Future<DeviceClaim> claim({String? existingUserId}) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/device/claim');
    final body = jsonEncode({
      if (existingUserId != null) 'existingUserId': existingUserId,
    });

    final response = await client
        .post(uri, body: body, headers: {'Content-Type': 'application/json'})
        .timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpDeviceAuthException(
        statusCode: response.statusCode,
        message: 'Device claim error: ${response.statusCode}',
      );
    }

    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return DeviceClaim(
      userId: json['userId'] as String,
      secret: json['secret'] as String,
    );
  }
}
