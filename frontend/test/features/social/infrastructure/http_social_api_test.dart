import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart' as http_testing;

import 'package:pano_chart_frontend/features/social/api/social_account_settings.dart';
import 'package:pano_chart_frontend/features/social/infrastructure/http_social_api.dart';

void main() {
  group('HttpSocialApi', () {
    test('fetchFeed returns response on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/social/feed');
        expect(req.url.queryParameters['handle'], 'elonmusk');
        return http.Response(
          jsonEncode({
            'handle': 'elonmusk',
            'count': 1,
            'posts': [
              {
                'id': 'p1',
                'account_id': 'twitter:elonmusk',
                'author': 'elonmusk',
                'title': 'Hello',
                'url': 'https://x.com/1',
                'timestamp': 1700000000,
              },
            ],
          }),
          200,
        );
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      final result = await api.fetchFeed('elonmusk');

      expect(result.handle, 'elonmusk');
      expect(result.count, 1);
      expect(result.posts.length, 1);
      expect(result.posts[0].id, 'p1');
    });

    test('fetchFeed throws on non-200', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('error', 500),
      );

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.fetchFeed('bad'),
        throwsA(isA<HttpSocialApiException>()),
      );
    });

    test('subscribe sends POST with correct body on 201', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/social/subscribe');
        expect(req.method, 'POST');
        final body = jsonDecode(req.body) as Map<String, dynamic>;
        expect(body['user_id'], 'u1');
        expect(body['handle'], 'elonmusk');
        expect(req.headers['Content-Type'], contains('application/json'));
        return http.Response(jsonEncode({'status': 'subscribed'}), 201);
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      await api.subscribe(userId: 'u1', handle: 'elonmusk');
    });

    test('subscribe throws on non-201', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('error', 400),
      );

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.subscribe(userId: 'u1', handle: 'bad'),
        throwsA(isA<HttpSocialApiException>()),
      );
    });

    test('unsubscribe sends POST with correct body on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/social/unsubscribe');
        expect(req.method, 'POST');
        final body = jsonDecode(req.body) as Map<String, dynamic>;
        expect(body['user_id'], 'u1');
        expect(body['handle'], 'elonmusk');
        return http.Response(jsonEncode({'status': 'unsubscribed'}), 200);
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      await api.unsubscribe(userId: 'u1', handle: 'elonmusk');
    });

    test('unsubscribe throws on non-200', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('error', 500),
      );

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.unsubscribe(userId: 'u1', handle: 'fail'),
        throwsA(isA<HttpSocialApiException>()),
      );
    });

    test('fetchAccounts returns response on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/social/accounts');
        expect(req.url.queryParameters['user_id'], 'u1');
        return http.Response(
          jsonEncode({
            'user_id': 'u1',
            'count': 2,
            'accounts': ['twitter:elonmusk', 'twitter:binance'],
          }),
          200,
        );
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      final result = await api.fetchAccounts('u1');

      expect(result.userId, 'u1');
      expect(result.count, 2);
      expect(result.accounts, ['twitter:elonmusk', 'twitter:binance']);
    });

    test('fetchAccounts throws on non-200', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('error', 403),
      );

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.fetchAccounts('u1'),
        throwsA(isA<HttpSocialApiException>()),
      );
    });

    test('fetchFeed passes filter query params when settings provided',
        () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/social/feed');
        expect(req.url.queryParameters['handle'], 'alice');
        expect(req.url.queryParameters['omit_retweets'], 'true');
        expect(req.url.queryParameters['min_length'], '50');
        expect(req.url.queryParameters['keywords'], 'bitcoin,eth');
        return http.Response(
          jsonEncode({
            'handle': 'alice',
            'count': 0,
            'posts': [],
          }),
          200,
        );
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      await api.fetchFeed(
        'alice',
        settings: const SocialAccountSettings(
          omitRetweets: true,
          minLength: 50,
          keywords: ['bitcoin', 'eth'],
        ),
      );
    });

    test('fetchFeed omits filter params when settings are default', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.queryParameters.containsKey('omit_retweets'), isFalse);
        expect(req.url.queryParameters.containsKey('min_length'), isFalse);
        expect(req.url.queryParameters.containsKey('keywords'), isFalse);
        return http.Response(
          jsonEncode({'handle': 'bob', 'count': 0, 'posts': []}),
          200,
        );
      });

      final api = HttpSocialApi(client: client, baseUrl: 'http://localhost');
      await api.fetchFeed('bob');
    });
  });
}
