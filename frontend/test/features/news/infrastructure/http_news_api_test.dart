import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart' as http_testing;

import 'package:pano_chart_frontend/features/news/infrastructure/http_news_api.dart';

void main() {
  group('HttpNewsApi', () {
    test('fetchList returns items on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/news');
        expect(req.url.queryParameters['limit'], '20');
        return http.Response(
          jsonEncode([
            {
              'slug': 'first-post',
              'title': 'First Post',
              'date': '2026-03-01',
              'status': 'released',
              'excerpt': 'Hello world.',
            },
          ]),
          200,
        );
      });

      final api = HttpNewsApi(client: client, baseUrl: 'http://localhost');
      final result = await api.fetchList();

      expect(result.length, 1);
      expect(result[0].slug, 'first-post');
      expect(result[0].title, 'First Post');
    });

    test('fetchList passes custom limit', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.queryParameters['limit'], '5');
        return http.Response(jsonEncode([]), 200);
      });

      final api = HttpNewsApi(client: client, baseUrl: 'http://localhost');
      final result = await api.fetchList(limit: 5);

      expect(result, isEmpty);
    });

    test('fetchList throws on non-200', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('error', 500),
      );

      final api = HttpNewsApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.fetchList(),
        throwsA(isA<HttpNewsApiException>()),
      );
    });

    test('fetchArticle returns article on 200', () async {
      final client = http_testing.MockClient((req) async {
        expect(req.url.path, '/api/news/my-article');
        return http.Response(
          jsonEncode({
            'slug': 'my-article',
            'title': 'My Article',
            'date': '2026-03-05',
            'status': 'planned',
            'tags': ['feature'],
            'body': 'Full body.',
            'eta': '2026-04-01',
            'priority': 'high',
          }),
          200,
        );
      });

      final api = HttpNewsApi(client: client, baseUrl: 'http://localhost');
      final article = await api.fetchArticle('my-article');

      expect(article.slug, 'my-article');
      expect(article.body, 'Full body.');
      expect(article.tags, ['feature']);
      expect(article.priority, 'high');
    });

    test('fetchArticle throws on 404', () async {
      final client = http_testing.MockClient(
        (req) async => http.Response('not found', 404),
      );

      final api = HttpNewsApi(client: client, baseUrl: 'http://localhost');

      expect(
        () => api.fetchArticle('nope'),
        throwsA(isA<HttpNewsApiException>()),
      );
    });
  });
}
