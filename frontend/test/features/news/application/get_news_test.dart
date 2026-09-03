import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/news_article.dart';
import 'package:pano_chart_frontend/features/news/api/news_api.dart';
import 'package:pano_chart_frontend/features/news/application/get_news.dart';

class _FakeNewsApi implements NewsApi {
  List<NewsListItem> listResult = [];
  NewsArticle? articleResult;
  int listCallCount = 0;
  int? lastLimit;
  String? lastSlug;

  @override
  Future<List<NewsListItem>> fetchList({int limit = 20}) async {
    listCallCount++;
    lastLimit = limit;
    return listResult;
  }

  @override
  Future<NewsArticle> fetchArticle(String slug) async {
    lastSlug = slug;
    if (articleResult != null) return articleResult!;
    throw Exception('Not found');
  }
}

void main() {
  group('GetNewsImpl', () {
    test('list delegates to api', () async {
      final fake = _FakeNewsApi()
        ..listResult = [
          const NewsListItem(
            slug: 's1',
            title: 'Title',
            date: '2026-01-01',
            status: 'released',
            excerpt: 'Excerpt.',
          ),
        ];

      final useCase = GetNewsImpl(fake);
      final result = await useCase.list();

      expect(result.length, 1);
      expect(result[0].slug, 's1');
      expect(fake.listCallCount, 1);
      expect(fake.lastLimit, 20);
    });

    test('list passes custom limit', () async {
      final fake = _FakeNewsApi();
      final useCase = GetNewsImpl(fake);

      await useCase.list(limit: 5);

      expect(fake.lastLimit, 5);
    });

    test('getBySlug delegates to api', () async {
      final fake = _FakeNewsApi()
        ..articleResult = const NewsArticle(
          slug: 'my-slug',
          title: 'Title',
          date: '2026-01-01',
          status: 'released',
          tags: ['a'],
          body: 'Body.',
        );

      final useCase = GetNewsImpl(fake);
      final article = await useCase.getBySlug('my-slug');

      expect(article.slug, 'my-slug');
      expect(fake.lastSlug, 'my-slug');
    });

    test('list returns empty when api returns empty', () async {
      final fake = _FakeNewsApi();
      final useCase = GetNewsImpl(fake);

      final result = await useCase.list();

      expect(result, isEmpty);
    });
  });
}
