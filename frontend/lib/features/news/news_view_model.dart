import 'dart:ui' show VoidCallback;

import '../../domain/news_article.dart';
import 'application/get_news.dart';

/// Immutable state for the news feature.
class NewsState {
  final bool isLoading;
  final List<NewsListItem> articles;
  final NewsArticle? selectedArticle;
  final String? error;

  const NewsState({
    required this.isLoading,
    required this.articles,
    this.selectedArticle,
    this.error,
  });

  factory NewsState.initial() => const NewsState(
        isLoading: false,
        articles: [],
        error: null,
      );

  NewsState copyWith({
    bool? isLoading,
    List<NewsListItem>? articles,
    NewsArticle? selectedArticle,
    String? error,
    bool clearSelectedArticle = false,
  }) {
    return NewsState(
      isLoading: isLoading ?? this.isLoading,
      articles: articles ?? this.articles,
      selectedArticle: clearSelectedArticle
          ? null
          : (selectedArticle ?? this.selectedArticle),
      error: error,
    );
  }
}

/// ViewModel for the news feature.
///
/// Follows the same callback-based notification pattern used throughout the app.
class NewsViewModel {
  final GetNews _getNews;
  VoidCallback? onChanged;

  NewsState _state = NewsState.initial();
  NewsState get state => _state;

  NewsViewModel(this._getNews);

  /// Loads the news article list.
  Future<void> loadArticles({int limit = 20}) async {
    _state = _state.copyWith(isLoading: true, error: null);
    onChanged?.call();

    try {
      final articles = await _getNews.list(limit: limit);
      _state = _state.copyWith(isLoading: false, articles: articles);
    } catch (e) {
      _state = _state.copyWith(isLoading: false, error: e.toString());
    }
    onChanged?.call();
  }

  /// Loads a single article by slug.
  Future<void> loadArticle(String slug) async {
    _state = _state.copyWith(isLoading: true, error: null);
    onChanged?.call();

    try {
      final article = await _getNews.getBySlug(slug);
      _state = _state.copyWith(isLoading: false, selectedArticle: article);
    } catch (e) {
      _state = _state.copyWith(isLoading: false, error: e.toString());
    }
    onChanged?.call();
  }

  /// Clears the selected article (e.g., when navigating back).
  void clearSelectedArticle() {
    _state = _state.copyWith(clearSelectedArticle: true);
    onChanged?.call();
  }
}
