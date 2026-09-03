import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/auth/infrastructure/http_device_auth_api.dart';

void main() {
  const baseUrl = 'http://localhost:8080';

  group('HttpDeviceAuthApi', () {
    test('claim sends POST and parses userId/secret', () async {
      Uri? capturedUri;
      String? capturedBody;
      final client = MockClient((req) async {
        capturedUri = req.url;
        capturedBody = req.body;
        return http.Response(
          jsonEncode({'userId': 'user1', 'secret': 'sekret'}),
          200,
        );
      });

      final api = HttpDeviceAuthApi(client: client, baseUrl: baseUrl);
      final claim = await api.claim();

      expect(capturedUri!.path, '/api/device/claim');
      expect(jsonDecode(capturedBody!), isEmpty);
      expect(claim.userId, 'user1');
      expect(claim.secret, 'sekret');
    });

    test('claim sends existingUserId when provided', () async {
      String? capturedBody;
      final client = MockClient((req) async {
        capturedBody = req.body;
        return http.Response(
          jsonEncode({'userId': 'legacy-1', 'secret': 'sekret'}),
          200,
        );
      });

      final api = HttpDeviceAuthApi(client: client, baseUrl: baseUrl);
      await api.claim(existingUserId: 'legacy-1');

      final decoded = jsonDecode(capturedBody!) as Map<String, dynamic>;
      expect(decoded['existingUserId'], 'legacy-1');
    });

    test('claim throws on non-200', () async {
      final client = MockClient((_) async => http.Response('error', 500));
      final api = HttpDeviceAuthApi(client: client, baseUrl: baseUrl);

      expect(() => api.claim(), throwsA(isA<HttpDeviceAuthException>()));
    });
  });
}
