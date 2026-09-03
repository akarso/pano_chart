import '../../../domain/news_article.dart';
import '../api/news_api.dart';

/// Use case interface for fetching news articles.
abstract class GetNews {
  Future<List<NewsListItem>> list({int limit = 20});
  Future<NewsArticle> getBySlug(String slug);
}

/// Implementation that delegates to the [NewsApi] port.
class GetNewsImpl implements GetNews {
  final NewsApi _api;

  GetNewsImpl(this._api);

  @override
  Future<List<NewsListItem>> list({int limit = 20}) =>
      _api.fetchList(limit: limit);

  @override
  Future<NewsArticle> getBySlug(String slug) => _api.fetchArticle(slug);
}
