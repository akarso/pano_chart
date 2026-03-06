import 'package:flutter_test/flutter_test.dart';
import 'package:pano_chart_frontend/domain/news_article.dart';
import 'package:pano_chart_frontend/features/news/application/get_news.dart';
import 'package:pano_chart_frontend/features/news/news_view_model.dart';

class _FakeGetNews implements GetNews {
  List<NewsListItem> listResult = [];
  NewsArticle? articleResult;
  bool shouldThrow = false;

  @override
  Future<List<NewsListItem>> list({int limit = 20}) async {
    if (shouldThrow) throw Exception('network error');
    return listResult;
  }

  @override
  Future<NewsArticle> getBySlug(String slug) async {
    if (shouldThrow) throw Exception('network error');
    if (articleResult != null) return articleResult!;
    throw Exception('Not found');
  }
}

void main() {
  group('NewsState', () {
    test('initial state', () {
      final state = NewsState.initial();

      expect(state.isLoading, isFalse);
      expect(state.articles, isEmpty);
      expect(state.selectedArticle, isNull);
      expect(state.error, isNull);
    });

    test('copyWith preserves unchanged fields', () {
      final state = NewsState.initial();
      final next = state.copyWith(isLoading: true);

      expect(next.isLoading, isTrue);
      expect(next.articles, isEmpty);
      expect(next.error, isNull);
    });

    test('copyWith clearSelectedArticle', () {
      final article = const NewsArticle(
        slug: 'a',
        title: 'A',
        date: '2026-01-01',
        status: 'released',
        tags: [],
        body: 'Body.',
      );
      final state = NewsState(
        isLoading: false,
        articles: const [],
        selectedArticle: article,
      );
      final next = state.copyWith(clearSelectedArticle: true);

      expect(next.selectedArticle, isNull);
    });
  });

  group('NewsViewModel', () {
    test('initial state', () {
      final vm = NewsViewModel(_FakeGetNews());

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.articles, isEmpty);
      expect(vm.state.error, isNull);
    });

    test('loadArticles sets articles on success', () async {
      final fake = _FakeGetNews()
        ..listResult = [
          const NewsListItem(
            slug: 'n1',
            title: 'News One',
            date: '2026-03-01',
            status: 'released',
            excerpt: 'Excerpt.',
          ),
        ];
      final vm = NewsViewModel(fake);
      int notifications = 0;
      vm.onChanged = () => notifications++;

      await vm.loadArticles();

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.articles.length, 1);
      expect(vm.state.articles[0].slug, 'n1');
      expect(vm.state.error, isNull);
      expect(notifications, greaterThanOrEqualTo(2)); // loading + done
    });

    test('loadArticles sets error on failure', () async {
      final fake = _FakeGetNews()..shouldThrow = true;
      final vm = NewsViewModel(fake);

      await vm.loadArticles();

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.articles, isEmpty);
      expect(vm.state.error, isNotNull);
    });

    test('loadArticle sets selectedArticle on success', () async {
      final fake = _FakeGetNews()
        ..articleResult = const NewsArticle(
          slug: 'full',
          title: 'Full Article',
          date: '2026-03-05',
          status: 'planned',
          tags: ['feature'],
          body: 'Full body content.',
          eta: '2026-04-01',
        );
      final vm = NewsViewModel(fake);
      int notifications = 0;
      vm.onChanged = () => notifications++;

      await vm.loadArticle('full');

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.selectedArticle, isNotNull);
      expect(vm.state.selectedArticle!.slug, 'full');
      expect(vm.state.error, isNull);
      expect(notifications, greaterThanOrEqualTo(2));
    });

    test('loadArticle sets error on failure', () async {
      final fake = _FakeGetNews()..shouldThrow = true;
      final vm = NewsViewModel(fake);

      await vm.loadArticle('bad-slug');

      expect(vm.state.isLoading, isFalse);
      expect(vm.state.selectedArticle, isNull);
      expect(vm.state.error, isNotNull);
    });

    test('clearSelectedArticle resets selection', () async {
      final fake = _FakeGetNews()
        ..articleResult = const NewsArticle(
          slug: 'x',
          title: 'X',
          date: '2026-01-01',
          status: 'released',
          tags: [],
          body: 'Body.',
        );
      final vm = NewsViewModel(fake);
      await vm.loadArticle('x');
      expect(vm.state.selectedArticle, isNotNull);

      vm.clearSelectedArticle();

      expect(vm.state.selectedArticle, isNull);
    });
  });
}
