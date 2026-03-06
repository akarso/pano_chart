/// Represents a news article summary in a list.
class NewsListItem {
  final String slug;
  final String title;
  final String date;
  final String status;
  final String excerpt;
  final String? eta;

  const NewsListItem({
    required this.slug,
    required this.title,
    required this.date,
    required this.status,
    required this.excerpt,
    this.eta,
  });

  factory NewsListItem.fromJson(Map<String, dynamic> json) {
    return NewsListItem(
      slug: json['slug'] as String,
      title: json['title'] as String,
      date: json['date'] as String,
      status: json['status'] as String,
      excerpt: json['excerpt'] as String,
      eta: json['eta'] as String?,
    );
  }
}

/// Represents a full news article with body content.
class NewsArticle {
  final String slug;
  final String title;
  final String date;
  final String status;
  final List<String> tags;
  final String body;
  final String? eta;
  final String? priority;

  const NewsArticle({
    required this.slug,
    required this.title,
    required this.date,
    required this.status,
    required this.tags,
    required this.body,
    this.eta,
    this.priority,
  });

  factory NewsArticle.fromJson(Map<String, dynamic> json) {
    return NewsArticle(
      slug: json['slug'] as String,
      title: json['title'] as String,
      date: json['date'] as String,
      status: json['status'] as String,
      tags: (json['tags'] as List<dynamic>?)?.cast<String>() ?? const [],
      body: json['body'] as String,
      eta: json['eta'] as String?,
      priority: json['priority'] as String?,
    );
  }
}
