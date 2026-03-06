import 'dart:convert';

import 'package:http/http.dart' as http;

import '../api/news_api.dart';
import '../../../domain/news_article.dart';

/// Exception thrown by [HttpNewsApi] on non-200 responses.
class HttpNewsApiException implements Exception {
  final int statusCode;
  final String message;

  const HttpNewsApiException({
    required this.statusCode,
    required this.message,
  });

  @override
  String toString() => 'HttpNewsApiException($statusCode): $message';
}

/// HTTP adapter implementing [NewsApi] against the backend
/// `GET /api/news` and `GET /api/news/{slug}` endpoints.
class HttpNewsApi implements NewsApi {
  final http.Client client;
  final String baseUrl;

  HttpNewsApi({required this.client, required this.baseUrl});

  @override
  Future<List<NewsListItem>> fetchList({int limit = 20}) async {
    final uri = Uri.parse(baseUrl).replace(
      path: '/api/news',
      queryParameters: {'limit': '$limit'},
    );

    final response = await client.get(uri).timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpNewsApiException(
        statusCode: response.statusCode,
        message: 'News API error: ${response.statusCode}',
      );
    }

    final list = jsonDecode(response.body) as List<dynamic>;
    return list
        .map((json) => NewsListItem.fromJson(json as Map<String, dynamic>))
        .toList();
  }

  @override
  Future<NewsArticle> fetchArticle(String slug) async {
    final uri = Uri.parse(baseUrl).replace(path: '/api/news/$slug');

    final response = await client.get(uri).timeout(const Duration(seconds: 15));

    if (response.statusCode != 200) {
      throw HttpNewsApiException(
        statusCode: response.statusCode,
        message: 'News API error: ${response.statusCode}',
      );
    }

    final json = jsonDecode(response.body) as Map<String, dynamic>;
    return NewsArticle.fromJson(json);
  }
}
