import '../../../domain/news_article.dart';

/// Port for fetching news articles from the backend.
abstract class NewsApi {
  /// Fetches a list of news articles, sorted by date descending.
  Future<List<NewsListItem>> fetchList({int limit = 20});

  /// Fetches a single article by its slug.
  Future<NewsArticle> fetchArticle(String slug);
}
