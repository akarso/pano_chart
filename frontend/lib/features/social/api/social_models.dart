/// A single social media post as returned by the backend.
class SocialPost {
  final String id;
  final String accountId;
  final String author;
  final String title;
  final String url;
  final int timestamp;
  final bool isRetweet;

  const SocialPost({
    required this.id,
    required this.accountId,
    required this.author,
    required this.title,
    required this.url,
    required this.timestamp,
    this.isRetweet = false,
  });

  factory SocialPost.fromJson(Map<String, dynamic> json) {
    return SocialPost(
      id: json['id'] as String,
      accountId: json['account_id'] as String,
      author: json['author'] as String,
      title: json['title'] as String,
      url: json['url'] as String,
      timestamp: json['timestamp'] as int,
      isRetweet: json['is_retweet'] as bool? ?? false,
    );
  }

  DateTime get dateTime =>
      DateTime.fromMillisecondsSinceEpoch(timestamp * 1000, isUtc: true);
}

/// Response wrapper for the social feed endpoint.
class SocialFeedResponse {
  final String handle;
  final int count;
  final List<SocialPost> posts;

  const SocialFeedResponse({
    required this.handle,
    required this.count,
    required this.posts,
  });

  factory SocialFeedResponse.fromJson(Map<String, dynamic> json) {
    return SocialFeedResponse(
      handle: json['handle'] as String,
      count: json['count'] as int,
      posts: (json['posts'] as List<dynamic>?)
              ?.map((e) => SocialPost.fromJson(e as Map<String, dynamic>))
              .toList() ??
          const [],
    );
  }
}

/// Response wrapper for the social accounts endpoint.
class SocialAccountsResponse {
  final String userId;
  final int count;
  final List<String> accounts;

  const SocialAccountsResponse({
    required this.userId,
    required this.count,
    required this.accounts,
  });

  factory SocialAccountsResponse.fromJson(Map<String, dynamic> json) {
    return SocialAccountsResponse(
      userId: json['user_id'] as String,
      count: json['count'] as int,
      accounts:
          (json['accounts'] as List<dynamic>?)?.cast<String>() ?? const [],
    );
  }
}
