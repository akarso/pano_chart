import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pano_chart_frontend/features/billing/api/subscription_api.dart';
import 'package:pano_chart_frontend/features/billing/infrastructure/http_subscription_api.dart';

void main() {
  group('HttpSubscriptionApi', () {
    group('verifyPurchase', () {
      test('sends correct POST request with JSON body', () async {
        String? capturedBody;
        String? capturedContentType;
        Uri? capturedUri;

        final client = MockClient((req) async {
          capturedUri = req.url;
          capturedContentType = req.headers['Content-Type'];
          capturedBody = req.body;
          return http.Response('', 200);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        await api.verifyPurchase(
          provider: 'google_play',
          purchaseToken: 'tok_abc',
          userId: 'user_1',
        );

        expect(capturedUri.toString(),
            'http://localhost:8080/api/payments/verify');
        expect(capturedContentType, startsWith('application/json'));

        final body = jsonDecode(capturedBody!) as Map<String, dynamic>;
        expect(body['provider'], 'google_play');
        expect(body['purchaseToken'], 'tok_abc');
        // userId is deliberately NOT sent — this endpoint has no
        // migration-window fallback, only the Authorization header counts.
        expect(body.containsKey('userId'), isFalse);
      });

      test('throws on non-200 response', () async {
        final client = MockClient((_) async {
          return http.Response('server error', 500);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        expect(
          () => api.verifyPurchase(
            provider: 'google_play',
            purchaseToken: 'tok',
            userId: 'u1',
          ),
          throwsException,
        );
      });

      test(
          'attaches Authorization header now that the server checks it (PR-071)',
          () async {
        String? authHeader;
        final client = MockClient((req) async {
          authHeader = req.headers['Authorization'];
          return http.Response('', 200);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
          getAuthSecret: () => 'my-secret',
        );

        await api.verifyPurchase(
          provider: 'google_play',
          purchaseToken: 'tok',
          userId: 'u1',
        );

        expect(authHeader, 'Bearer my-secret');
      });

      test('retries once after 401 with a re-claimed secret', () async {
        var callCount = 0;
        var reclaimCalled = false;
        var secret = 'stale-secret';

        final client = MockClient((req) async {
          callCount++;
          if (req.headers['Authorization'] == 'Bearer stale-secret') {
            return http.Response('unauthorized', 401);
          }
          return http.Response('', 200);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
          getAuthSecret: () => secret,
          onUnauthorized: () async {
            reclaimCalled = true;
            secret = 'fresh-secret';
          },
        );

        await api.verifyPurchase(
          provider: 'google_play',
          purchaseToken: 'tok',
          userId: 'u1',
        );

        expect(callCount, 2);
        expect(reclaimCalled, isTrue);
      });
    });

    group('getStatus', () {
      test('parses active subscription with expiry', () async {
        final client = MockClient((req) async {
          expect(req.url.toString(),
              'http://localhost:8080/api/subscription/status');
          return http.Response(
            jsonEncode({
              'active': true,
              'expires_at': '2025-12-31T23:59:59Z',
            }),
            200,
          );
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        final status = await api.getStatus('user_1');

        expect(status.active, isTrue);
        expect(status.expiresAt, isNotNull);
        expect(status.expiresAt!.year, 2025);
        expect(status.expiresAt!.month, 12);
      });

      test('parses inactive subscription', () async {
        final client = MockClient((_) async {
          return http.Response(jsonEncode({'active': false}), 200);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        final status = await api.getStatus('user_1');

        expect(status.active, isFalse);
        expect(status.expiresAt, isNull);
      });

      test('throws on non-200 response', () async {
        final client = MockClient((_) async {
          return http.Response('not found', 404);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
        );

        expect(() => api.getStatus('user_1'), throwsException);
      });

      test('retries once after 401 with a re-claimed secret', () async {
        var callCount = 0;
        var reclaimCalled = false;
        var secret = 'stale-secret';

        final client = MockClient((req) async {
          callCount++;
          if (req.headers['Authorization'] == 'Bearer stale-secret') {
            return http.Response('unauthorized', 401);
          }
          return http.Response(jsonEncode({'active': true}), 200);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
          getAuthSecret: () => secret,
          onUnauthorized: () async {
            reclaimCalled = true;
            secret = 'fresh-secret';
          },
        );

        final status = await api.getStatus('user_1');

        expect(callCount, 2);
        expect(reclaimCalled, isTrue);
        expect(status.active, isTrue);
      });

      test('does not retry when onUnauthorized is not provided', () async {
        var callCount = 0;
        final client = MockClient((req) async {
          callCount++;
          return http.Response('unauthorized', 401);
        });

        final api = HttpSubscriptionApi(
          client: client,
          baseUrl: 'http://localhost:8080',
          getAuthSecret: () => 'secret',
        );

        await expectLater(api.getStatus('user_1'), throwsException);
        expect(callCount, 1);
      });
    });
  });

  group('SubscriptionStatus', () {
    test('inactive factory', () {
      final s = SubscriptionStatus.inactive();
      expect(s.active, isFalse);
      expect(s.expiresAt, isNull);
    });

    test('toString includes fields', () {
      final s = SubscriptionStatus(active: true);
      expect(s.toString(), contains('active=true'));
    });
  });
}
