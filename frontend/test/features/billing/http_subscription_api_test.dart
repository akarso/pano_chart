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
        expect(body['userId'], 'user_1');
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
    });

    group('getStatus', () {
      test('parses active subscription with expiry', () async {
        final client = MockClient((req) async {
          expect(req.url.toString(),
              'http://localhost:8080/api/subscription/status?userId=user_1');
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
    });
  });

  group('getSolPrice', () {
    test('parses SOL price response', () async {
      final client = MockClient((req) async {
        expect(req.url.toString(),
            'http://localhost:8080/api/sol/price');
        expect(req.method, 'GET');
        return http.Response(
          jsonEncode({
            'sol_price': 130.5,
            'required_sol': 0.038238,
            'price_usd': 4.99,
            'wallet': 'SomeWalletAddress123',
          }),
          200,
        );
      });

      final api = HttpSubscriptionApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      final info = await api.getSolPrice();

      expect(info.solPrice, 130.5);
      expect(info.requiredSOL, 0.038238);
      expect(info.priceUSD, 4.99);
      expect(info.wallet, 'SomeWalletAddress123');
    });

    test('throws on non-200 response', () async {
      final client = MockClient((_) async {
        return http.Response('service unavailable', 503);
      });

      final api = HttpSubscriptionApi(
        client: client,
        baseUrl: 'http://localhost:8080',
      );

      expect(() => api.getSolPrice(), throwsException);
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
