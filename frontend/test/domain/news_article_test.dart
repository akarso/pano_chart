import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/news_article.dart';

void main() {
  group('NewsListItem', () {
    test('fromJson parses all fields', () {
      final json = {
        'slug': 'test-article',
        'title': 'Test Article',
        'date': '2026-03-01',
        'status': 'released',
        'excerpt': 'Short summary...',
        'eta': '2026-04-01',
      };
      final item = NewsListItem.fromJson(json);

      expect(item.slug, 'test-article');
      expect(item.title, 'Test Article');
      expect(item.date, '2026-03-01');
      expect(item.status, 'released');
      expect(item.excerpt, 'Short summary...');
      expect(item.eta, '2026-04-01');
    });

    test('fromJson with null eta', () {
      final json = {
        'slug': 'no-eta',
        'title': 'No ETA',
        'date': '2026-01-01',
        'status': 'info',
        'excerpt': 'Body text.',
      };
      final item = NewsListItem.fromJson(json);

      expect(item.eta, isNull);
    });
  });

  group('NewsArticle', () {
    test('fromJson parses all fields', () {
      final json = {
        'slug': 'full-article',
        'title': 'Full Article',
        'date': '2026-03-05',
        'status': 'planned',
        'tags': ['feature', 'release'],
        'body': 'Full body content here.',
        'eta': '2026-04-15',
        'priority': 'high',
      };
      final article = NewsArticle.fromJson(json);

      expect(article.slug, 'full-article');
      expect(article.title, 'Full Article');
      expect(article.date, '2026-03-05');
      expect(article.status, 'planned');
      expect(article.tags, ['feature', 'release']);
      expect(article.body, 'Full body content here.');
      expect(article.eta, '2026-04-15');
      expect(article.priority, 'high');
    });

    test('fromJson with null optional fields', () {
      final json = {
        'slug': 'minimal',
        'title': 'Minimal',
        'date': '2026-01-01',
        'status': 'info',
        'tags': <String>[],
        'body': 'Some body.',
      };
      final article = NewsArticle.fromJson(json);

      expect(article.eta, isNull);
      expect(article.priority, isNull);
      expect(article.tags, isEmpty);
    });

    test('fromJson with null tags defaults to empty list', () {
      final json = {
        'slug': 'no-tags',
        'title': 'No Tags',
        'date': '2026-01-01',
        'status': 'released',
        'body': 'Content.',
      };
      final article = NewsArticle.fromJson(json);

      expect(article.tags, isEmpty);
    });
  });
}
