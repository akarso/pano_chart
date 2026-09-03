import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/notifications/infrastructure/http_notification_config_api.dart';
import 'package:pano_chart_frontend/features/notifications/notification_settings_model.dart';

void main() {
  group('HttpNotificationConfigApi', () {
    test('fetch parses server response into settings', () async {
      final client = MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/api/notification/config');
        expect(request.url.queryParameters.containsKey('user_id'), isFalse);

        return http.Response(
          jsonEncode({
            'user_id': 'u1',
            'social': true,
            'macro_high': false,
            'macro_moderate': true,
            'news': true,
            'uptrend': true,
            'downtrend': false,
            'sideways': true,
            'setup_of_day': false,
            'uptrend_min_dominance': 0.80,
            'downtrend_min_dominance': 0.60,
            'sideways_min_dominance': 0.70,
            'setup_min_score': 0.85,
            'uptrend_timeframe': '15m',
            'downtrend_timeframe': '4h',
            'sideways_timeframe': '1d',
            'setup_timeframe': '5m',
          }),
          200,
        );
      });

      final api = HttpNotificationConfigApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final s = await api.fetch('u1');
      expect(s.social, isTrue);
      expect(s.macroHigh, isFalse);
      expect(s.macroModerate, isTrue);
      expect(s.news, isTrue);
      expect(s.uptrend, isTrue);
      expect(s.downtrend, isFalse);
      expect(s.sideways, isTrue);
      expect(s.setupOfDay, isFalse);
      expect(s.uptrendMinDominance, 0.80);
      expect(s.downtrendMinDominance, 0.60);
      expect(s.sidewaysMinDominance, 0.70);
      expect(s.setupMinScore, 0.85);
      expect(s.uptrendTimeframe, '15m');
      expect(s.downtrendTimeframe, '4h');
      expect(s.sidewaysTimeframe, '1d');
      expect(s.setupTimeframe, '5m');
    });

    test('fetch attaches Authorization header when a secret is available',
        () async {
      String? authHeader;
      final client = MockClient((request) async {
        authHeader = request.headers['Authorization'];
        return http.Response(jsonEncode({'user_id': 'u1'}), 200);
      });

      final api = HttpNotificationConfigApi(
        client: client,
        baseUrl: 'http://localhost:8080',
        getAuthSecret: () => 'my-secret',
      );

      await api.fetch('u1');
      expect(authHeader, 'Bearer my-secret');
    });

    test('fetch throws on non-200', () async {
      final client = MockClient(
          (request) async => http.Response('error', 500));

      final api = HttpNotificationConfigApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(
        () => api.fetch('u1'),
        throwsA(isA<HttpNotificationConfigException>()),
      );
    });

    test('save sends PUT with correct body', () async {
      Map<String, dynamic>? sentBody;
      final client = MockClient((request) async {
        expect(request.method, 'PUT');
        expect(request.url.path, '/api/notification/config');
        sentBody = jsonDecode(request.body) as Map<String, dynamic>;
        return http.Response('{"ok":true}', 200);
      });

      final api = HttpNotificationConfigApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final s = NotificationSettings(
        social: false,
        macroHigh: true,
        macroModerate: false,
        uptrend: true,
        downtrend: false,
        sideways: true,
        setupOfDay: true,
        news: false,
        uptrendMinDominance: 0.60,
        downtrendMinDominance: 0.80,
        sidewaysMinDominance: 0.50,
        setupMinScore: 0.90,
        uptrendTimeframe: '15m',
        downtrendTimeframe: '4h',
        sidewaysTimeframe: '1d',
        setupTimeframe: '5m',
      );

      await api.save('u1', s);
      expect(sentBody, isNotNull);
      expect(sentBody!['user_id'], 'u1');
      expect(sentBody!['social'], isFalse);
      expect(sentBody!['uptrend'], isTrue);
      expect(sentBody!['uptrend_min_dominance'], 0.60);
      expect(sentBody!['setup_min_score'], 0.90);
      expect(sentBody!['uptrend_timeframe'], '15m');
      expect(sentBody!['downtrend_timeframe'], '4h');
      expect(sentBody!['sideways_timeframe'], '1d');
      expect(sentBody!['setup_timeframe'], '5m');
    });

    test('save throws on non-200', () async {
      final client = MockClient(
          (request) async => http.Response('error', 500));

      final api = HttpNotificationConfigApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final s = NotificationSettings.defaults();
      expect(
        () => api.save('u1', s),
        throwsA(isA<HttpNotificationConfigException>()),
      );
    });
  });
}
