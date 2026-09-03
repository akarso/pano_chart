import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:pano_chart_frontend/features/social/api/device_registration_api.dart';
import 'package:pano_chart_frontend/features/social/infrastructure/http_device_registration_api.dart';

void main() {
  const baseUrl = 'http://localhost:8080';

  group('HttpDeviceRegistrationApi', () {
    test('register sends POST with correct body', () async {
      String? capturedBody;
      Uri? capturedUri;
      final client = MockClient((req) async {
        capturedBody = req.body;
        capturedUri = req.url;
        return http.Response('{"status":"registered"}', 201);
      });

      final api =
          HttpDeviceRegistrationApi(client: client, baseUrl: baseUrl);

      await api.register(
        userId: 'u1',
        deviceId: 'd1',
        fcmToken: 'tok-abc',
        platform: 'android',
      );

      expect(capturedUri!.path, '/api/device/register');

      // user_id is deliberately NOT sent — the backend derives it from the
      // authenticated caller (Authorization header), never from the body.
      final decoded = jsonDecode(capturedBody!) as Map<String, dynamic>;
      expect(decoded.containsKey('user_id'), isFalse);
      expect(decoded['device_id'], 'd1');
      expect(decoded['fcm_token'], 'tok-abc');
      expect(decoded['platform'], 'android');
    });

    test('register attaches Authorization header when a secret is available',
        () async {
      Map<String, String>? capturedHeaders;
      final client = MockClient((req) async {
        capturedHeaders = req.headers;
        return http.Response('{"status":"registered"}', 201);
      });

      final api = HttpDeviceRegistrationApi(
        client: client,
        baseUrl: baseUrl,
        getAuthSecret: () => 'my-secret',
      );

      await api.register(
        userId: 'u1',
        deviceId: 'd1',
        fcmToken: 'tok-abc',
        platform: 'android',
      );

      expect(capturedHeaders!['Authorization'], 'Bearer my-secret');
    });

    test('register omits Authorization header when no secret is available',
        () async {
      Map<String, String>? capturedHeaders;
      final client = MockClient((req) async {
        capturedHeaders = req.headers;
        return http.Response('{"status":"registered"}', 201);
      });

      final api =
          HttpDeviceRegistrationApi(client: client, baseUrl: baseUrl);

      await api.register(
        userId: 'u1',
        deviceId: 'd1',
        fcmToken: 'tok-abc',
        platform: 'android',
      );

      expect(capturedHeaders!.containsKey('Authorization'), isFalse);
    });

    test('register throws on non-201', () async {
      final client = MockClient((_) async {
        return http.Response('{"error":"bad"}', 400);
      });

      final api =
          HttpDeviceRegistrationApi(client: client, baseUrl: baseUrl);

      expect(
        () => api.register(
          userId: 'u1',
          deviceId: 'd1',
          fcmToken: 'tok',
          platform: 'android',
        ),
        throwsA(isA<HttpDeviceRegistrationException>()),
      );
    });

    test('unregister sends POST with device_id', () async {
      String? capturedBody;
      final client = MockClient((req) async {
        capturedBody = req.body;
        return http.Response('{"status":"unregistered"}', 200);
      });

      final api =
          HttpDeviceRegistrationApi(client: client, baseUrl: baseUrl);

      await api.unregister(deviceId: 'd1');

      final decoded = jsonDecode(capturedBody!) as Map<String, dynamic>;
      expect(decoded['device_id'], 'd1');
    });

    test('unregister throws on non-200', () async {
      final client = MockClient((_) async {
        return http.Response('{"error":"bad"}', 500);
      });

      final api =
          HttpDeviceRegistrationApi(client: client, baseUrl: baseUrl);

      expect(
        () => api.unregister(deviceId: 'd1'),
        throwsA(isA<HttpDeviceRegistrationException>()),
      );
    });
  });
}
